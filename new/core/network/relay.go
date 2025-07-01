package network

import (
	"encoding/json"
	"io"
	"log"
	"net"

	"github.com/Singert/DockRat/core/common"
	"github.com/Singert/DockRat/core/node"
	"github.com/Singert/DockRat/core/protocol"
)

// -------------------------------中继节点专用监听函数 -------------------------------

// RelayContext 持有本 relay 节点的所有状态
type RelayContext struct {
	SelfID      int
	Registry    *node.Registry
	Topology    *node.NodeGraph
	IDAllocator *common.IDAllocator
	Upstream    net.Conn // 与上级 admin 或 relay 的连接
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

// 接收 agentY 并分配 ID，注册后上报给上级
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
	ctx.Topology.SetParent(n.ID, ctx.SelfID)
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
	buf, _ := protocol.EncodeMessage(msgOut)
	ctx.Upstream.Write(buf)

	// 启动消息读取
	go HandleRelayAgentMessages(n, ctx)
}

// 该函数与 admin 的 handleAgentMessages() 类似，
// 但 relay 不直接处理业务消息，
// 而是将其封装为 RelayPacket 并转发给 ctx.Upstream（admin）。
func HandleRelayAgentMessages(n *node.Node, ctx *RelayContext) {
	conn := n.Conn
	for {
		lengthBuf := make([]byte, 4)
		if _, err := io.ReadFull(conn, lengthBuf); err != nil {
			log.Printf("[Relay] Node %d disconnected: %v", n.ID, err)
			ctx.Registry.Remove(n.ID)
			ctx.Topology.RemoveNode(n.ID)
			ctx.IDAllocator.Free(n.ID)
			conn.Close()
			return
		}
		length := bytesToUint32(lengthBuf)
		data := make([]byte, length)
		if _, err := io.ReadFull(conn, data); err != nil {
			log.Printf("[Relay] Node %d read failed: %v", n.ID, err)
			ctx.Registry.Remove(n.ID)
			ctx.Topology.RemoveNode(n.ID)
			ctx.IDAllocator.Free(n.ID)
			conn.Close()
			return
		}

		// 打包为 RelayPacket 上送
		pkt := protocol.RelayPacket{
			DestID: n.ID,
			Data:   data,
		}
		pktBytes, _ := json.Marshal(pkt)
		msg := protocol.Message{
			Type:    protocol.MsgRelayPacket,
			Payload: pktBytes,
		}
		out, _ := protocol.EncodeMessage(msg)
		ctx.Upstream.Write(out)
	}
}

// 该函数用于 relay 收到一个 RelayPacket 后，向下路由目标 agent。
func HandleRelayPacket(ctx *RelayContext, pkt protocol.RelayPacket) {
	target, ok := ctx.Registry.Get(pkt.DestID)
	if !ok {
		log.Printf("[-] Relay: unknown target ID %d", pkt.DestID)
		return
	}
	_, err := target.Conn.Write(pkt.Data)
	if err != nil {
		log.Printf("[-] Relay: write to node %d failed: %v", pkt.DestID, err)
		ctx.Registry.Remove(pkt.DestID)
		ctx.Topology.RemoveNode(pkt.DestID)
		ctx.IDAllocator.Free(pkt.DestID)
	}
}
