package ui

import (
	"fmt"
	"github.com/barspielberg/pr-pile/internal/github"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"strconv"
	"strings"
)

// copyField is one thing the `Y` menu can copy, after lazygit's copy menu on
// commits: a letter, a name, and how to read the value off a PR.
type copyField struct {
	key, name, plural string
	value             func(github.PR) string
}

var (
	urlField = copyField{"u", "url", "urls", func(pr github.PR) string { return pr.URL }}

	// The author is the login, not the display name: the login is what you
	// @-mention, and the display name is missing for a third of this board.
	copyFields = []copyField{
		{"n", "number", "numbers", func(pr github.PR) string { return "#" + strconv.Itoa(pr.Number) }},
		{"t", "title", "titles", func(pr github.PR) string { return pr.Title }},
		urlField,
		{"b", "branch", "branches", func(pr github.PR) string { return pr.HeadRefName }},
		{"a", "author", "authors", func(pr github.PR) string {
			if pr.Author == "" {
				return ""
			}
			return "@" + pr.Author
		}},
		{"m", "markdown", "markdown links", func(pr github.PR) string {
			if pr.URL == "" {
				return ""
			}
			return fmt.Sprintf("[#%d %s](%s)", pr.Number, pr.Title, pr.URL)
		}},
	}
)

// fieldValues also returns the number of the PR the first value came from,
// which is not prs[0] when that PR has no value.
func fieldValues(prs []github.PR, f copyField) (values []string, first int) {
	for _, pr := range prs {
		// A PR with no value contributes nothing rather than an empty line, and
		// the reported count follows what was actually copied.
		if v := f.value(pr); v != "" {
			if values == nil {
				first = pr.Number
			}
			values = append(values, v)
		}
	}
	return values, first
}

// copyField copies one field of every PR, one per line: what pastes into a PR
// description, a ticket or a Slack message.
func (m Model) copyField(prs []github.PR, f copyField) (Model, tea.Cmd) {
	values, first := fieldValues(prs, f)
	if len(values) == 0 {
		if len(prs) == 1 {
			return m.setStatus("no " + f.name + " for this PR"), nil
		}
		return m.setStatus("no " + f.plural + " to copy"), nil
	}

	payload := strings.Join(values, "\n")
	text := fmt.Sprintf("copied #%d %s", first, f.name)
	if len(values) > 1 {
		text = fmt.Sprintf("copied %d %s", len(values), f.plural)
	}

	m.statusSeq++
	seq := m.statusSeq
	// The selection is dropped on success only. A failed copy is the one case
	// where the user has to try again, and clearing what they picked would
	// make them pick it a second time.
	cleared := m
	cleared.clearSelection()
	return cleared, func() tea.Msg {
		if err := copyToClipboard(payload); err != nil {
			return asyncStatusMsg{seq: seq, text: "copy failed: " + err.Error(), restore: m.selection}
		}
		return asyncStatusMsg{seq: seq, text: text}
	}
}

// openCopyMenu captures the PRs the menu acts on. The set is held rather than
// re-read on the answer, for the same reason the confirm holds its own.
func (m Model) openCopyMenu() Model {
	prs := m.actionPRs()
	if len(prs) == 0 {
		return m.setStatus("no PR selected")
	}
	m.copyMenu, m.copyCursor = prs, 0
	return m
}

// handleCopyMenuKey follows the help page's rule: the menu's own keys work,
// and every other key closes it, so a guess never leaves you stuck.
func (m Model) handleCopyMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.copyCursor = min(m.copyCursor+1, len(copyFields)-1)
		return m, nil
	case "k", "up":
		m.copyCursor = max(m.copyCursor-1, 0)
		return m, nil
	case "enter":
		return m.pickCopyField(copyFields[m.copyCursor])
	}
	for _, f := range copyFields {
		if f.key == key {
			return m.pickCopyField(f)
		}
	}
	m.copyMenu = nil
	return m, nil
}

func (m Model) pickCopyField(f copyField) (Model, tea.Cmd) {
	prs := m.copyMenu
	m.copyMenu = nil
	return m.copyField(prs, f)
}

// copyMenuBox draws the menu as a bordered box, one row per field, with a
// preview of what the key would copy. With several PRs the preview is the
// first value and a count of the rest.
//
// It is at most rows tall. A short pane gets a window of fields around the
// cursor, and below three rows the borders go too, so the frame never grows
// past the pane and pushes the footer off.
func (m Model) copyMenuBox(rows int) []string {
	prs := m.copyMenu
	title := fmt.Sprintf(" copy #%d ", prs[0].Number)
	if len(prs) > 1 {
		title = fmt.Sprintf(" copy %d PRs ", len(prs))
	}
	hint := " j/k · enter · esc "

	width := min(72, m.width-4)
	inner := width - 2
	nameW := 0
	for _, f := range copyFields {
		nameW = max(nameW, len(f.name))
	}

	border := func(left, label, right string) string {
		fill := max(0, inner-1-lipgloss.Width(label))
		return mutedStyle.Render(left+"─") + headerStyle.Render(label) +
			mutedStyle.Render(strings.Repeat("─", fill)+right)
	}
	shown := min(len(copyFields), max(1, rows-2))
	from := min(max(0, m.copyCursor-shown/2), len(copyFields)-shown)
	var lines []string
	for i := from; i < from+shown; i++ {
		f := copyFields[i]
		values, _ := fieldValues(prs, f)
		preview := "—"
		if len(values) > 0 {
			preview = values[0]
		}
		more := ""
		if len(values) > 1 {
			more = fmt.Sprintf(" (+%d more)", len(values)-1)
		}
		lead := fmt.Sprintf("  %s  %s  ", f.key, pad(f.name, nameW))
		room := max(0, inner-lipgloss.Width(lead)-len(more)-1)
		text := pad(lead+clip(preview, room)+more, inner)

		st := fgStyle
		if len(values) == 0 {
			st = mutedStyle
		}
		if i == m.copyCursor {
			st = st.Background(selBg)
		}
		lines = append(lines, mutedStyle.Render("│")+st.Render(text)+mutedStyle.Render("│"))
	}
	if rows < shown+2 {
		return lines[:min(len(lines), max(0, rows))]
	}
	fill := max(0, inner-1-lipgloss.Width(hint))
	bottom := mutedStyle.Render("╰" + strings.Repeat("─", fill) + hint + "─╯")
	return append(append([]string{border("╭", title, "╮")}, lines...), bottom)
}

// overlayCopyMenu lays the box over the middle of the board. The board stays
// visible around it, so the menu reads as a question about the board rather
// than a page of its own.
func (m Model) overlayCopyMenu(lines []string) []string {
	rows := len(lines)
	if m.height <= 0 {
		// No known height, so nothing to fit: draw the whole box.
		rows = len(copyFields) + 2
		for len(lines) < rows {
			lines = append(lines, "")
		}
	}
	box := m.copyMenuBox(rows)
	if len(box) == 0 {
		return lines
	}
	top := (len(lines) - len(box)) / 2
	left := max(0, (m.width-lipgloss.Width(box[0]))/2)
	boxW := lipgloss.Width(box[0])
	out := append([]string(nil), lines...)
	for i, b := range box {
		under := out[top+i]
		before := pad(ansi.Truncate(under, left, ""), left)
		after := ansi.TruncateLeft(under, left+boxW, "")
		// The reset stops a cut style, like the cursor row's fill, running on
		// into the box.
		out[top+i] = before + ansi.ResetStyle + b + after
	}
	return out
}
