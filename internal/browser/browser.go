// Package browser opens a URL, reusing an existing tab when it can.
package browser

import (
	"os/exec"
	"strings"
)

// `open <url>` always spawns a new tab in Arc, so repeatedly opening the same
// PR buries the window in duplicates. Arc exposes its tabs over AppleScript,
// so look for the URL first and only fall back to `open`.
const findTab = `on run argv
  set target to item 1 of argv
  tell application "Arc"
    repeat with w in windows
      repeat with t in tabs of w
        if URL of t is target then
          tell t to select
          return "FOUND"
        end if
      end repeat
    end repeat
  end tell
  return "NOT_FOUND"
end run`

func Open(url string) error {
	if url == "" {
		return nil
	}
	if isArcDefault() && focusArcTab(url) {
		// `tell t to select` focuses the tab and its space, but setting the
		// window index is unsupported and raises -10000, so activate the app
		// separately.
		_ = exec.Command("osascript", "-e", `tell application "Arc" to activate`).Run()
		return nil
	}
	return exec.Command("open", url).Run()
}

// Only Arc is scripted here; anything else gets the normal behavior.
func isArcDefault() bool {
	out, err := exec.Command("osascript", "-e", `id of application "Arc"`).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "company.thebrowser.Browser"
}

func focusArcTab(url string) bool {
	cmd := exec.Command("osascript", "-", url)
	cmd.Stdin = strings.NewReader(findTab)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "FOUND"
}
