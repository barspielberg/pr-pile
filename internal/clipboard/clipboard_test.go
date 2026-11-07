package clipboard

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestCopyCommand(t *testing.T) {
	for _, tc := range []struct {
		name, goos, wayland, display string
		tools                        []string
		want                         []string
	}{
		{"macOS", "darwin", "", "", nil, []string{"pbcopy"}},
		{"Wayland preferred", "linux", "wayland-0", ":0", []string{"wl-copy", "xclip", "xsel"}, []string{"wl-copy"}},
		{"X11 xclip", "linux", "", ":0", []string{"wl-copy", "xclip", "xsel"}, []string{"xclip", "-selection", "clipboard"}},
		{"X11 xsel", "linux", "", ":0", []string{"xsel"}, []string{"xsel", "--clipboard", "--input"}},
		{"XWayland fallback", "linux", "wayland-0", ":0", []string{"xclip"}, []string{"xclip", "-selection", "clipboard"}},
		{"missing Wayland tool", "linux", "wayland-0", "", []string{"xclip"}, nil},
		{"missing X11 tools", "linux", "", ":0", nil, nil},
		{"headless", "linux", "", "", []string{"wl-copy", "xclip", "xsel"}, nil},
		{"unsupported", "unsupported", "", "", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PATH", dir)
			t.Setenv("WAYLAND_DISPLAY", tc.wayland)
			t.Setenv("DISPLAY", tc.display)
			for _, name := range tc.tools {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			cmd, err := copyCommand(tc.goos)
			if tc.want == nil {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := append([]string{filepath.Base(cmd.Args[0])}, cmd.Args[1:]...)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("command = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCopyLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux integration")
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("DISPLAY", "")
	out := filepath.Join(dir, "clipboard")
	t.Setenv("PILE_TEST_CLIPBOARD", out)
	path := filepath.Join(dir, "wl-copy")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n/bin/cat > \"$PILE_TEST_CLIPBOARD\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	text := "first PR\nsecond PR — $(literal)\n"
	if err := Copy(text); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil || string(got) != text {
		t.Fatalf("copied %q, error = %v", got, err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Copy(text); err == nil {
		t.Fatal("clipboard failure should return an error")
	}
}
