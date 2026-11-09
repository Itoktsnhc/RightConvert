//go:build windows

package convert

import (
	"context"
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// command runs a console program without flashing a console window, as
// RightConvert is a GUI app.
func command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}
