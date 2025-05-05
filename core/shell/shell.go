// core/shell/shell.go
package shell

import (
	"os/exec"
)

func ExecCommand(cmd string) string {
	out, err := exec.Command("bash", "-c", cmd).CombinedOutput()
	if err != nil {
		return err.Error() + ": " + string(out)
	}
	return string(out)
}
