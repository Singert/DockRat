package protocol

import (
	"encoding/json"
	"log"
	"net"

	"github.com/Singert/DockRat/core/node"
	"github.com/google/uuid"
)

func StartPortForward(agentID int, localPort string, target string, reg *node.Registry) {
	addr := ":" + localPort
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("[-] Failed to listen on %s: %v", addr, err)
		return
	}
	log.Printf("[+] Port forward listening on %s → agent[%d] → %s", addr, agentID, target)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("[-] Accept error: %v", err)
			continue
		}

		connID := uuid.New().String()
		log.Printf("[+] New connection %s from %s", connID, conn.RemoteAddr())

		// 通知 agent 建立连接
		start := ForwardStartPayload{ConnID: connID, Target: target}
		data, _ := json.Marshal(start)
		msg := Message{Type: MsgForwardStart, Payload: data}
		err = sendMessageOrRelay(agentID, msg, reg)
		if err != nil {
			log.Printf("[-] Send forward_start failed: %v", err)
			conn.Close()
			continue
		}

		// 启动发送线程
		go handleForwardSend(connID, conn, agentID, reg)

		// 等待 agent 数据回传（由 handleAgentMessages 分发）
		registerForwardConn(connID, conn)
	}
}
func handleForwardSend(connID string, conn net.Conn, agentID int, reg *node.Registry) {
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			break
		}
		payload := ForwardDataPayload{
			ConnID: connID,
			Data:   buf[:n],
		}
		data, _ := json.Marshal(payload)
		msg := Message{Type: MsgForwardData, Payload: data}
		err = sendMessageOrRelay(agentID, msg, reg)
		if err != nil {
			break
		}
	}
	// 通知 agent 关闭连接
	stop := ForwardStopPayload{ConnID: connID}
	data, _ := json.Marshal(stop)
	msg := Message{Type: MsgForwardStop, Payload: data}
	sendMessageOrRelay(agentID, msg, reg)
	conn.Close()
}

var forwardConnMap = make(map[string]net.Conn)

func registerForwardConn(connID string, conn net.Conn) {
	forwardConnMap[connID] = conn
}

func getForwardConn(connID string) (net.Conn, bool) {
	c, ok := forwardConnMap[connID]
	return c, ok
}

func removeForwardConn(connID string) {
	delete(forwardConnMap, connID)
}
func GetForwardConn(connID string) (net.Conn, bool) {
	conn, ok := forwardConnMap[connID]
	return conn, ok

}

func RemoveForwardConn(connID string) {
	delete(forwardConnMap, connID)
}
