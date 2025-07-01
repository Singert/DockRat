
package main

import (
	"log"

	"github.com/Singert/DockRat/core/network"
	"github.com/Singert/DockRat/core/protocol"
	"github.com/Singert/DockRat/core/node"
)

var registry = node.NewRegistry()

func main() {
	log.Println("[+] Admin starting...")

	// 启动控制台命令处理
	go protocol.StartConsole(registry)

	// 启动监听并处理 Agent 连接
	network.StartListener(":9999", registry)
}


/*
响应消息（如 MsgResponse）缓存起来，以便在控制台中输出最近一条响应？
你也可以将 MsgShell 输出定向到带颜色或带提示的终端 UI，后续支持退出、上传等命令扩展。
如需进一步支持 shell 会话保持、窗口调整、或 stdout 缓存，
*/
