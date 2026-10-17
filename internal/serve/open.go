package serve

import (
	"fmt"
	"os/exec"
	"runtime"
)

func openBrowser(address string) (<-chan error, error) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", address)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	default:
		command = exec.Command("xdg-open", address)
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("browser launcher: %w", err)
	}
	result := make(chan error, 1)
	go func() { result <- command.Wait() }() // Reap without blocking on a browser window.
	return result, nil
}
