// core/node/manager.go
// Package node provides the NodeManager struct and its methods for managing nodes.

package node

import (
	"net"
	"sync"
)

type NodeManager struct {
	mu    sync.Mutex
	nodes map[int]net.Conn
	next  int
}

func NewNoddManager() *NodeManager {
	return &NodeManager{
		nodes: make(map[int]net.Conn),
		next:  0,
	}
}

func (nm *NodeManager) AddNode(conn net.Conn) int {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	nodeID := nm.next
	nm.nodes[nodeID] = conn
	nm.next++

	return nodeID
}

func (nm *NodeManager) Get(nodeID int) (net.Conn, bool) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	conn, ok := nm.nodes[nodeID]
	return conn, ok
}

func (nm *NodeManager) Remove(nodeID int) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	delete(nm.nodes, nodeID)
}
