// core/shell/shell.go
package shell

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

type ShellSession struct {
	cmd    *exec.Cmd
	ptmx   *os.File
	reader *bufio.Reader
	mu     sync.Mutex
	shell  string
}

// StartSession 启动一个新的 shell 会话
func StartSession(shell string) (*ShellSession, error) {
	session := &ShellSession{shell: shell}
	err := session.spawn()
	if err != nil {
		return nil, err
	}
	return session, nil
}

// spawn 用于初始化/重启 shell
func (s *ShellSession) spawn() error {
	cmd := exec.Command(s.shell)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	s.cmd = cmd
	s.ptmx = ptmx
	s.reader = bufio.NewReader(ptmx)
	return nil
}

// Exec 向 shell 发送命令，并读取直到特殊行
func (s *ShellSession) Exec(command string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.IsAlive() {
		return "", fmt.Errorf("shell is not running")
	}

	tag := "###END###"
	fullCmd := fmt.Sprintf("%s\necho %s\n", command, tag)
	if _, err := s.ptmx.Write([]byte(fullCmd)); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			break
		}
		if strings.Contains(line, tag) {
			break
		}
		buf.WriteString(line)
	}
	return buf.String(), nil
}

// Kill 优雅关闭 shell
func (s *ShellSession) Kill() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ptmx != nil {
		_ = s.ptmx.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Kill()
	}
	return nil
}

// IsAlive 判断 shell 是否仍在运行
func (s *ShellSession) IsAlive() bool {
	if s.cmd == nil || s.cmd.Process == nil {
		return false
	}
	// 调用系统 syscall 检查进程是否还活着
	err := s.cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}

// Reset 重启 shell 会话
func (s *ShellSession) Reset() error {
	_ = s.Kill()
	return s.spawn()
}
