package node

import (
	"fmt"
	"net"
	"sync"
)

type Node struct {
	ID       int
	ParentID int
	Conn     net.Conn
	Hostname string
	Username string
	OS       string
	Addr     string
}

type Registry struct {
	nodes  map[int]*Node
	mu     sync.Mutex
	nextID int
}

func NewRegistry() *Registry {
	return &Registry{
		nodes: make(map[int]*Node),
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

func (r *Registry) GetChildren(parentID int) []*Node {
	r.mu.Lock()
	defer r.mu.Unlock()

	children := []*Node{}
	for _, node := range r.nodes {
		if node.ParentID == parentID {
			children = append(children, node)
		}
	}
	return children
}

func (n *Node) String() string {
	return fmt.Sprintf("Node[%d] -> IP: %s, Hostname: %s, User: %s, OS: %s",
		n.ID, n.Addr, n.Hostname, n.Username, n.OS)
}

/*
现在你的 NodeRegistry 已具备完整的增、删、查、列能力，下一步你可以轻松支持：

    控制指定节点：通过 registry.Get(id) 发送命令

    实现自动断线剔除：通过 Remove(id) 清除离线节点
*/
