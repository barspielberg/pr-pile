// Package shelltmpl renders the shell templates that actions run, and passes
// them GitHub's fields only once each value is known to hold no shell syntax.
// It sits apart from the UI so config can refuse a bad template before the
// board opens.
package shelltmpl

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"github.com/barspielberg/pr-pile/internal/github"
	"github.com/charmbracelet/x/ansi"
)

// The GitHub fields are methods, not fields: the template calls them however
// it spells the access ($.Branch, a with block, a define), so the check runs on
// the value itself instead of on the template text. Only a template that uses a
// field can be refused by it. The raw PR sits behind a pointer because fmt
// prints a nested pointer as an address, so {{.}} or printf "%v" cannot dump it.
type Data struct {
	Number         int
	Repo, RepoPath string
	pr             *github.PR
}

// NewData is the data a singular template sees for pr. repo and repoPath come
// from the user's config, not from GitHub.
func NewData(pr github.PR, repo, repoPath string) Data {
	return Data{Number: pr.Number, Repo: repo, RepoPath: repoPath, pr: &pr}
}

func (d Data) Branch() (string, error) { return checkRef("branch", d.pr.HeadRefName) }
func (d Data) Base() (string, error)   { return checkRef("base", d.pr.BaseRefName) }
func (d Data) URL() (string, error)    { return checkURL(d.pr.URL) }
func (d Data) Author() (string, error) { return checkLogin(d.pr.Author) }
func (d Data) Title() (string, error) {
	return "", errTitleRemoved(".Title", "gh pr view {{.Number}} --repo {{.Repo}} --json title --jq .title")
}

// MultiData is what a `multi: true` template sees: each field space-joins one
// checked value per PR.
type MultiData struct {
	Numbers        string
	Repo, RepoPath string
	prs            *[]github.PR
}

func NewMultiData(prs []github.PR, repo, repoPath string) MultiData {
	numbers := make([]string, 0, len(prs))
	for _, pr := range prs {
		numbers = append(numbers, fmt.Sprint(pr.Number))
	}
	return MultiData{Numbers: strings.Join(numbers, " "), Repo: repo, RepoPath: repoPath, prs: &prs}
}

func (d MultiData) Branches() (string, error) {
	return d.each(func(pr github.PR) (string, error) { return checkRef("branch", pr.HeadRefName) })
}

func (d MultiData) Bases() (string, error) {
	return d.each(func(pr github.PR) (string, error) { return checkRef("base", pr.BaseRefName) })
}

func (d MultiData) URLs() (string, error) {
	return d.each(func(pr github.PR) (string, error) { return checkURL(pr.URL) })
}

func (d MultiData) Authors() (string, error) {
	return d.each(func(pr github.PR) (string, error) { return checkLogin(pr.Author) })
}

func (d MultiData) Titles() (string, error) {
	return "", errTitleRemoved(".Titles", "gh pr view on each of {{.Numbers}}")
}

func (d MultiData) each(field func(github.PR) (string, error)) (string, error) {
	values := make([]string, 0, len(*d.prs))
	for _, pr := range *d.prs {
		v, err := field(pr)
		if err != nil {
			return "", err
		}
		values = append(values, v)
	}
	return strings.Join(values, " "), nil
}

// The GitHub fields are written by whoever opened the PR, and the rendered
// line runs through sh -c. Rather than quote them, which only holds where the
// placeholder sits outside any quotes and nothing re-parses the line, each must
// match a pattern with no shell syntax in it. A value that passes is inert in
// any position. The first character is never "-", so none reads as an option.
var (
	safeRef   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/+@,-]*$`)
	safeLogin = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)
	safePRURL = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+/pull/[0-9]+$`)
)

// FieldError is a GitHub value refused by its check, or the removed title.
type FieldError string

func (e FieldError) Error() string { return string(e) }

func checkRef(what, ref string) (string, error) {
	if !safeRef.MatchString(ref) {
		return "", FieldError(fmt.Sprintf("refusing %s name %s: only letters, digits and . _ / + @ , - are passed to actions", what, quoted(ref)))
	}
	return ref, nil
}

func checkLogin(login string) (string, error) {
	if !safeLogin.MatchString(login) {
		return "", FieldError("refusing author " + quoted(login) + ": not a GitHub login")
	}
	return login, nil
}

func checkURL(url string) (string, error) {
	if !safePRURL.MatchString(url) {
		return "", FieldError("refusing URL " + quoted(url) + ": not a GitHub PR URL")
	}
	return url, nil
}

// The title is free text, so no pattern can make it safe to splice into a
// command. A script that needs it can ask gh for it.
func errTitleRemoved(field, instead string) error {
	return FieldError(field + " is no longer available to actions; use " + instead)
}

// quoted makes a refused value safe to show on the board, which draws these
// errors in the footer.
func quoted(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, ansi.Strip(strconv.Quote(s)))
}

// A singular template in a multi action (or the reverse) is an error rather
// than an empty expansion: `{{.URL}}` in a multi action would render nothing at
// all, and an action that silently does the wrong thing is worse than one that
// refuses.
var singularFields = []string{"Number", "Branch", "Base", "URL", "Author", "Title"}
var pluralFields = []string{"Numbers", "Branches", "Bases", "URLs", "Authors", "Titles"}

// RepoBoundFields only mean something inside one repo: #12 or a branch name
// names a different PR, or nothing, in another. URLs, authors and titles do not
// depend on where the command runs.
var RepoBoundFields = []string{"Repo", "RepoPath", "Number", "Numbers", "Branch", "Branches", "Base", "Bases"}

// Check reports what can be known about tmpl before any PR is in hand: it
// parses, and it uses only the singular or only the plural fields. The GitHub
// values are checked when Render calls them.
func Check(tmpl string, multi bool) error {
	_, err := parse(tmpl, multi)
	return err
}

// Render executes tmpl over NewData or NewMultiData.
func Render(tmpl string, multi bool, data any) (string, error) {
	t, err := parse(tmpl, multi)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		// A refused field reads as its own message, not wrapped in the
		// template's "executing action at <.Branch>: error calling Branch:".
		var refused FieldError
		if errors.As(err, &refused) {
			return "", refused
		}
		return "", err
	}
	return b.String(), nil
}

func parse(tmpl string, multi bool) (*template.Template, error) {
	wrong := pluralFields
	if multi {
		wrong = singularFields
	}
	for _, field := range wrong {
		if Mentions(tmpl, field) {
			return nil, fmt.Errorf("field %s does not belong in this action", "."+field)
		}
	}
	return template.New("action").Parse(tmpl)
}

func MentionsAny(tmpl string, fields []string) bool {
	for _, f := range fields {
		if Mentions(tmpl, f) {
			return true
		}
	}
	return false
}

// Mentions reports whether a template uses a given field, matching the whole
// name so .URL does not answer for .URLs.
func Mentions(tmpl, field string) bool {
	for offset := 0; ; {
		rel := strings.Index(tmpl[offset:], "{{")
		if rel < 0 {
			return false
		}
		start := offset + rel
		closeRel := strings.Index(tmpl[start+2:], "}}")
		if closeRel < 0 {
			return false
		}
		end := start + 2 + closeRel
		for _, token := range strings.FieldsFunc(tmpl[start+2:end], func(r rune) bool {
			return r == ' ' || r == '\t' || r == '(' || r == ')' || r == '|'
		}) {
			if token == "."+field {
				return true
			}
		}
		offset = end + 2
	}
}
