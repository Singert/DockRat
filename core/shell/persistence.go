// core/shell/persistent.go
package shell

import (
	"fmt"
	"io"
	"sync"
)

var (
	persistentShell     *ShellSession
	persistentShellOnce sync.Once
)

// InitPersistentShell initializes a global shell session
func InitPersistentShell(shellName string) error {
	var err error
	persistentShellOnce.Do(func() {
		persistentShell, err = StartSession(shellName)
	})
	return err
}

// AttachInteractiveSession bridges the current shell to a conn (admin)
func AttachInteractiveSession(conn io.ReadWriter) error {
	if persistentShell == nil || !persistentShell.IsAlive() {
		return fmt.Errorf("no persistent shell running")
	}

	fmt.Fprintln(persistentShell.ptmx, "echo '[*] Attached to shell.'")

	done := make(chan struct{})

	// Admin -> Shell
	go func() {
		_, _ = io.Copy(persistentShell.ptmx, conn)
		done <- struct{}{}
	}()

	// Shell -> Admin
	go func() {
		_, _ = io.Copy(conn, persistentShell.ptmx)
		done <- struct{}{}
	}()

	<-done // 任一方向中断就退出
	return nil
}

// StopPersistentShell cleanly terminates the session
func StopPersistentShell() error {
	if persistentShell != nil {
		return persistentShell.Kill()
	}
	return nil
}
