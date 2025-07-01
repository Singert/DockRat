// File: core/node/registry.go
package node

import (
	"fmt"
	"net"
	"sync"
)

type Node struct {
	ID       int
	Conn     net.Conn
	Hostname string
	Username string
	OS       string
	Addr     string
}

type Registry struct {
	nodes     map[int]*Node
	mu        sync.Mutex
	nextID    int
	NodeGraph *NodeGraph
}

func NewRegistry() *Registry {
	return &Registry{
		nodes:     make(map[int]*Node),
		NodeGraph: NewNodeGraph(), // 初始化拓扑图
	}
}

func (r *Registry) Add(node *Node) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := r.nextID
	node.ID = id
	r.nodes[id] = node
	r.nextID++
	return id
}

func (r *Registry) List() []*Node {
	r.mu.Lock()
	defer r.mu.Unlock()

	nodes := make([]*Node, 0, len(r.nodes))
	for _, node := range r.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

func (r *Registry) Get(id int) (*Node, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	node, ok := r.nodes[id]
	return node, ok
}

func (r *Registry) Remove(id int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.nodes, id)
}

func (n *Node) String() string {
	return fmt.Sprintf("Node[%d] -> IP: %s, Hostname: %s, User: %s, OS: %s",
		n.ID, n.Addr, n.Hostname, n.Username, n.OS)
}

func (r *Registry) AddWithID(n *Node) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := n.ID
	r.nodes[id] = n
	if id >= r.nextID {
		r.nextID = id + 1
	}
}
