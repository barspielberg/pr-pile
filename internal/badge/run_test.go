package badge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func run(t *testing.T, line string, timeout time.Duration) (Result, time.Duration) {
	t.Helper()
	start := time.Now()
	r := Run(context.Background(), Job{Key: KeyFor("b", "o/r", 1), Line: line, Dir: t.TempDir(), Timeout: timeout})
	return r, time.Since(start)
}

func TestRunReturnsTheCleanedFirstLine(t *testing.T) {
	r, _ := run(t, `printf '\033[1mDone\033[0m\nignored\n'`, 5*time.Second)
	if r.Err != nil || r.Text != "Done" {
		t.Fatalf("result = %+v", r)
	}
}

func TestRunReportsTheLastStderrLine(t *testing.T) {
	r, _ := run(t, `echo noise >&2; echo "no such ticket" >&2; exit 3`, 5*time.Second)
	if r.Err == nil || r.Err.Error() != "no such ticket" {
		t.Fatalf("err = %v", r.Err)
	}
}

func TestRunKillsTheWholeGroupOnTimeout(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "survived")
	r, took := run(t, `(sleep 1; touch `+marker+`) & sleep 30`, 200*time.Millisecond)
	if r.Err == nil || !strings.Contains(r.Err.Error(), "timed out") {
		t.Fatalf("err = %v, want a timeout", r.Err)
	}
	if took > 2*time.Second {
		t.Fatalf("took %v past a 200ms timeout", took)
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a child of the timed-out script kept running")
	}
}

// A child that inherits stdout would hold the pipe open long after the script
// itself answered.
func TestRunDoesNotWaitForABackgroundChild(t *testing.T) {
	r, took := run(t, `sleep 5 & echo ok`, 10*time.Second)
	if r.Err != nil || r.Text != "ok" {
		t.Fatalf("result = %+v", r)
	}
	if took > 2*time.Second {
		t.Fatalf("took %v waiting on a background child", took)
	}
}

func TestRunDrainsOutputPastTheCap(t *testing.T) {
	r, _ := run(t, `head -c 1000000 /dev/zero | tr '\0' x; echo; echo done >&2`, 5*time.Second)
	if r.Err != nil || len(r.Text) != outputCap {
		t.Fatalf("err = %v, text length %d, want %d", r.Err, len(r.Text), outputCap)
	}
}

func TestRunStopsWhenTheContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	start := time.Now()
	r := Run(ctx, Job{Line: "sleep 30", Timeout: time.Minute})
	if r.Err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("err = %v after %v", r.Err, time.Since(start))
	}
}

func TestRunCannotPromptOnTheTerminal(t *testing.T) {
	r, _ := run(t, `if : </dev/tty 2>/dev/null; then echo tty; else echo none; fi`, 5*time.Second)
	if r.Text != "none" {
		t.Fatalf("script reached a terminal: %+v", r)
	}
	r, _ = run(t, `echo "$GIT_TERMINAL_PROMPT$GIT_OPTIONAL_LOCKS"`, 5*time.Second)
	if r.Text != "00" {
		t.Fatalf("git env = %q", r.Text)
	}
}
