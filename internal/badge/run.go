package badge

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Job is one script run for one badge on one PR.
type Job struct {
	Key     Key
	Line    string // rendered, ready for sh -c
	Dir     string
	Timeout time.Duration
}

type Result struct {
	Key  Key
	Text string
	Err  error
}

// outputCap is far more than a badge can draw. Past it the script's output is
// read and thrown away rather than left unread, which would block it, or cut
// off, which would kill it with SIGPIPE before it exits.
const outputCap = 4096

// waitDelay bounds how long Run waits for the script's output to close after
// it exits or is killed. A background child that inherited stdout would
// otherwise hold Run open past the timeout for as long as it lives.
const waitDelay = 500 * time.Millisecond

// Run runs one job to completion. It never touches the terminal: the script
// gets its own session, so ssh, git or gh cannot prompt on /dev/tty over the
// board, and a timeout or a cancelled ctx kills the whole process group, not
// only sh.
func Run(ctx context.Context, job Job) Result {
	ctx, cancel := context.WithTimeout(ctx, job.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", job.Line)
	cmd.Dir = job.Dir
	// GIT_OPTIONAL_LOCKS=0 keeps a git status badge off index.lock, where a
	// run across many rows would make the user's own commit fail.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = waitDelay

	var stdout capped
	stdout.limit = outputCap
	var stderr capped
	stderr.limit = outputCap
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	// WaitDelay reports an error even for a script that answered and exited 0
	// while a child it left behind still held stdout. The answer stands.
	if errors.Is(err, exec.ErrWaitDelay) {
		err = nil
	}
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		return Result{Key: job.Key, Err: errors.New("timed out after " + job.Timeout.String())}
	case ctx.Err() != nil:
		return Result{Key: job.Key, Err: ctx.Err()}
	case err != nil:
		if line := lastLine(stderr.buf.String()); line != "" {
			return Result{Key: job.Key, Err: errors.New(Clean([]byte(line)))}
		}
		return Result{Key: job.Key, Err: err}
	}
	return Result{Key: job.Key, Text: Clean(stdout.buf.Bytes())}
}

func lastLine(s string) string {
	s = strings.TrimRight(s, "\r\n")
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// capped keeps the first limit bytes and accepts the rest unread.
type capped struct {
	buf   bytes.Buffer
	limit int
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.limit - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}
