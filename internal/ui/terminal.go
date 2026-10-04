package ui

import (
	"os"
	"sync"
)

// Terminal is stdout behind one lock, handed to Bubble Tea as its output so a
// notification written from a command cannot land inside a frame: a frame is
// one Write, and the renderer's lock does not cover writers outside it. It
// embeds the *os.File so Bubble Tea still sees a terminal and can size it.
var Terminal = &lockedFile{File: os.Stdout}

type lockedFile struct {
	mu sync.Mutex
	*os.File
}

func (f *lockedFile) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.File.Write(p)
}

// The renderer sends short sequences through io.WriteString, which would
// otherwise reach the embedded file's own WriteString and skip the lock.
func (f *lockedFile) WriteString(s string) (int, error) {
	return f.Write([]byte(s))
}
