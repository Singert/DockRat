// file: new/core/network/utils.go
package network

import (
	"encoding/json"
	"log"

	"github.com/Singert/DockRat/core/protocol"
)

// RelayUpward 向上报告消息给 admin 或上级 relay
func RelayUpward(ctx *RelayContext, msg protocol.Message) {
	log.Printf("[RelayUpward] type=%s len=%d", msg.Type, len(msg.Payload))

	inner, _ := protocol.EncodeMessage(msg) // 使用正确的带前缀格式
	pkt := protocol.RelayPacket{
		DestID: -1,
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

func bytesToUint32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
