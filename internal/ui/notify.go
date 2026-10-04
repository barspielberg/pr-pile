package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type notifyFailedMsg struct{ err error }

// sendNotice delivers one notice. A var so tests can watch what would be sent
// without popping anything on the developer's screen.
var sendNotice = func(how, command string, n notice) error {
	title := "pile: " + n.text
	switch how {
	case "osc":
		// Ghostty reads a body starting with digits and ';' as a ConEmu
		// sequence, which "pile: " rules out.
		_, err := fmt.Fprintf(Terminal, "\x1b]9;%s — %s\x1b\\", title, n.title)
		return err
	case "command":
		cmd := exec.Command("sh", "-c", command)
		cmd.Env = append(os.Environ(),
			"PILE_TITLE="+title, "PILE_MESSAGE="+n.title,
			"PILE_URL="+n.url, fmt.Sprintf("PILE_NUMBER=%d", n.number))
		if out, err := cmd.CombinedOutput(); err != nil {
			if last := lastLine(string(out)); last != "" {
				return fmt.Errorf("%s", last)
			}
			return err
		}
	}
	return nil
}

// A multiplexer may not pass OSC 9 through to the outer terminal; the way out
// is notify: command with the multiplexer's own notifier, not a mode per tool.
func (m Model) notify(news []notice) tea.Cmd {
	how := m.cfg.Watch.Notify
	if how == "" {
		how = "osc"
	}
	if how == "none" || len(news) == 0 {
		return nil
	}
	command := m.cfg.Watch.Command
	return func() tea.Msg {
		// One notice failing must not swallow the others from the same poll.
		var first error
		for _, n := range news {
			if err := sendNotice(how, command, n); err != nil && first == nil {
				first = err
			}
		}
		if first != nil {
			return notifyFailedMsg{err: first}
		}
		return nil
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return terminalText(strings.TrimSpace(lines[len(lines)-1]))
}
