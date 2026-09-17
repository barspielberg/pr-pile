package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The bucketing runs against the real context shapes the API returns, captured
// from acme/monorepo: a CheckRun that finished reports a conclusion, one
// still running reports a null conclusion and a status, and a StatusContext
// reports neither and uses state. Getting the second case wrong is what made
// every running check look like it had no result.
func TestSearchBucketsChecksByState(t *testing.T) {
	const body = `{"data":{"search":{"nodes":[{
	  "number":3229,"title":"t","url":"u","isDraft":false,
	  "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"nodes":[
	    {"__typename":"CheckRun","name":"apps_ci / build-push-image webapp","status":"COMPLETED","conclusion":"FAILURE"},
	    {"__typename":"CheckRun","name":"bump-affected","status":"COMPLETED","conclusion":"FAILURE"},
	    {"__typename":"CheckRun","name":"apps_ci / CI Gate","status":"COMPLETED","conclusion":"FAILURE"},
	    {"__typename":"CheckRun","name":"apps_ci / Deploy","status":"COMPLETED","conclusion":"SKIPPED"},
	    {"__typename":"CheckRun","name":"apps_ci / lint-typecheck-test webapp","status":"COMPLETED","conclusion":"SUCCESS"},
	    {"__typename":"CheckRun","name":"Analyze Changed Files","status":"COMPLETED","conclusion":"NEUTRAL"},
	    {"__typename":"CheckRun","name":"e2e-shard","status":"IN_PROGRESS","conclusion":null},
	    {"__typename":"StatusContext","context":"webapp_e2e","state":"PENDING"}
	  ]}}}}]}
	}]}}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := &Client{token: "x", http: srv.Client(), endpoint: srv.URL}
	prs, err := c.Search(context.Background(), "q", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 {
		t.Fatalf("got %d PRs, want 1", len(prs))
	}
	pr := prs[0]

	// "CI Gate" is an umbrella that only restates its children, so it is
	// dropped from every bucket rather than counted in one.
	want := []string{"build-push-image webapp", "bump-affected"}
	if len(pr.FailedGates) != len(want) {
		t.Fatalf("failed gates %v, want %v", pr.FailedGates, want)
	}
	for i, g := range want {
		if pr.FailedGates[i] != g {
			t.Errorf("failed gate %d = %q, want %q", i, pr.FailedGates[i], g)
		}
	}

	// A CheckRun with a null conclusion and an IN_PROGRESS status is running,
	// as is a PENDING StatusContext.
	wantPending := []string{"e2e-shard", "webapp_e2e"}
	if len(pr.PendingGates) != len(wantPending) {
		t.Fatalf("pending gates %v, want %v", pr.PendingGates, wantPending)
	}
	for i, g := range wantPending {
		if pr.PendingGates[i] != g {
			t.Errorf("pending gate %d = %q, want %q", i, pr.PendingGates[i], g)
		}
	}

	// NEUTRAL counts as passing: it does not block the merge.
	if pr.PassedCount != 2 {
		t.Errorf("passed count = %d, want 2", pr.PassedCount)
	}
	if pr.SkippedCount != 1 {
		t.Errorf("skipped count = %d, want 1", pr.SkippedCount)
	}
}
