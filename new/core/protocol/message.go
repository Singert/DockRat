// file: new/core/protocol/message.go
package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
)

// MessageType 定义了消息类型，如 handshake、shell、upload 等
type MessageType string

const (
	MsgHandshake MessageType = "handshake"
	MsgHeartbeat MessageType = "heartbeat"
	MsgCommand   MessageType = "command"
	MsgResponse  MessageType = "response"
	MsgShell     MessageType = "shell"
	// 拓扑相关消息	「
	MsgListen        MessageType = "listen"
	MsgConnect       MessageType = "connect"
	MsgBindRelayConn MessageType = "bind_relay_conn" // 用于转发连接请求的回复
)

// Message 是基本通信结构
// 结构体经过 JSON 编码后再加上长度前缀发送
type Message struct {
	Type       MessageType // such as handshake,shell,upload
	Payload    []byte      // the data to be sent, such as command or file content
	ToNodeID   int         `json:"to,omitempty"`   // 目标节点ID，,该字段仅由 admin → relay agent 时设置，用于转发给 child 节点
	FromNodeID int         `json:"from,omitempty"` // 源节点ID，通常由 agent → relay agent 时设置
}

type BindRelayConnPayload struct {
	ID int `json:"id"` // 要绑定的目标 Node ID
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
