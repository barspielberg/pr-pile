// Package browser opens a URL in the default browser.
package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

func Open(url string) error {
	if url == "" {
		return nil
	}
	cmd, err := openCommand(runtime.GOOS, url)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func openCommand(goos, url string) (*exec.Cmd, error) {
	switch goos {
	case "darwin":
		return exec.Command("open", url), nil
	case "linux":
		return exec.Command("xdg-open", url), nil
	default:
		return nil, fmt.Errorf("opening a browser is unsupported on %s", goos)
	}
}
