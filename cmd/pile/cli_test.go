package main

import (
	"bytes"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func TestInformationalFlagsDoNotStartBoard(t *testing.T) {
	// A startup attempt would fail: no gh, no config, and no terminal needed.
	t.Setenv("PATH", t.TempDir())
	t.Setenv("PILE_CONFIG", filepath.Join(t.TempDir(), "missing.yml"))
	for _, tc := range []struct {
		arg, want string
	}{
		{"--help", "Usage: pile"},
		{"-h", "Usage: pile"},
		{"--version", "pile "},
	} {
		t.Run(tc.arg, func(t *testing.T) {
			var out bytes.Buffer
			if err := runCLI([]string{tc.arg}, &out); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("output = %q, want %q", out.String(), tc.want)
			}
		})
	}
}

func TestInvalidArgumentsDoNotStartBoard(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--unknown"}, "flag provided but not defined"},
		{[]string{"repo"}, "unexpected argument"},
		{[]string{"--version", "repo"}, "unexpected argument"},
	} {
		var out bytes.Buffer
		if err := runCLI(tc.args, &out); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("arguments %v: error = %v, want %q", tc.args, err, tc.want)
		}
		if out.Len() != 0 {
			t.Fatalf("unexpected stdout: %q", out.String())
		}
	}
}

func TestVersionMetadata(t *testing.T) {
	originalVersion, originalCommit, originalDate := version, commit, date
	t.Cleanup(func() { version, commit, date = originalVersion, originalCommit, originalDate })
	info := &debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "1234567890abcdef"},
			{Key: "vcs.modified", Value: "true"},
		},
	}
	version, commit, date = "dev", "", ""
	if got := versionString(nil); got != "pile dev" {
		t.Fatal(got)
	}
	if got := versionString(info); got != "pile 0.1.0 (commit 1234567890ab-dirty)" {
		t.Fatal(got)
	}
	version, commit, date = "0.2.0", "abcdef0", "2026-11-07T12:00:00Z"
	if got := versionString(info); got != "pile 0.2.0 (commit abcdef0, built 2026-11-07T12:00:00Z)" {
		t.Fatal(got)
	}
}
