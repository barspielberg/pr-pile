// Package clipboard copies text using the desktop session's clipboard tools.
package clipboard

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func Copy(text string) error {
	cmd, err := copyCommand(runtime.GOOS)
	if err != nil {
		return err
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func copyCommand(goos string) (*exec.Cmd, error) {
	switch goos {
	case "darwin":
		return exec.Command("pbcopy"), nil
	case "linux":
		// Prefer the native Wayland clipboard. XWayland is a fallback when
		// DISPLAY is available and wl-copy is not installed.
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			if path, err := exec.LookPath("wl-copy"); err == nil {
				return exec.Command(path), nil
			}
		}
		if os.Getenv("DISPLAY") != "" {
			if path, err := exec.LookPath("xclip"); err == nil {
				return exec.Command(path, "-selection", "clipboard"), nil
			}
			if path, err := exec.LookPath("xsel"); err == nil {
				return exec.Command(path, "--clipboard", "--input"), nil
			}
		}
		return nil, fmt.Errorf("clipboard unavailable: install wl-copy (wl-clipboard) for Wayland or xclip/xsel for X11; a desktop session (WAYLAND_DISPLAY or DISPLAY) is required")
	default:
		return nil, fmt.Errorf("clipboard is unsupported on %s", goos)
	}
}
