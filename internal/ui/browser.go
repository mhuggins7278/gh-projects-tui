package ui

import (
	"fmt"
	"os/exec"
	"runtime"
)

func openURLInBrowser(target string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", target)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start system browser: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release system browser process: %w", err)
	}
	return nil
}
