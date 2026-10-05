package github

import (
	"context"
	"encoding/json"
	"fmt"
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

func searchOne(t *testing.T, contexts string) PR {
	t.Helper()
	return searchOnePage(t, false, contexts)
}

func searchOnePage(t *testing.T, hasNextPage bool, contexts string) PR {
	t.Helper()
	body := fmt.Sprintf(`{"data":{"search":{"nodes":[{"number":1,"title":"t","url":"u",
	  "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{
	    "pageInfo":{"hasNextPage":%t},"nodes":[%s]}}}}]}}]}}}`, hasNextPage, contexts)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := &Client{token: "x", http: srv.Client(), endpoint: srv.URL}
	prs, err := c.Search(context.Background(), "q", 10)
	if err != nil {
		t.Fatal(err)
	}
	return prs[0]
}

// Captured from Autofleet/api-gateway-ms#1020: a second workflow run started 4s
// after the first and cancelled it. GitHub's PR page is green and its rollup
// still says FAILURE, so only the newest run of each check may count.
func TestSearchKeepsOnlyTheNewestRunOfEachCheck(t *testing.T) {
	pr := searchOne(t, `
	  {"__typename":"CheckRun","name":"Build","status":"COMPLETED","conclusion":"CANCELLED","startedAt":"2026-10-05T08:25:37Z"},
	  {"__typename":"CheckRun","name":"Deploy","status":"COMPLETED","conclusion":"CANCELLED","startedAt":"2026-10-05T08:25:38Z"},
	  {"__typename":"CheckRun","name":"Build","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-10-05T08:25:42Z"},
	  {"__typename":"CheckRun","name":"Deploy","status":"COMPLETED","conclusion":"SKIPPED","startedAt":"2026-10-05T08:28:45Z"},
	  {"__typename":"CheckRun","name":"check-e2e-label","status":"COMPLETED","conclusion":"FAILURE","startedAt":"2026-10-05T08:50:43Z"},
	  {"__typename":"CheckRun","name":"check-e2e-label","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-10-05T08:56:19Z"}`)

	if pr.CIState != "SUCCESS" {
		t.Errorf("CIState = %q, want SUCCESS", pr.CIState)
	}
	if len(pr.FailedGates)+len(pr.CancelledGates) != 0 {
		t.Errorf("superseded runs still listed: failed %v, cancelled %v", pr.FailedGates, pr.CancelledGates)
	}
	if pr.PassedCount != 2 || pr.SkippedCount != 1 {
		t.Errorf("passed %d skipped %d, want 2 and 1", pr.PassedCount, pr.SkippedCount)
	}
}

// Newest by startedAt, not by position: the API does not order runs by time.
// A queued run has no startedAt yet and is the newest of all.
func TestSearchPicksTheNewestRunByStartTime(t *testing.T) {
	pr := searchOne(t, `
	  {"__typename":"CheckRun","name":"run-e2e","status":"COMPLETED","conclusion":"FAILURE","startedAt":"2026-10-05T13:28:30Z"},
	  {"__typename":"CheckRun","name":"run-e2e","status":"COMPLETED","conclusion":"SKIPPED","startedAt":"2026-10-05T08:50:50Z"},
	  {"__typename":"CheckRun","name":"lint","status":"COMPLETED","conclusion":"FAILURE","startedAt":"2026-10-05T08:00:00Z"},
	  {"__typename":"CheckRun","name":"lint","status":"QUEUED","conclusion":null,"startedAt":null}`)

	if len(pr.FailedGates) != 1 || pr.FailedGates[0] != "run-e2e" {
		t.Errorf("failed gates %v, want [run-e2e]", pr.FailedGates)
	}
	if len(pr.PendingGates) != 1 || pr.PendingGates[0] != "lint" {
		t.Errorf("pending gates %v, want [lint]", pr.PendingGates)
	}
	if pr.SkippedCount != 0 {
		t.Errorf("skipped %d, want 0", pr.SkippedCount)
	}
}

// Two workflows with a job of the same name are two checks, so one passing
// later must not hide the other failing.
func TestSearchKeepsSameNamedJobsFromDifferentWorkflowsApart(t *testing.T) {
	pr := searchOne(t, `
	  {"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"FAILURE","startedAt":"2026-10-05T10:00:00Z",
	   "checkSuite":{"workflowRun":{"workflow":{"name":"CI"}}}},
	  {"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-10-05T10:01:00Z",
	   "checkSuite":{"workflowRun":{"workflow":{"name":"E2E"}}}}`)

	if pr.CIState != "FAILURE" || len(pr.FailedGates) != 1 {
		t.Errorf("CIState %q, failed %v: a failure was hidden by another workflow's job", pr.CIState, pr.FailedGates)
	}
}

// Only a run that has not finished is newest for lacking a startedAt. One
// cancelled while still queued has none either, and must not outrank a real run.
func TestSearchDoesNotTreatAFinishedRunWithoutAStartAsNewest(t *testing.T) {
	pr := searchOne(t, `
	  {"__typename":"CheckRun","name":"Build","status":"COMPLETED","conclusion":"CANCELLED","startedAt":null},
	  {"__typename":"CheckRun","name":"Build","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-10-05T08:25:42Z"},
	  {"__typename":"CheckRun","name":"Test","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-10-05T08:25:42Z"},
	  {"__typename":"CheckRun","name":"Test","status":"COMPLETED","conclusion":"CANCELLED","startedAt":null}`)

	if pr.CIState != "SUCCESS" || len(pr.CancelledGates) != 0 {
		t.Errorf("CIState %q, cancelled %v, want SUCCESS and none", pr.CIState, pr.CancelledGates)
	}
}

// With more contexts than one page, an unseen run may be the newest or the
// failing one, so the state falls back to the rollup, which covers them all.
func TestSearchTrustsTheRollupWhenContextsAreTruncated(t *testing.T) {
	pr := searchOnePage(t, true, `
	  {"__typename":"CheckRun","name":"Build","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-10-05T08:25:42Z"}`)

	if pr.CIState != "FAILURE" {
		t.Errorf("CIState = %q, want the rollup's FAILURE", pr.CIState)
	}
}

// A cancel with no newer run is still the check's current state: it blocks
// the merge, so it is listed, but it is not a failure.
func TestSearchListsACancelledLatestRun(t *testing.T) {
	pr := searchOne(t, `
	  {"__typename":"CheckRun","name":"Build","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-10-05T08:25:37Z"},
	  {"__typename":"CheckRun","name":"Test","status":"COMPLETED","conclusion":"CANCELLED","startedAt":"2026-10-05T08:25:38Z"}`)

	if pr.CIState != "CANCELLED" {
		t.Errorf("CIState = %q, want CANCELLED", pr.CIState)
	}
	if len(pr.CancelledGates) != 1 || pr.CancelledGates[0] != "Test" {
		t.Errorf("cancelled gates %v, want [Test]", pr.CancelledGates)
	}
	if len(pr.FailedGates) != 0 {
		t.Errorf("a cancel was listed as a failure: %v", pr.FailedGates)
	}
}

// isRequired needs a PR number, so a search cannot ask for it and a second
// request does, for the PRs with something failing. A green PR costs nothing.
func TestSearchAsksWhichFailingChecksAreRequired(t *testing.T) {
	const search = `{"data":{"search":{"nodes":[
	  {"number":7,"repository":{"nameWithOwner":"o/r"},
	   "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"nodes":[
	    {"__typename":"CheckRun","name":"run e2e / run-e2e","status":"COMPLETED","conclusion":"FAILURE"},
	    {"__typename":"CheckRun","name":"claude-review","status":"COMPLETED","conclusion":"FAILURE"},
	    {"__typename":"CheckRun","name":"Build","status":"COMPLETED","conclusion":"SUCCESS"}]}}}}]}},
	  {"number":8,"repository":{"nameWithOwner":"o/r"},
	   "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"SUCCESS","contexts":{"nodes":[
	    {"__typename":"CheckRun","name":"Build","status":"COMPLETED","conclusion":"SUCCESS"}]}}}}]}}]}}}`
	const required = `{"data":{"r0":{"p7":{"number":7,
	   "commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
	    {"__typename":"CheckRun","name":"run e2e / run-e2e","isRequired":true},
	    {"__typename":"CheckRun","name":"claude-review","isRequired":false},
	    {"__typename":"CheckRun","name":"Build","isRequired":true}]}}}}]}}}}}`

	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Query string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		if strings.Contains(req.Query, "isRequired") {
			asked = req.Query
			_, _ = w.Write([]byte(required))
			return
		}
		_, _ = w.Write([]byte(search))
	}))
	defer srv.Close()
	c := &Client{token: "x", http: srv.Client(), endpoint: srv.URL}
	prs, err := c.Search(context.Background(), "q", 10)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(asked, "pullRequest(number: 7)") || strings.Contains(asked, "pullRequest(number: 8)") {
		t.Errorf("asked about the wrong PRs:\n%s", asked)
	}
	red := prs[0]
	if got := red.RequiredFailures(); len(got) != 1 || got[0] != "run-e2e" {
		t.Errorf("required failures %v, want [run-e2e]", got)
	}
	if !red.IsOptional("claude-review") || red.OnlyOptionalFailing() {
		t.Errorf("claude-review optional %v, only optional %v", red.IsOptional("claude-review"), red.OnlyOptionalFailing())
	}
	if prs[1].Required != nil {
		t.Errorf("a green PR was asked about: %v", prs[1].Required)
	}
}

// searchWithRequired answers the board search with search and the isRequired
// follow-up with required.
func searchWithRequired(t *testing.T, search, required string) []PR {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Query string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		if strings.Contains(req.Query, "isRequired") {
			_, _ = w.Write([]byte(required))
			return
		}
		_, _ = w.Write([]byte(search))
	}))
	defer srv.Close()
	c := &Client{token: "x", http: srv.Client(), endpoint: srv.URL}
	prs, err := c.Search(context.Background(), "q", 10)
	if err != nil {
		t.Fatal(err)
	}
	return prs
}

// Captured from Autofleet/autorepo#3429: the only required check is the CI Gate
// umbrella, which pile never names, so its failing child looked optional and
// the row went grey on a PR GitHub would not let merge. The umbrella only
// speaks for its own workflow, so another workflow's optional check stays so.
func TestSearchTreatsFailuresUnderARequiredUmbrellaAsRequired(t *testing.T) {
	prs := searchWithRequired(t, `{"data":{"search":{"nodes":[
	  {"number":3429,"repository":{"nameWithOwner":"o/r"},
	   "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"nodes":[
	    {"__typename":"CheckRun","name":"call_apps_ci / CI Gate","status":"COMPLETED","conclusion":"FAILURE"},
	    {"__typename":"CheckRun","name":"call_apps_ci / lint-typecheck-test control-center","status":"COMPLETED","conclusion":"FAILURE"},
	    {"__typename":"CheckRun","name":"claude-review","status":"COMPLETED","conclusion":"FAILURE"}]}}}}]}}]}}}`,
		`{"data":{"r0":{"p3429":{"number":3429,
	   "commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
	    {"__typename":"CheckRun","name":"call_apps_ci / CI Gate","isRequired":true},
	    {"__typename":"CheckRun","name":"call_apps_ci / lint-typecheck-test control-center","isRequired":false},
	    {"__typename":"CheckRun","name":"claude-review","isRequired":false}]}}}}]}}}}}`)

	got := prs[0].RequiredFailures()
	if len(got) != 1 || got[0] != "lint-typecheck-test control-center" {
		t.Errorf("required failures %v, want only the umbrella's own child", got)
	}
}

// Past one page of contexts a required failure may be unseen, so no failure is
// called optional and the follow-up is not even asked.
func TestSearchDoesNotCallFailuresOptionalWhenContextsAreTruncated(t *testing.T) {
	prs := searchWithRequired(t, `{"data":{"search":{"nodes":[
	  {"number":7,"repository":{"nameWithOwner":"o/r"},
	   "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{
	    "pageInfo":{"hasNextPage":true},"nodes":[
	    {"__typename":"CheckRun","name":"claude-review","status":"COMPLETED","conclusion":"FAILURE"}]}}}}]}}]}}}`,
		`{"data":{"r0":{"p7":{"number":7,
	   "commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
	    {"__typename":"CheckRun","name":"claude-review","isRequired":false}]}}}}]}}}}}`)

	if prs[0].Required != nil || prs[0].OnlyOptionalFailing() {
		t.Errorf("required %v: a truncated PR must not have optional failures", prs[0].Required)
	}
}

// The second request is best effort: when it fails the board must look
// exactly as it did before it existed, every failure blocking.
func TestSearchTreatsEveryCheckAsRequiredWhenTheSecondRequestFails(t *testing.T) {
	pr := searchOne(t, `
	  {"__typename":"CheckRun","name":"claude-review","status":"COMPLETED","conclusion":"FAILURE","startedAt":"2026-10-05T08:25:42Z"}`)

	if pr.Required != nil || pr.IsOptional("claude-review") || len(pr.RequiredFailures()) != 1 {
		t.Errorf("required %v, failures %v: an unknown answer must read as required", pr.Required, pr.RequiredFailures())
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

// A fork's branch is not in the base repo, so compare needs owner:branch.
func TestSearchCarriesAForksHeadOwner(t *testing.T) {
	const body = `{"data":{"search":{"nodes":[
	  {"number":1,"headRefName":"feat","isCrossRepository":true,"headRepositoryOwner":{"login":"fork"},"repository":{"nameWithOwner":"o/r"}},
	  {"number":2,"headRefName":"fix","isCrossRepository":false,"headRepositoryOwner":{"login":"o"},"repository":{"nameWithOwner":"o/r"}}
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
	if got := prs[0].CompareRef(); got != "fork:feat" {
		t.Errorf("fork: got %q, want fork:feat", got)
	}
	if got := prs[1].CompareRef(); got != "fix" {
		t.Errorf("same repo: got %q, want fix", got)
	}
}
