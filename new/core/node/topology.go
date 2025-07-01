// File: core/node/topology.go
package node

import (
	"fmt"
	"sync"
)

type NodeGraph struct {
	mu        sync.RWMutex
	parentMap map[int]int   // childID → parentID
	childMap  map[int][]int // parentID → []childID
}

func NewNodeGraph() *NodeGraph {
	return &NodeGraph{
		parentMap: make(map[int]int),
		childMap:  make(map[int][]int),
	}
}

// 设置 child 的父节点（会自动从原父节点解绑）
func (g *NodeGraph) SetParent(childID int, parentID int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 如果已有父节点，则从原父节点的子列表中移除
	if oldParent, ok := g.parentMap[childID]; ok {
		children := g.childMap[oldParent]
		newChildren := []int{}
		for _, cid := range children {
			if cid != childID {
				newChildren = append(newChildren, cid)
			}
		}
		g.childMap[oldParent] = newChildren
	}

	// 设置新的父子关系
	g.parentMap[childID] = parentID
	g.childMap[parentID] = append(g.childMap[parentID], childID)
}

// 获取某个节点的所有子节点
func (g *NodeGraph) GetChildren(parentID int) []int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return append([]int{}, g.childMap[parentID]...) // 返回副本防止外部修改
}

// 获取某个子节点的父节点 ID，若无则为 -1
func (g *NodeGraph) GetParent(childID int) int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if pid, ok := g.parentMap[childID]; ok {
		return pid
	}
	return -1
}

// 删除节点及其拓扑关系
func (g *NodeGraph) RemoveNode(id int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 移除父指针
	delete(g.parentMap, id)

	// 移除子节点列表
	delete(g.childMap, id)

	// 从其他人的子列表中删除该节点
	for pid, children := range g.childMap {
		newChildren := []int{}
		for _, cid := range children {
			if cid != id {
				newChildren = append(newChildren, cid)
			}
		}
		g.childMap[pid] = newChildren
	}
}
func (r *Registry) PrintTopology() {
	r.mu.Lock()
	defer r.mu.Unlock()

	fmt.Println("[+] Node Topology:")
	roots := []int{}
	for id := range r.nodes {
		if r.NodeGraph.GetParent(id) == -1 {
			roots = append(roots, id)
		}
	}

	for _, rootID := range roots {
		r.printSubtree(rootID, 0)
	}
}

func (r *Registry) printSubtree(id int, depth int) {
	node := r.nodes[id]
	prefix := ""
	for i := 0; i < depth; i++ {
		prefix += "  "
	}
	fmt.Printf("%s|- Node[%d] %s@%s (%s)\n", prefix, node.ID, node.Username, node.Hostname, node.OS)

	children := r.NodeGraph.GetChildren(id)
	for _, cid := range children {
		r.printSubtree(cid, depth+1)
	}
}
