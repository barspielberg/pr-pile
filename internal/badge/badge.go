// Package badge runs the scripts behind row badges and keeps their answers.
// Nothing here draws or blocks: the UI asks the cache what to run next, runs
// it off its own goroutine, and hands the result back.
package badge

import (
	"strings"
	"time"
)

// Key names one badge on one PR. The repo is part of it because #12 in one
// repo is a different PR from #12 in another.
type Key struct {
	Badge  string
	Repo   string
	Number int
}

func KeyFor(badge, repo string, number int) Key {
	return Key{Badge: badge, Repo: strings.ToLower(repo), Number: number}
}

// Entry is what the cache knows about one key. Text is the last good answer
// and stays on screen while the entry refreshes or after a failure, so a
// flaky script does not make a badge flicker.
type Entry struct {
	Text    string
	Fetched time.Time // of Text; zero until the first good answer
	Err     error     // of the last run, nil once one succeeds
	Due     time.Time // when to run again; zero means now
	Running bool
}

// A failure retries after its TTL or retryAfter, whichever is sooner, since a
// failed answer is worth replacing sooner than a good one. retryFloor keeps a
// script that fails at once (no sh, a missing checkout) out of a tight loop.
const (
	retryAfter = time.Minute
	retryFloor = 5 * time.Second
)

// Cache holds every entry and counts the scripts in flight. It is not safe for
// concurrent use; the UI owns it and touches it only from Update.
type Cache struct {
	entries  map[Key]*Entry
	workers  int
	inFlight int
}

func NewCache(workers int) *Cache {
	return &Cache{entries: map[Key]*Entry{}, workers: max(workers, 1)}
}

// Get returns the entry for k, if there is one.
func (c *Cache) Get(k Key) (Entry, bool) {
	e, ok := c.entries[k]
	if !ok {
		return Entry{}, false
	}
	return *e, true
}

// Take returns the keys to run now, in order, up to the free worker slots,
// and marks them running. Priority is recomputed on every call from wants
// rather than kept in a queue, so a row that moves up the board moves up the
// line with it.
func (c *Cache) Take(wants []Key, now time.Time) []Key {
	var out []Key
	for _, w := range wants {
		if c.inFlight >= c.workers {
			break
		}
		e := c.entries[w]
		if e == nil {
			e = &Entry{}
			c.entries[w] = e
		}
		if e.Running || now.Before(e.Due) {
			continue
		}
		e.Running = true
		c.inFlight++
		out = append(out, w)
	}
	return out
}

// Done records a result and frees its worker slot. ttl is the badge's, so the
// next due time is known even for a key no row wants any more.
func (c *Cache) Done(r Result, ttl time.Duration, now time.Time) {
	if c.inFlight > 0 {
		c.inFlight--
	}
	e := c.entries[r.Key]
	if e == nil {
		e = &Entry{}
		c.entries[r.Key] = e
	}
	e.Running = false
	if r.Err != nil {
		e.Err = r.Err
		e.Due = now.Add(max(min(ttl, retryAfter), retryFloor))
		return
	}
	e.Text, e.Fetched, e.Err = r.Text, now, nil
	e.Due = now.Add(ttl)
}

// NextDue is the earliest time a key that is not already running falls due,
// for the one timer the UI keeps. ok is false when none is waiting.
func (c *Cache) NextDue(wants []Key) (due time.Time, ok bool) {
	for _, w := range wants {
		e := c.entries[w]
		if e == nil {
			return time.Time{}, true
		}
		if e.Running {
			continue
		}
		if !ok || e.Due.Before(due) {
			due, ok = e.Due, true
		}
	}
	return due, ok
}

// Busy reports whether every worker slot is taken.
func (c *Cache) Busy() bool { return c.inFlight >= c.workers }
