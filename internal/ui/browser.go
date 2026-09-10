package ui

import (
	"fmt"
	"os/exec"
	"runtime"
)

func OpenBrowser(targetURL string) error {
	name, arguments, err := browserCommand(runtime.GOOS, targetURL)
	if err != nil {
		return err
	}
	if err := exec.Command(name, arguments...).Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}

func browserCommand(goos, targetURL string) (string, []string, error) {
	switch goos {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", targetURL}, nil
	case "darwin":
		return "open", []string{targetURL}, nil
	case "linux":
		return "xdg-open", []string{targetURL}, nil
	default:
		return "", nil, fmt.Errorf("automatic browser opening is unsupported on %s", goos)
	}
}
