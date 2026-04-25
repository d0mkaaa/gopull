package tui

import (
	"os/exec"
	"runtime"
	"strings"
)

func copyToClipboard(s string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("clip")
	case "darwin":
		cmd = exec.Command("pbcopy")
	default:
		cmd = exec.Command("xclip", "-selection", "clipboard")
	}
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run()
}
