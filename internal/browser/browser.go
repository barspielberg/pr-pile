// Package browser opens a URL in the default browser.
package browser

import "os/exec"

func Open(url string) error {
	if url == "" {
		return nil
	}
	return exec.Command("open", url).Run()
}
