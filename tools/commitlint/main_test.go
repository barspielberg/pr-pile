package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSubjects(t *testing.T) {
	for _, typ := range strings.Fields("feat fix docs style refactor perf test build ci chore revert") {
		for _, suffix := range []string{": add support", "!: change API", "(ui): render board", "(go.mod)!: change API", "(docs/setup): explain setup", "(api-v2): support Unicode 日本語"} {
			if err := validate(typ + suffix); err != nil {
				t.Errorf("%q: %v", typ+suffix, err)
			}
		}
	}
	for _, subject := range []string{
		"", "Add support", "Feat: support", "feature: support", "fix:support",
		"fix: ", "fix:  support", "fix: support ", " fix: support", "fix(): support",
		"fix(UI): support", "fix(two words): support", "fix(scope)!!: support",
		"fix!:support", "fix: support\nchore: second line", "fix: support\r",
		"fix: support\tmore", "fix: support\x00", "fix: support\x1b[31m",
		"fix: \u00a0support", "fix: support\u0085more", "fix: support\u2028more",
	} {
		if err := validate(subject); err == nil {
			t.Errorf("accepted invalid subject %q", subject)
		}
	}
}

func TestInputs(t *testing.T) {
	file := filepath.Join(t.TempDir(), "message with spaces")
	for _, tc := range []struct {
		mode, data string
		valid      bool
	}{
		{"--commit-msg", "fix(ui)!: change layout\n\nFree-form body\nBREAKING CHANGE: layout changed\n", true},
		{"--commit-msg", "docs: explain setup\r\n\r\nBody\r\n", true},
		{"--commit-msg", "Bad subject\n\nfix: valid body cannot rescue subject", false},
		{"--event", `{"pull_request":{"title":"ci: validate titles"}}`, true},
		{"--event", `{"pull_request":{"title":"ci: valid\nfix: extra"}}`, false},
		{"--event", `{"pull_request":{"title":"Bad title"}}`, false},
		{"--event", `{}`, false},
		{"--event", `not JSON`, false},
	} {
		if err := os.WriteFile(file, []byte(tc.data), 0600); err != nil {
			t.Fatal(err)
		}
		if err := run([]string{tc.mode, file}); (err == nil) != tc.valid {
			t.Errorf("%s %q: error=%v, want valid=%v", tc.mode, tc.data, err, tc.valid)
		}
	}
	for _, args := range [][]string{nil, {"--unknown", file}, {"--event", file + ".missing"}} {
		if err := run(args); err == nil {
			t.Errorf("accepted invalid invocation %q", args)
		}
	}
}

func TestWorkflowAndUntrustedTitle(t *testing.T) {
	root := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(root, ".github/workflows/conventional-commits.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		On map[string]struct {
			Types []string `yaml:"types"`
		} `yaml:"on"`
		Permissions map[string]string `yaml:"permissions"`
		Jobs        map[string]struct {
			Name  string `yaml:"name"`
			Steps []struct {
				Uses string            `yaml:"uses"`
				Run  string            `yaml:"run"`
				With map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	if len(workflow.On) != 1 || !reflect.DeepEqual(workflow.On["pull_request"].Types, []string{"opened", "reopened", "synchronize", "edited"}) {
		t.Fatalf("unexpected events: %+v", workflow.On)
	}
	if !reflect.DeepEqual(workflow.Permissions, map[string]string{"contents": "read"}) {
		t.Fatalf("unexpected permissions: %v", workflow.Permissions)
	}
	job, ok := workflow.Jobs["pr-title"]
	if !ok || job.Name != "Conventional PR title" || len(workflow.Jobs) != 1 || len(job.Steps) != 4 {
		t.Fatalf("unexpected jobs: %+v", workflow.Jobs)
	}
	if job.Steps[0].Uses != "actions/checkout@v4" || job.Steps[0].With["persist-credentials"] != "false" ||
		job.Steps[1].Uses != "actions/setup-go@v5" || job.Steps[1].With["go-version-file"] != "go.mod" ||
		job.Steps[2].Run != "go test ./tools/commitlint" {
		t.Fatal("checkout, Go setup or policy regression tests not wired")
	}
	command := job.Steps[3].Run
	if command != `go run ./tools/commitlint --event "$GITHUB_EVENT_PATH"` || strings.Contains(string(data), "${{") {
		t.Fatal("title must come from the event file, never an Actions expression in shell")
	}
	// Execute the actual workflow command with adversarial title data. Both
	// valid and invalid titles must remain inert, including workflow commands.
	dir := t.TempDir()
	marker := filepath.Join(dir, "executed")
	event := filepath.Join(dir, "event with spaces.json")
	for _, prefix := range []string{"fix: ", "invalid "} {
		title := prefix + `$(touch '` + marker + `') ` + "`touch '" + marker + "'`" + ` "; touch '` + marker + `'; # ::error::injected`
		payload, err := json.Marshal(map[string]any{"pull_request": map[string]string{"title": title}})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(event, payload, 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("sh", "-c", command)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GITHUB_EVENT_PATH="+event)
		output, err := cmd.CombinedOutput()
		if (err == nil) != (prefix == "fix: ") || strings.Contains(string(output), "::error::injected") {
			t.Fatalf("untrusted title: error=%v output=%q", err, output)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("title executed shell code: %v", err)
		}
	}
}

func TestHook(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(root, ".githooks", "commit-msg")
	info, err := os.Stat(hook)
	if err != nil || info.Mode()&0111 == 0 {
		t.Fatalf("hook must be executable: %v", err)
	}
	file := filepath.Join(t.TempDir(), "commit message")
	for _, subject := range []string{"fix(ui)!: change layout\n\nBody", "Invalid subject"} {
		if err := os.WriteFile(file, []byte(subject), 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(hook, file)
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		if (err == nil) != strings.HasPrefix(subject, "fix") {
			t.Fatalf("hook: error=%v output=%s", err, output)
		}
	}
}
