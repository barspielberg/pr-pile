package badge

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func keys(n int) []Key {
	var out []Key
	for i := 1; i <= n; i++ {
		out = append(out, KeyFor("jira", "o/r", i))
	}
	return out
}

func TestTakeFillsFreeSlotsInOrder(t *testing.T) {
	c := NewCache(2)
	got := c.Take(keys(5), t0)
	if len(got) != 2 || got[0].Number != 1 || got[1].Number != 2 {
		t.Fatalf("took %v, want #1 and #2", got)
	}
	if more := c.Take(keys(5), t0); len(more) != 0 {
		t.Fatalf("took %v with every slot busy", more)
	}
	c.Done(Result{Key: got[0], Text: "x"}, time.Minute, t0)
	if next := c.Take(keys(5), t0); len(next) != 1 || next[0].Number != 3 {
		t.Fatalf("after one finished took %v, want #3", next)
	}
}

// Order is read from the call, not remembered, so a row that moved up the
// board since the last call goes first.
func TestTakeFollowsTheCurrentOrder(t *testing.T) {
	c := NewCache(1)
	ks := keys(3)
	if got := c.Take([]Key{ks[2], ks[0], ks[1]}, t0); got[0] != ks[2] {
		t.Fatalf("took %v, want #3 first", got)
	}
}

func TestDoneKeepsTheLastGoodTextOnFailure(t *testing.T) {
	c := NewCache(1)
	k := c.Take(keys(1), t0)[0]
	c.Done(Result{Key: k, Text: "In review"}, 5*time.Minute, t0)
	c.Take(keys(1), t0.Add(5*time.Minute))
	c.Done(Result{Key: k, Err: errors.New("boom")}, 5*time.Minute, t0.Add(5*time.Minute))
	e, _ := c.Get(k)
	if e.Text != "In review" || e.Err == nil || !e.Fetched.Equal(t0) {
		t.Fatalf("entry = %+v, want the old text and the new error", e)
	}
}

func TestTTLAndRetryDecideWhenAKeyRunsAgain(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		ttl  time.Duration
		next time.Duration
	}{
		{"answer waits its ttl", nil, 5 * time.Minute, 5 * time.Minute},
		{"failure retries within a minute", errors.New("x"), 5 * time.Minute, time.Minute},
		{"failure on a short ttl retries at the ttl", errors.New("x"), 20 * time.Second, 20 * time.Second},
		{"failure never retries in a tight loop", errors.New("x"), time.Second, 5 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewCache(1)
			k := c.Take(keys(1), t0)[0]
			c.Done(Result{Key: k, Err: tc.err}, tc.ttl, t0)
			if got := c.Take(keys(1), t0.Add(tc.next-time.Millisecond)); len(got) != 0 {
				t.Fatalf("ran again before %v", tc.next)
			}
			if due, _ := c.NextDue(keys(1)); !due.Equal(t0.Add(tc.next)) {
				t.Fatalf("next due %v, want %v", due.Sub(t0), tc.next)
			}
			if got := c.Take(keys(1), t0.Add(tc.next)); len(got) != 1 {
				t.Fatalf("did not run again at %v", tc.next)
			}
		})
	}
}

func TestNextDueSkipsRunningKeysAndSeesNewOnes(t *testing.T) {
	c := NewCache(1)
	ks := keys(2)
	c.Take(ks[:1], t0)
	if _, ok := c.NextDue(ks[:1]); ok {
		t.Fatal("a running key counted as waiting")
	}
	if due, ok := c.NextDue(ks); !ok || !due.IsZero() {
		t.Fatalf("a key never run is due now, got %v %v", due, ok)
	}
}

func TestKeyIgnoresRepoCase(t *testing.T) {
	if KeyFor("b", "Owner/Repo", 1) != KeyFor("b", "owner/repo", 1) {
		t.Fatal("keys differ by repo case")
	}
}

func TestExpireMakesAnsweredKeysDueAndKeepsTheirText(t *testing.T) {
	c := NewCache(1)
	ks := keys(2)
	c.Take(ks[:1], t0)
	c.Done(Result{Key: ks[0], Text: "old"}, time.Hour, t0)
	c.Take(ks[1:], t0)
	c.Expire(ks)
	if got := c.Take(ks, t0); len(got) != 0 {
		t.Fatalf("took %v with the only slot still busy", got)
	}
	c.Done(Result{Key: ks[1], Text: "x"}, time.Hour, t0)
	c.Expire(ks)
	if got := c.Take(ks, t0); len(got) != 1 || got[0] != ks[0] {
		t.Fatalf("took %v, want #1 again before its hour is up", got)
	}
	if e, _ := c.Get(ks[0]); e.Text != "old" {
		t.Fatalf("text = %q, want the old answer kept while it reruns", e.Text)
	}
}
