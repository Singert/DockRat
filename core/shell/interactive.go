// === /core/shell/interactive.go ===
package shell

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

func StartShellIO(conn io.ReadWriter, shell string) error {
	cmd := exec.Command("bash", "-i")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	// ✅ 设置初始窗口大小：常规 80x24
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: 24,
		Cols: 80,
	})
	if err != nil {
		return err
	}
	defer func() {
		_ = ptmx.Close()
		_ = cmd.Process.Kill()
	}()

	fmt.Fprintln(os.Stderr, "[*] Shell session started.")
	go io.Copy(ptmx, conn)     // admin -> shell
	_, _ = io.Copy(conn, ptmx) // shell -> admin
	return nil
}

// func handleInteractive(conn net.Conn) {
// 	fmt.Println("[*] Switched to interactive shell. Press Ctrl+C to exit.")

// 	done := make(chan struct{})

// 	// admin -> agent (stdin -> conn)
// 	go func() {
// 		_, _ = io.Copy(conn, os.Stdin)
// 		done <- struct{}{}
// 	}()

// 	// agent -> admin (conn -> stdout)
// 	go func() {
// 		_, _ = io.Copy(os.Stdout, conn)
// 		done <- struct{}{}
// 	}()

// 	<-done // wait for either direction to close
// }
