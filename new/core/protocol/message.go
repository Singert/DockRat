package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/Singert/DockRat/core/node"
)

// MessageType 定义了消息类型，如 handshake、shell、upload 等
type MessageType string

const (
	MsgHandshake MessageType = "handshake"
	MsgHeartbeat MessageType = "heartbeat"
	MsgCommand   MessageType = "command"
	MsgResponse  MessageType = "response"
	MsgShell     MessageType = "shell"

	// Relay
	MsgStartRelay    MessageType = "start_relay"    // Admin → agentX：命令其启动 relay 模式
	MsgRelayReady    MessageType = "relay_ready"    // AgentX → admin：监听启动成功
	MsgRelayRegister MessageType = "relay_register" // AgentX → admin：上报子节点信息
	MsgRelayAck      MessageType = "relay_ack"      // Admin → agentX：注册成功确认
	MsgRelayError    MessageType = "relay_error"    // Admin → agentX：注册失败说明
	MsgRelayPacket   MessageType = "relay_packet"   // 任意层级间透明转发消息
	// Upload
	MsgUpload    MessageType = "upload"     // Admin → Agent：开始上传文件
	MsgFileChunk MessageType = "file_chunk" // Admin → Agent：分片内容
	MsgFileAck   MessageType = "file_ack"   // Agent → Admin：确认收片
	// Download
	MsgDownload     MessageType = "download"      // Admin → Agent：请求下载文件
	MsgDownloadDone MessageType = "download_done" // Agent → Admin：传输完成标志
	// Forwarding / Port Proxy
	MsgForwardStart MessageType = "forward_start" // Admin → Agent：请求发起端口连接
	MsgForwardData  MessageType = "forward_data"  // 双向传输数据（ConnID+Data）
	MsgForwardStop  MessageType = "forward_stop"  // 任一方主动关闭连接
	// Backward
	MsgBackwardStart  MessageType = "backward_start"  // Agent → Admin：有连接接入，请求建立目标连接
	MsgBackwardData   MessageType = "backward_data"   // Agent ↔ Admin：数据透传
	MsgBackwardStop   MessageType = "backward_stop"   // 任意一方断开
	MsgBackwardListen MessageType = "backward_listen" // Admin → Agent：指令其监听端口

)

// Message 是基本通信结构
// 结构体经过 JSON 编码后再加上长度前缀发送
type Message struct {
	Type    MessageType // such as handshake,shell,upload
	Payload []byte      // the data to be sent, such as command or file content
}
type HandshakePayload struct {
	Hostname string `json:"hostname"`
	Username string `json:"username"`
	OS       string `json:"os"`
}

type FileMeta struct {
	Filename string `json:"filename"`
	Path     string `json:"path"` // agent 端保存路径
	Size     int64  `json:"size"`
}
type DownloadRequest struct {
	Path string `json:"path"` // agent 端要下载的路径
}
type FileChunk struct {
	Offset int64  `json:"offset"`
	Data   []byte `json:"data"`
	EOF    bool   `json:"eof"`
}

// 1. 启动 relay 请求（admin → agentX）
type StartRelayPayload struct {
	SelfID     int    `json:"self_id"`     // relay 节点自己的 ID
	ListenAddr string `json:"listen_addr"` // relay 要监听的地址
	IDStart    int    `json:"id_start"`    // 分配给该 relay 的编号段起始
	Count      int    `json:"count"`       // 分配数量（默认1000）

}

// 2. relay 启动成功回报（agentX → admin）
type RelayReadyPayload struct {
	SelfID     int    `json:"self_id"`     // relay 节点自己的 ID
	ListenAddr string `json:"listen_addr"` // 成功监听的地址
}

// 3. relay 向 admin 上报子节点注册请求
type RelayRegisterPayload struct {
	ParentID int       `json:"parent_id"` // relay 的 ID
	Node     node.Node `json:"node"`      // 新子节点信息
}

// 4. 注册结果反馈（admin → relay）
type RelayAckPayload struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"` // 可选信息
}

// ------ forward Payload ------

// 5. 通用转发消息（relay 用于向下或向上传递）
type RelayPacket struct {
	DestID int    `json:"dest_id"` // 最终目标节点 ID
	Data   []byte `json:"data"`    // 原始 Message 的字节流（即 protocol.EncodeMessage(...)）
}

// MsgForwardStart：建立一个远程连接请求
type ForwardStartPayload struct {
	ConnID string `json:"conn_id"` // 唯一连接标识
	Target string `json:"target"`  // 目标地址（如 192.168.1.100:22）
}

// MsgForwardData：数据传输
type ForwardDataPayload struct {
	ConnID string `json:"conn_id"` // 对应连接
	Data   []byte `json:"data"`    // 原始数据
}

// MsgForwardStop：关闭连接
type ForwardStopPayload struct {
	ConnID string `json:"conn_id"` // 要关闭的连接
}

// ------ backward Payload ------
type BackwardListenPayload struct {
	ListenPort int    `json:"listen_port"` // agent 端监听端口
	Target     string `json:"target"`      // admin 本地连接目标，如 127.0.0.1:22
}

type BackwardStartPayload struct {
	ConnID string `json:"conn_id"`
}

type BackwardDataPayload struct {
	ConnID string `json:"conn_id"`
	Data   []byte `json:"data"`
}

type BackwardStopPayload struct {
	ConnID string `json:"conn_id"`
}

// EncodeMessage 将Message编码带长度前缀的字节流
func EncodeMessage(msg Message) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	// 写入长度前缀(大端字节序)
	err = binary.Write(buf, binary.BigEndian, uint32(len(data)))
	if err != nil {
		return nil, err
	}
	// 写入消息内容
	_, err = buf.Write(data)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecodeMessage 从带长度前缀的字节流解码为Message
func DecodeMessage(data []byte) (Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return msg, fmt.Errorf("decode json: %w", err)
	}
	return msg, nil
}

// ReadMessage 从连接中读取一个完整的消息帧（包括长度前缀和内容）
func ReadMessage(reader *bytes.Reader) (Message, error) {
	var length uint32
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
		return Message{}, fmt.Errorf("read length: %w", err)
	}
	msgData := make([]byte, length)
	if _, err := reader.Read(msgData); err != nil {
		return Message{}, fmt.Errorf("read payload: %w", err)
	}
	return DecodeMessage(msgData)

}
