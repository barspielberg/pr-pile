package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// A rule can search several repos, so each PR carries the repo it came from.
func TestSearchCarriesEachPRsRepo(t *testing.T) {
	const body = `{"data":{"search":{"nodes":[
	  {"number":1,"repository":{"nameWithOwner":"o/r"}},
	  {"number":1,"repository":{"nameWithOwner":"o/api"}}
	]}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := &Client{token: "x", http: srv.Client(), endpoint: srv.URL}
	prs, err := c.Search(context.Background(), "q", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 2 || prs[0].Key() != (Key{"o/r", 1}) || prs[1].Key() != (Key{"o/api", 1}) {
		t.Errorf("want o/r#1 and o/api#1, got %+v", prs)
	}
}

// A PR that no longer resolves comes back null beside the others, with an
// error in the same body; the rest of the poll still counts.
func TestWatchParsesEachAliasAndSkipsMissing(t *testing.T) {
	const body = `{"data":{"repository":{
	  "p7":{"number":7,"title":"t","state":"MERGED","reviewDecision":"APPROVED",
	    "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"SUCCESS","contexts":{"nodes":[]}}}}]}},
	  "p8":null
	}},"errors":[{"message":"Could not resolve to a PullRequest with the number of 8."}]}`

	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Query string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		query = in.Query
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := &Client{token: "x", http: srv.Client(), endpoint: srv.URL}
	got, err := c.Watch(context.Background(), "o/r", []int{7, 8})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "p7: pullRequest(number: 7)") || !strings.Contains(query, "p8: pullRequest(number: 8)") {
		t.Errorf("query is missing an alias:\n%s", query)
	}
	w, ok := got[7]
	if !ok || w.State != "MERGED" || w.CIState != "SUCCESS" || w.Review != "APPROVED" {
		t.Errorf("got %+v", got)
	}
	if _, ok := got[8]; ok {
		t.Error("a missing PR should be left out")
	}
}

func TestWatchFailsWhenNothingCameBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"bad credentials"}]}`))
	}))
	defer srv.Close()

	c := &Client{token: "x", http: srv.Client(), endpoint: srv.URL}
	if _, err := c.Watch(context.Background(), "o/r", []int{1}); err == nil {
		t.Error("expected an error")
	}
}

// Every watched PR gone is still an answer, not a failed poll, or the watches
// would stay forever.
func TestWatchSucceedsWhenEveryPRIsMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"repository":{"p8":null}},"errors":[{"type":"NOT_FOUND","path":["repository","p8"],"message":"Could not resolve to a PullRequest with the number of 8."}]}`))
	}))
	defer srv.Close()

	c := &Client{token: "x", http: srv.Client(), endpoint: srv.URL}
	got, err := c.Watch(context.Background(), "o/r", []int{8})
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v", got, err)
	}
}

func TestWatchFailsWhenTheRepoIsMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"repository":null},"errors":[{"type":"NOT_FOUND","path":["repository"],"message":"Could not resolve to a Repository."}]}`))
	}))
	defer srv.Close()

	c := &Client{token: "x", http: srv.Client(), endpoint: srv.URL}
	if _, err := c.Watch(context.Background(), "o/r", []int{1}); err == nil {
		t.Error("a missing repo should fail the poll, not drop every watch")
	}
}
