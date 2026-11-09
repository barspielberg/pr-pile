package scripts

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

type workflowStep struct {
	Uses string
	With map[string]string
	Env  map[string]string
	Run  string
}

func TestAggregateCheckRequiresTestsPackagingAndDocs(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Jobs map[string]struct {
			Name  string
			If    string
			Needs []string
			Steps []workflowStep
			Uses  string
		}
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	check := config.Jobs["check"]
	if check.Name != "check" || check.If != "${{ always() }}" || strings.Join(check.Needs, ",") != "test,package,docs" {
		t.Fatalf("aggregate must run even on failed/skipped dependencies: %+v", check)
	}
	if len(check.Steps) != 2 || check.Steps[1].Run != "python3 scripts/check-ci.py" || check.Steps[1].Env["CI_NEEDS"] != "${{ toJSON(needs) }}" {
		t.Fatal("aggregate must pass actual dependency results to the tested checker")
	}
	if config.Jobs["docs"].Uses != "./.github/workflows/docs-check.yml" {
		t.Fatal("internal docs validation must gate the aggregate")
	}
}

func goreleaserStep(t *testing.T, workflow, job string) workflowStep {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", workflow))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Jobs map[string]struct{ Steps []workflowStep }
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	for _, step := range config.Jobs[job].Steps {
		if strings.HasPrefix(step.Uses, "goreleaser/goreleaser-action@") {
			return step
		}
	}
	t.Fatalf("no GoReleaser step in %s job %s", workflow, job)
	return workflowStep{}
}

func TestSnapshotInstallsGoReleaserOnPath(t *testing.T) {
	step := goreleaserStep(t, "ci.yml", "package")
	// The action only exports its binary directory to later steps in this mode.
	if step.With["install-only"] != "true" {
		t.Fatal("snapshot CI must use install-only so the Make target can find GoReleaser")
	}
}

func TestReleaseBindsTriggeringTag(t *testing.T) {
	step := goreleaserStep(t, "release.yml", "release")
	if step.Env["GORELEASER_CURRENT_TAG"] != "${{ github.ref_name }}" {
		t.Fatal("release must bind GORELEASER_CURRENT_TAG to the triggering tag")
	}
}

func TestReleaseTagSelection(t *testing.T) {
	goreleaser, err := exec.LookPath("goreleaser")
	if err != nil {
		if os.Getenv("PILE_REQUIRE_GORELEASER") == "1" {
			t.Fatal("packaging regression requires GoReleaser on PATH")
		}
		t.Skip("install GoReleaser to run tag-selection integration coverage")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	fixture := t.TempDir()
	shim := filepath.Join(fixture, "git")
	// Supply Git's default ordering when an RC and stable tag point at HEAD.
	// All other Git operations remain read-only; no real refs or tags are made.
	script := fmt.Sprintf(`#!/bin/sh
case " $* " in
  *" tag --points-at HEAD "*) printf 'v0.1.0-rc.1\nv0.1.0\n'; exit 0 ;;
esac
exec %q "$@"
`, git)
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	binding := goreleaserStep(t, "release.yml", "release").Env["GORELEASER_CURRENT_TAG"]
	for _, tc := range []struct {
		name, tag, want string
	}{
		{"unbound selects RC", "", "v0.1.0-rc.1"},
		{"workflow selects triggering stable", strings.ReplaceAll(binding, "${{ github.ref_name }}", "v0.1.0"), "v0.1.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "dist")
			config := filepath.Join(t.TempDir(), "goreleaser.yaml")
			text := fmt.Sprintf(`version: 2
project_name: pile
dist: %q
builds:
  - main: ./cmd/pile
    binary: pile
    env: [CGO_ENABLED=0]
    goos: [%s]
    goarch: [%s]
    ldflags:
      - '-X main.version={{ .Version }}'
`, out, runtime.GOOS, runtime.GOARCH)
			if err := os.WriteFile(config, []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", fixture+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("GITHUB_REF", "refs/tags/v0.1.0")
			t.Setenv("GORELEASER_CURRENT_TAG", tc.tag)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, goreleaser, "release", "--snapshot", "--skip=archive,changelog", "--config", config)
			cmd.Dir = root
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("snapshot failed: %v\n%s", err, output)
			}
			data, err := os.ReadFile(filepath.Join(out, "metadata.json"))
			if err != nil {
				t.Fatal(err)
			}
			var metadata struct{ Tag string }
			if err := json.Unmarshal(data, &metadata); err != nil {
				t.Fatal(err)
			}
			if metadata.Tag != tc.want {
				t.Fatalf("selected %q, want %q", metadata.Tag, tc.want)
			}
		})
	}
}
