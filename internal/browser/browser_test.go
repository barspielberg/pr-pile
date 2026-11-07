package browser

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestOpenCommand(t *testing.T) {
	url := "https://github.com/o/r/pull/1?x=a&y=b"
	for _, tc := range []struct{ goos, command string }{
		{"darwin", "open"}, {"linux", "xdg-open"},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			cmd, err := openCommand(tc.goos, url)
			if err != nil || !reflect.DeepEqual(cmd.Args, []string{tc.command, url}) {
				t.Fatalf("command = %v, error = %v", cmd, err)
			}
		})
	}
	if _, err := openCommand("unsupported", url); err == nil {
		t.Fatal("unsupported OS should return an error")
	}
}

func TestOpenLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux integration")
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if err := Open(""); err != nil {
		t.Fatal(err)
	}
	if err := Open("https://github.com"); err == nil {
		t.Fatal("missing xdg-open should return an error")
	}
	out := filepath.Join(dir, "url")
	t.Setenv("PILE_TEST_URL", out)
	script := "#!/bin/sh\nprintf '%s' \"$1\" > \"$PILE_TEST_URL\"\n"
	path := filepath.Join(dir, "xdg-open")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	url := "https://github.com/o/r/pull/1?x=a&y=b"
	if err := Open(url); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil || string(got) != url {
		t.Fatalf("opened %q, error = %v", got, err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Open(url); err == nil {
		t.Fatal("opener failure should return an error")
	}
}
