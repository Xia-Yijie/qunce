//go:build !windows

package clientapp

import "os/exec"

func configureChildProcess(cmd *exec.Cmd) {
	_ = cmd
}
