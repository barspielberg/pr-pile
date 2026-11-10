package badge

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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

func TestRunKillsTheWholeGroupOnTimeout(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	r, took := run(t, `sh -c 'echo $$ > `+pidFile+`; exec sleep 30' & sleep 30`, 300*time.Millisecond)
	if r.Err == nil || !strings.Contains(r.Err.Error(), "timed out") {
		t.Fatalf("err = %v, want a timeout", r.Err)
	}
	if took > 2*time.Second {
		t.Fatalf("took %v past a 300ms timeout", took)
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	// Orphaned, the child is reaped by init, and answers signal 0 until then.
	for deadline := time.Now().Add(2 * time.Second); syscall.Kill(pid, 0) == nil; {
		if time.Now().After(deadline) {
			t.Fatalf("background child %d outlived the timed-out script", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Stop has to kill before it returns: pile exits right after, and a kill left
// to exec's own goroutine never runs.
func TestRunnerStopKillsBeforeReturning(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	r := NewRunner()
	done := make(chan Result, 1)
	go func() { done <- r.Run(Job{Line: `echo $$ > ` + pidFile + `; exec sleep 30`, Timeout: time.Minute}) }()
	var pid int
	for deadline := time.Now().Add(2 * time.Second); pid == 0; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("script never started")
		}
		b, _ := os.ReadFile(pidFile)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	// Only the kill Stop makes itself counts: the one exec makes on its own
	// goroutine is the one that loses the race with exit.
	r.cancel = func() {}
	r.Stop()
	// Straight after Stop, before Run has reaped it: dead or a zombie.
	out, _ := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if stat := strings.TrimSpace(string(out)); stat != "" && !strings.HasPrefix(stat, "Z") {
		t.Fatalf("script is %q after Stop returned, want it dead", stat)
	}
	if res := <-done; res.Err == nil {
		t.Fatalf("result = %+v, want an error", res)
	}
}
