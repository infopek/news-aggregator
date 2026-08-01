package platform

import (
	"fmt"
	"os/exec"
	"runtime"
)

type SystemBrowser struct{}

func (SystemBrowser) Open(target string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		command = exec.Command("open", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start browser command: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release browser process: %w", err)
	}
	return nil
}
