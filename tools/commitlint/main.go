// commitlint validates new commit subjects and squash-merge PR titles.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// Keep the grammar shared by the hook and CI. Scopes are optional, lowercase
// identifiers; descriptions may contain any printable Unicode text.
var subjectPattern = regexp.MustCompile(`^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([a-z0-9][a-z0-9._/-]*\))?!?: \S.*$`)

func validate(subject string) error {
	_, description, _ := strings.Cut(subject, ": ")
	if !subjectPattern.MatchString(subject) || strings.TrimSpace(subject) != subject ||
		strings.TrimSpace(description) != description || strings.IndexFunc(subject, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 {
		return fmt.Errorf("use type(scope)!: description (scope and ! are optional); types: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert")
	}
	return nil
}

func run(args []string) error {
	if len(args) != 2 || (args[0] != "--commit-msg" && args[0] != "--event") {
		return fmt.Errorf("usage: commitlint --commit-msg FILE | --event GITHUB_EVENT_PATH")
	}
	data, err := os.ReadFile(args[1])
	if err != nil {
		return err
	}
	var subject string
	if args[0] == "--event" {
		var event struct {
			PullRequest struct {
				Title string `json:"title"`
			} `json:"pull_request"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("read PR event: %w", err)
		}
		subject = event.PullRequest.Title
	} else {
		// Only the subject is constrained; commit bodies and trailers are free.
		subject, _, _ = strings.Cut(string(data), "\n")
		subject = strings.TrimSuffix(subject, "\r")
	}
	// Never interpolate or echo untrusted text into commands or workflow logs.
	return validate(subject)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Conventional Commits:", err)
		os.Exit(1)
	}
}
