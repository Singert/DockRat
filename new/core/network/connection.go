// file: new/core/network/listener.go
package network

import (
	"encoding/json"
	"io"
	"log"
	"net"

	"github.com/Singert/DockRat/core/node"
	"github.com/Singert/DockRat/core/protocol"
)

// 启动监听器并处理连接
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

// 处理与节点的连接
// 处理与节点的连接
func handleConnection(conn net.Conn, registry *node.Registry) {
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
		var payload protocol.HandshakePayload
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
		ctx := NewRelayContext(-1, 0, 10000, conn) // 创建一个新的 RelayContext，ID 和 ID 分配器可以根据需要调整
		// 这里调用 StartRelayAgent 来处理与代理的通信
		go StartRelayAgent(conn, ctx) // 这里传递的是 nil 作为上下文，后续可以根据需要修改
	} else {
		log.Println("[!] Unknown message type:", msg.Type)
		conn.Close()
	}
}
