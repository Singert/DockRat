package protocol

import (
	"encoding/json"
	"log"
	"net"
	"sync"

	"github.com/Singert/DockRat/core/node"
)

var (
	backwardConnMap = make(map[string]net.Conn)
	backwardTarget  = make(map[string]string) // connID → target
	mu              sync.Mutex
	connAgentMap    = make(map[string]int)
)
var backwardTargetPrefix = make(map[string]struct {
	Target  string
	AgentID int
})

func SetBackwardTargetPrefix(prefix string, target string, agentID int) {
	mu.Lock()
	defer mu.Unlock()
	backwardTargetPrefix[prefix] = struct {
		Target  string
		AgentID int
	}{target, agentID}
}

func SetBackwardTargetFromPrefix(connID string) bool {
	mu.Lock()
	defer mu.Unlock()

	for prefix, val := range backwardTargetPrefix {
		log.Printf("[match] Trying prefix %s against connID %s", prefix, connID)
		if len(connID) >= len(prefix) && connID[:len(prefix)] == prefix {
			backwardTarget[connID] = val.Target
			connAgentMap[connID] = val.AgentID
			//TODO: delete or not ?
			delete(backwardTargetPrefix, prefix)
			return true
		}
	}
	return false
}

var Registry *node.Registry // ← 必须设置，由 connection.go 提供

func SetBackwardTarget(connID string, target string, agentID int) {
	mu.Lock()
	defer mu.Unlock()
	backwardTarget[connID] = target
	connAgentMap[connID] = agentID
}
func RegisterBackwardConn(connID string, conn net.Conn) {
	mu.Lock()
	defer mu.Unlock()
	backwardConnMap[connID] = conn
}

func GetBackwardConn(connID string) (net.Conn, bool) {
	mu.Lock()
	defer mu.Unlock()
	c, ok := backwardConnMap[connID]
	return c, ok
}

func RemoveBackwardConn(connID string) {
	mu.Lock()
	defer mu.Unlock()
	delete(backwardConnMap, connID)
}
func HandleBackwardStart(connID string) {
	if !SetBackwardTargetFromPrefix(connID) {
		log.Printf("[-] No target for BackwardConn %s", connID)
		return
	}
	log.Printf("[admin] preparing to connect target for connID=%s", connID)

	mu.Lock()
	target, ok := backwardTarget[connID]
	mu.Unlock()
	if !ok {
		log.Printf("[-] No target for BackwardConn %s", connID)
		return
	}

	conn, err := net.Dial("tcp", target)
	if err != nil {
		log.Printf("[-] Failed to connect target for BackwardConn %s: %v", connID, err)
		return
	}

	mu.Lock()
	backwardConnMap[connID] = conn
	mu.Unlock()

	log.Printf("[+] BackwardConn %s connected to %s", connID, target)
	RegisterBackwardConn(connID, conn)

	// 启动读取 goroutine
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				break
			}
			payload := BackwardDataPayload{
				ConnID: connID,
				Data:   buf[:n],
			}
			data, _ := json.Marshal(payload)
			msg := Message{Type: MsgBackwardData, Payload: data}
			sendMessageToAgent(connID, msg)
		}
		// 出错关闭
		payload := BackwardStopPayload{ConnID: connID}
		data, _ := json.Marshal(payload)
		msg := Message{Type: MsgBackwardStop, Payload: data}
		sendMessageToAgent(connID, msg)

		conn.Close()
		mu.Lock()
		delete(backwardConnMap, connID)
		delete(backwardTarget, connID)
		mu.Unlock()
		log.Printf("[-] BackwardConn %s closed (read)", connID)
	}()
}

func sendMessageToAgent(connID string, msg Message) {
	mu.Lock()
	agentID, ok := connAgentMap[connID]
	mu.Unlock()
	if !ok {
		log.Printf("[-] Cannot send to agent: unknown connID %s", connID)
		return
	}
	if Registry == nil {
		log.Println("[-] sendMessageToAgent: Registry not initialized")
		return
	}
	err := sendMessageOrRelay(agentID, msg, Registry)
	if err != nil {
		log.Printf("[-] sendMessageToAgent failed: %v", err)
	}
}
func ListBackwardConns() map[string]net.Conn {
	mu.Lock()
	defer mu.Unlock()
	result := make(map[string]net.Conn)
	for k, v := range backwardConnMap {
		result[k] = v
	}
	return result
}
