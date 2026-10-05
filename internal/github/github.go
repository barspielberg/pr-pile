// Package github fetches PRs from the GraphQL API.
//
// Not via the gh CLI: `gh search prs` cannot return statusCheckRollup,
// reviewDecision or mergeable (most of the board), `gh pr list` cannot express
// review-requested:/team-review-requested: queries, and shelling out costs
// ~1.7s of process startup per call. One GraphQL search per rule returns
// everything in one round trip.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

const endpoint = "https://api.github.com/graphql"

type Client struct {
	token string
	http  *http.Client

	// endpoint is a field only so tests can point it at a stub; production
	// always uses the const above.
	endpoint string
}

func (c *Client) url() string {
	if c.endpoint != "" {
		return c.endpoint
	}
	return endpoint
}

// Token comes from `gh auth token` so we inherit the user's existing login
// rather than asking them to provision one.
func New() (*Client, error) {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return nil, fmt.Errorf("gh auth token: %w (is gh installed and logged in?)", err)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return nil, fmt.Errorf("gh auth token returned nothing; run `gh auth login`")
	}
	return &Client{token: token, http: &http.Client{Timeout: 30 * time.Second}}, nil
}

// Key identifies a PR on a board that can span repos, where numbers repeat.
type Key struct {
	Repo   string
	Number int
}

type PR struct {
	Repo        string // owner/name
	Number      int
	Title       string
	URL         string
	Author      string // login; the identifier you actually @-mention
	AuthorName  string // display name, null for 36% of this board: see docs/pr-detail.md §5.2
	IsDraft     bool
	Review      string // APPROVED | CHANGES_REQUESTED | REVIEW_REQUIRED | ""
	Mergeable   string // MERGEABLE | CONFLICTING | UNKNOWN
	UpdatedAt   time.Time
	CreatedAt   time.Time
	HeadRefName string
	// HeadOwner is the fork's owner for a PR from a fork, and empty otherwise.
	HeadOwner      string
	BaseRefName    string
	Additions      int
	Deletions      int
	ChangedFiles   int
	CIState        string   // SUCCESS | FAILURE | PENDING | CANCELLED | "" (none)
	FailedGates    []string // names of failing checks, deduped
	PendingGates   []string // names of still-running checks, deduped
	CancelledGates []string // names of checks whose latest run was cancelled, deduped
	PassedCount    int      // passing checks are counted, not named: see docs/checks-page.md

	// Skipped is the largest bucket on this board (47% of contexts) and means
	// a job's path filter did not match, which is a fact about the workflow
	// rather than about the PR. Counted so the overlay can reconcile its
	// total against GitHub's, never listed.
	SkippedCount int

	// Required holds the gate names branch protection requires, fetched only
	// for PRs with a failing or cancelled check. Nil means unknown, and then
	// every check is treated as required, which is what the board did before.
	Required map[string]bool

	// openUmbrellas are the raw names of umbrella gates that have not passed.
	// They are never named, but if one is required it blocks the merge on
	// behalf of every failure under it.
	openUmbrellas []string
	// rawProblems are the raw names behind FailedGates and CancelledGates,
	// which keep the workflow prefix an umbrella's children share.
	rawProblems []string
	// truncated is set when the contexts ran past one page, so a required
	// failure may be unseen and no failure can safely be called optional.
	truncated bool
}

func (p PR) Key() Key { return Key{Repo: p.Repo, Number: p.Number} }

// IsOptional is true only when the PR is known not to need the gate to merge.
func (p PR) IsOptional(gate string) bool {
	return p.Required != nil && !p.Required[gate]
}

func (p PR) RequiredFailures() []string { return p.required(p.FailedGates) }

func (p PR) RequiredCancels() []string { return p.required(p.CancelledGates) }

func (p PR) required(gates []string) []string {
	var out []string
	for _, g := range gates {
		if !p.IsOptional(g) {
			out = append(out, g)
		}
	}
	return out
}

// OnlyOptionalFailing is a PR whose named failures can all be merged past.
func (p PR) OnlyOptionalFailing() bool {
	return len(p.FailedGates) > 0 && len(p.RequiredFailures()) == 0
}

// CompareRef names the PR's head for a compare against its base. A fork's
// branch is not in the base repo, so it needs the owner:branch form.
func (p PR) CompareRef() string {
	if p.HeadOwner != "" {
		return p.HeadOwner + ":" + p.HeadRefName
	}
	return p.HeadRefName
}

// prFields is everything a row draws. The watch poll asks for the same fields
// so a watched PR parses into the same PR the board holds.
const prFields = `
        number title url isDraft reviewDecision mergeable updatedAt createdAt
        headRefName baseRefName
        isCrossRepository headRepositoryOwner { login }
        repository { nameWithOwner }
        additions deletions changedFiles
        author { login ... on User { name } }
        commits(last: 1) {
          nodes {
            commit {
              statusCheckRollup {
                state
                contexts(first: 100) {
                  pageInfo { hasNextPage }
                  nodes {
                    __typename
                    ... on CheckRun {
                      name status conclusion startedAt
                      checkSuite { workflowRun { workflow { name } } }
                    }
                    ... on StatusContext { context state }
                  }
                }
              }
            }
          }
        }`

const searchQuery = `
query($q: String!, $n: Int!) {
  search(query: $q, type: ISSUE, first: $n) {
    nodes {
      ... on PullRequest {` + prFields + `
      }
    }
  }
}`

type graphQLResponse struct {
	Data struct {
		Search struct {
			Nodes []prNode `json:"nodes"`
		} `json:"search"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type prNode struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	IsDraft        bool      `json:"isDraft"`
	ReviewDecision string    `json:"reviewDecision"`
	Mergeable      string    `json:"mergeable"`
	UpdatedAt      time.Time `json:"updatedAt"`
	CreatedAt      time.Time `json:"createdAt"`
	HeadRefName    string    `json:"headRefName"`
	BaseRefName    string    `json:"baseRefName"`
	IsCross        bool      `json:"isCrossRepository"`
	HeadOwner      *struct {
		Login string `json:"login"`
	} `json:"headRepositoryOwner"`
	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
	ChangedFiles int `json:"changedFiles"`
	Repository   struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Author struct {
		Login string `json:"login"`
		Name  string `json:"name"`
	} `json:"author"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State    string `json:"state"`
					Contexts struct {
						PageInfo struct {
							HasNextPage bool `json:"hasNextPage"`
						} `json:"pageInfo"`
						Nodes []contextNode `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

type contextNode struct {
	TypeName   string    `json:"__typename"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	StartedAt  time.Time `json:"startedAt"`
	CheckSuite *struct {
		WorkflowRun *struct {
			Workflow struct {
				Name string `json:"name"`
			} `json:"workflow"`
		} `json:"workflowRun"`
	} `json:"checkSuite"`
	Context string `json:"context"`
	State   string `json:"state"`
}

// workflow is empty for a CheckRun posted by an app rather than by Actions.
func (n contextNode) workflow() string {
	if n.CheckSuite == nil || n.CheckSuite.WorkflowRun == nil {
		return ""
	}
	return n.CheckSuite.WorkflowRun.Workflow.Name
}

// CheckRepo verifies the repo is actually reachable. The search index reports
// issueCount 0 for a repo the caller cannot see -- an org IP allow list blocking
// you is indistinguishable from having no PRs -- but a direct repository query
// returns a real error, so ask for one before trusting an empty board.
func (c *Client) CheckRepo(ctx context.Context, repo string) error {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return fmt.Errorf("repo %q must be owner/name", repo)
	}
	body, err := json.Marshal(map[string]any{
		"query":     `query($o:String!,$n:String!){repository(owner:$o,name:$n){name}}`,
		"variables": map[string]any{"o": owner, "n": name},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var out struct {
		Data struct {
			Repository *struct{ Name string } `json:"repository"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if len(out.Errors) > 0 {
		return fmt.Errorf("%s", out.Errors[0].Message)
	}
	if out.Data.Repository == nil {
		return fmt.Errorf("repo %s not found, or you cannot see it", repo)
	}
	return nil
}

// Search runs one rule's query. The search index is noisy (the same query has
// ranged 1.4s-4.1s), so callers should run rules concurrently.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]PR, error) {
	body, err := json.Marshal(map[string]any{
		"query":     searchQuery,
		"variables": map[string]any{"q": query, "n": limit},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github returned %s", resp.Status)
	}
	var out graphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	// GraphQL reports query errors in a 200 body, so this is not redundant
	// with the status check above.
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("github: %s", out.Errors[0].Message)
	}

	prs := make([]PR, 0, len(out.Data.Search.Nodes))
	for _, n := range out.Data.Search.Nodes {
		prs = append(prs, n.toPR())
	}
	c.fillRequired(ctx, prs)
	return prs, nil
}

// Watched is a PR as the watch poll saw it. State is OPEN, MERGED or CLOSED:
// the board only searches open PRs, so once one merges this is the only
// request that still sees it.
type Watched struct {
	PR
	State string
}

// Watch fetches every watched PR in one request, one alias per PR. A PR that
// no longer resolves is left out of the result rather than failing the rest.
func (c *Client) Watch(ctx context.Context, repo string, numbers []int) (map[int]Watched, error) {
	if len(numbers) == 0 {
		return nil, nil
	}
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return nil, fmt.Errorf("repo %q must be owner/name", repo)
	}
	var q strings.Builder
	q.WriteString("query($o: String!, $r: String!) {\n  repository(owner: $o, name: $r) {\n")
	for _, n := range numbers {
		fmt.Fprintf(&q, "    p%d: pullRequest(number: %d) { state %s\n    }\n", n, n, prFields)
	}
	q.WriteString("  }\n}")

	body, err := json.Marshal(map[string]any{
		"query":     q.String(),
		"variables": map[string]any{"o": owner, "r": name},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github returned %s", resp.Status)
	}
	var out struct {
		Data struct {
			Repository map[string]*struct {
				prNode
				State string `json:"state"`
			} `json:"repository"`
		} `json:"data"`
		Errors []struct {
			Message string   `json:"message"`
			Type    string   `json:"type"`
			Path    []string `json:"path"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	got := make(map[int]Watched, len(numbers))
	for _, n := range out.Data.Repository {
		if n != nil && n.Number != 0 {
			got[n.Number] = Watched{PR: n.toPR(), State: n.State}
		}
	}
	// A missing PR is a NOT_FOUND on its own alias, so a poll where every PR
	// is missing still succeeds and the caller can drop them. Anything else
	// fails the poll when nothing came back.
	if len(got) == 0 {
		for _, e := range out.Errors {
			if e.Type != "NOT_FOUND" || len(e.Path) != 2 {
				return nil, fmt.Errorf("github: %s", e.Message)
			}
		}
	}
	// The watch copy is what a row draws from when it is fresher, so it needs
	// the same answer as the board or the row would flip between the two.
	prs := make([]PR, 0, len(got))
	for _, w := range got {
		prs = append(prs, w.PR)
	}
	c.fillRequired(ctx, prs)
	for _, p := range prs {
		w := got[p.Number]
		w.PR = p
		got[p.Number] = w
	}
	return got, nil
}

// fillRequired asks which checks branch protection requires, for the PRs that
// have a failing or cancelled check. A separate request because isRequired
// needs the PR number as an argument, which a search cannot pass per node,
// and mergeStateStatus came back UNKNOWN for 54 of 101 open PRs measured.
// Best effort: on any error the PRs keep a nil Required, which reads as all
// required, the board's behaviour before this existed.
func (c *Client) fillRequired(ctx context.Context, prs []PR) {
	var want []int
	for i, p := range prs {
		if len(p.FailedGates)+len(p.CancelledGates) > 0 && !p.truncated && strings.Contains(p.Repo, "/") {
			want = append(want, i)
		}
	}
	if len(want) == 0 {
		return
	}

	repoAlias := map[string]int{}
	byRepo := map[int][]int{}
	vars := map[string]any{}
	var repoOrder []int
	for _, i := range want {
		a, ok := repoAlias[prs[i].Repo]
		if !ok {
			a = len(repoAlias)
			repoAlias[prs[i].Repo] = a
			owner, name, _ := strings.Cut(prs[i].Repo, "/")
			vars[fmt.Sprintf("o%d", a)], vars[fmt.Sprintf("r%d", a)] = owner, name
			repoOrder = append(repoOrder, a)
		}
		byRepo[a] = append(byRepo[a], prs[i].Number)
	}

	var q strings.Builder
	q.WriteString("query(")
	for _, a := range repoOrder {
		fmt.Fprintf(&q, "$o%d: String!, $r%d: String!, ", a, a)
	}
	q.WriteString(") {\n")
	for _, a := range repoOrder {
		fmt.Fprintf(&q, "  r%d: repository(owner: $o%d, name: $r%d) {\n", a, a, a)
		for _, n := range byRepo[a] {
			fmt.Fprintf(&q, `    p%d: pullRequest(number: %d) { number commits(last: 1) { nodes { commit {
      statusCheckRollup { contexts(first: 100) { nodes {
        __typename
        ... on CheckRun { name isRequired(pullRequestNumber: %d) }
        ... on StatusContext { context isRequired(pullRequestNumber: %d) }
      } } } } } } }
`, n, n, n, n)
		}
		q.WriteString("  }\n")
	}
	q.WriteString("}")

	body, err := json.Marshal(map[string]any{"query": q.String(), "variables": vars})
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(), bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var out struct {
		Data map[string]map[string]*struct {
			Number  int `json:"number"`
			Commits struct {
				Nodes []struct {
					Commit struct {
						StatusCheckRollup *struct {
							Contexts struct {
								Nodes []struct {
									Name       string `json:"name"`
									Context    string `json:"context"`
									IsRequired bool   `json:"isRequired"`
								} `json:"nodes"`
							} `json:"contexts"`
						} `json:"statusCheckRollup"`
					} `json:"commit"`
				} `json:"nodes"`
			} `json:"commits"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return
	}

	// Partial errors leave their own alias null; every PR that did come back
	// is still a full answer for that PR.
	type answer struct {
		leaves    map[string]bool
		umbrellas map[string]bool
	}
	answers := map[Key]answer{}
	for repo, a := range repoAlias {
		for _, pr := range out.Data[fmt.Sprintf("r%d", a)] {
			if pr == nil || len(pr.Commits.Nodes) == 0 || pr.Commits.Nodes[0].Commit.StatusCheckRollup == nil {
				continue
			}
			ans := answer{map[string]bool{}, map[string]bool{}}
			for _, ctx := range pr.Commits.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes {
				if !ctx.IsRequired {
					continue
				}
				name := ctx.Name
				if name == "" {
					name = ctx.Context
				}
				// Keyed by gate name, which drops the workflow: if two
				// workflows share a job name and one is required, both read as
				// required, which errs toward showing a failure as blocking.
				if leaf := gateName(name); leaf != "" {
					ans.leaves[leaf] = true
				} else {
					ans.umbrellas[name] = true
				}
			}
			answers[Key{Repo: repo, Number: pr.Number}] = ans
		}
	}
	for _, i := range want {
		ans, ok := answers[prs[i].Key()]
		if !ok {
			continue
		}
		// A required umbrella that has not passed blocks on behalf of the
		// checks under it, which share its workflow prefix. A bare umbrella
		// with no prefix could be gating anything, so it claims every check.
		for _, u := range prs[i].openUmbrellas {
			if !ans.umbrellas[u] {
				continue
			}
			prefix := ""
			if j := strings.LastIndex(u, " / "); j >= 0 {
				prefix = u[:j+3]
			}
			for _, raw := range prs[i].rawProblems {
				if strings.HasPrefix(raw, prefix) {
					ans.leaves[gateName(raw)] = true
				}
			}
		}
		prs[i].Required = ans.leaves
	}
}

func (n prNode) toPR() PR {
	pr := PR{
		Repo:        n.Repository.NameWithOwner,
		Number:      n.Number,
		Title:       n.Title,
		URL:         n.URL,
		Author:      n.Author.Login,
		AuthorName:  n.Author.Name,
		IsDraft:     n.IsDraft,
		Review:      n.ReviewDecision,
		Mergeable:   n.Mergeable,
		UpdatedAt:   n.UpdatedAt,
		CreatedAt:   n.CreatedAt,
		HeadRefName: n.HeadRefName,
		BaseRefName: n.BaseRefName,

		Additions:    n.Additions,
		Deletions:    n.Deletions,
		ChangedFiles: n.ChangedFiles,
	}
	if n.IsCross && n.HeadOwner != nil {
		pr.HeadOwner = n.HeadOwner.Login
	}
	if len(n.Commits.Nodes) == 0 {
		return pr
	}
	rollup := n.Commits.Nodes[0].Commit.StatusCheckRollup
	if rollup == nil {
		return pr
	}
	// Deduped per bucket rather than globally: the same leaf name legitimately
	// appears once as a skipped reusable-workflow stub and once as the real
	// run, and dropping the second would lose the one that matters.
	failed := map[string]bool{}
	pending := map[string]bool{}
	cancelled := map[string]bool{}
	passed := map[string]bool{}
	var anyFailed, anyPending, anyCancelled, anyDone bool
	for _, c := range latestChecks(rollup.Contexts.Nodes) {
		if c.state == "SKIPPED" {
			pr.SkippedCount++
			anyDone = true
			continue
		}
		// Umbrella gates still count toward CIState, since they can be the
		// only thing that failed; they are just never named.
		leaf := gateName(c.name)
		if leaf == "" && !isSuccess(c.state) {
			pr.openUmbrellas = append(pr.openUmbrellas, c.name)
		}
		if leaf != "" && (isFailure(c.state) || c.state == "CANCELLED") {
			pr.rawProblems = append(pr.rawProblems, c.name)
		}
		switch {
		case isFailure(c.state):
			anyFailed = true
			if leaf != "" && !failed[leaf] {
				failed[leaf] = true
				pr.FailedGates = append(pr.FailedGates, leaf)
			}
		case isPending(c.state):
			anyPending = true
			if leaf != "" && !pending[leaf] {
				pending[leaf] = true
				pr.PendingGates = append(pr.PendingGates, leaf)
			}
		case c.state == "CANCELLED":
			anyCancelled = true
			if leaf != "" && !cancelled[leaf] {
				cancelled[leaf] = true
				pr.CancelledGates = append(pr.CancelledGates, leaf)
			}
		case isSuccess(c.state):
			anyDone = true
			if leaf != "" && !passed[leaf] {
				passed[leaf] = true
				pr.PassedCount++
			}
		}
	}
	// Derived rather than taken from rollup.state, which counts every run on
	// the commit: a run cancelled by a newer one keeps the rollup FAILURE
	// while GitHub's own PR page shows green.
	switch {
	case anyFailed:
		pr.CIState = "FAILURE"
	case anyPending:
		pr.CIState = "PENDING"
	case anyCancelled:
		pr.CIState = "CANCELLED"
	case anyDone:
		pr.CIState = "SUCCESS"
	default:
		pr.CIState = rollup.State
	}
	// Past the first page a check's newest run, or a failure, may be unseen,
	// and the rollup is the only state that covers them all.
	if rollup.Contexts.PageInfo.HasNextPage {
		pr.CIState = rollup.State
		pr.truncated = true
	}
	return pr
}

type check struct{ name, state string }

// latestChecks keeps only the newest run of each workflow's job, as GitHub's PR
// page does. A re-run or a superseded workflow run leaves the older runs on
// the commit, so without this a cancelled or since-fixed run stays red. The
// workflow is part of the key because two workflows can both have a "test" job.
// StatusContexts need nothing: GitHub already keeps only the latest per context.
func latestChecks(nodes []contextNode) []check {
	type run struct {
		at     int
		start  time.Time
		queued bool
	}
	var out []check
	newest := map[[2]string]run{}
	for _, n := range nodes {
		if n.TypeName == "StatusContext" {
			out = append(out, check{n.Context, n.State})
			continue
		}
		state := n.Conclusion
		if state == "" {
			// A CheckRun that has not finished carries a null conclusion, so
			// its status is the only thing that says it is still running.
			state = n.Status
		}
		// A queued run has no startedAt yet, and is the newest by definition.
		// A finished run can also lack one, if it was cancelled while queued.
		queued := n.StartedAt.IsZero() && n.Status != "COMPLETED"
		key := [2]string{n.workflow(), n.Name}
		cur, seen := newest[key]
		if !seen {
			newest[key] = run{len(out), n.StartedAt, queued}
			out = append(out, check{n.Name, state})
			continue
		}
		if queued || (!cur.queued && n.StartedAt.After(cur.start)) {
			newest[key] = run{cur.at, n.StartedAt, queued}
			out[cur.at].state = state
		}
	}
	return out
}

func isFailure(state string) bool {
	switch state {
	case "FAILURE", "TIMED_OUT", "STARTUP_FAILURE", "ERROR", "ACTION_REQUIRED":
		return true
	}
	return false
}

// PENDING covers both a StatusContext that has not reported and a CheckRun's
// queued/in-progress status, which arrive on different fields.
func isPending(state string) bool {
	switch state {
	case "PENDING", "EXPECTED", "QUEUED", "IN_PROGRESS", "WAITING", "REQUESTED":
		return true
	}
	return false
}

// NEUTRAL counts as passing because it does not block a merge, which is the
// question the count is standing in for.
func isSuccess(state string) bool {
	switch state {
	case "SUCCESS", "NEUTRAL":
		return true
	}
	return false
}

// Check names arrive as "apps_ci / webapp_e2e / Run E2E Tests (1, 5)".
// Keep the last path segment and drop the shard suffix so a sharded job counts
// once. Umbrella gates just restate their children.
func gateName(raw string) string {
	parts := strings.Split(raw, " / ")
	leaf := strings.TrimSpace(parts[len(parts)-1])
	if i := strings.LastIndex(leaf, " ("); i > 0 && strings.HasSuffix(leaf, ")") {
		if inner := leaf[i+2 : len(leaf)-1]; strings.Contains(inner, ",") {
			leaf = strings.TrimSpace(leaf[:i])
		}
	}
	switch leaf {
	case "E2E Status", "CI Gate":
		return ""
	}
	return leaf
}

// Detail is the second request, made once per `d` press for one PR. Everything
// here is either per-PR literal (compare needs the head ref, so it cannot be
// batched across a search) or too expensive on a 50-PR query: reviewer names
// cost +6,565 bytes and a rate-limit point on the board, and nothing here.
// Measured at 912-1,400 bytes, 1.2-1.9s, 1 point. See docs/pr-detail.md §2.4.
type Detail struct {
	// Repo and Number tag the response so a stale one can be dropped.
	Repo   string
	Number int

	BehindBy      int
	Unresolved    int
	Reviewers     []Reviewer
	DefaultBranch string
}

func (d Detail) Key() Key { return Key{Repo: d.Repo, Number: d.Number} }

// Reviewer is someone who has actually formed an opinion. Requested reviewers
// are not carried: 17 of 50 are teams, which the board already expresses as a
// whole section, so naming them restates the section header.
type Reviewer struct {
	Login string
	Name  string
	State string // APPROVED | CHANGES_REQUESTED
}

const detailQuery = `
query($o: String!, $r: String!, $n: Int!, $head: String!) {
  repository(owner: $o, name: $r) {
    defaultBranchRef { name }
    pullRequest(number: $n) {
      baseRef { compare(headRef: $head) { behindBy } }
      reviewThreads(first: 100) { nodes { isResolved isOutdated } }
      latestOpinionatedReviews(first: 10) {
        nodes { state author { login ... on User { name } } }
      }
    }
  }
}`

// Detail fetches the on-demand half of the overlay. The caller renders without
// it first and lets these lines arrive late, so a failure here costs the two
// lines and nothing else.
func (c *Client) Detail(ctx context.Context, repo string, number int, head string) (Detail, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return Detail{}, fmt.Errorf("repo %q must be owner/name", repo)
	}
	body, err := json.Marshal(map[string]any{
		"query": detailQuery,
		"variables": map[string]any{
			"o": owner, "r": name, "n": number, "head": head,
		},
	})
	if err != nil {
		return Detail{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(), bytes.NewReader(body))
	if err != nil {
		return Detail{}, err
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Detail{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Detail{}, fmt.Errorf("github returned %s", resp.Status)
	}

	var out struct {
		Data struct {
			Repository struct {
				DefaultBranchRef *struct {
					Name string `json:"name"`
				} `json:"defaultBranchRef"`
				PullRequest *struct {
					BaseRef *struct {
						Compare *struct {
							BehindBy int `json:"behindBy"`
						} `json:"compare"`
					} `json:"baseRef"`
					ReviewThreads struct {
						Nodes []struct {
							IsResolved bool `json:"isResolved"`
							IsOutdated bool `json:"isOutdated"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
					LatestOpinionatedReviews struct {
						Nodes []struct {
							State  string `json:"state"`
							Author struct {
								Login string `json:"login"`
								Name  string `json:"name"`
							} `json:"author"`
						} `json:"nodes"`
					} `json:"latestOpinionatedReviews"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Detail{}, err
	}
	if len(out.Errors) > 0 {
		return Detail{}, fmt.Errorf("github: %s", out.Errors[0].Message)
	}

	d := Detail{Number: number}
	if ref := out.Data.Repository.DefaultBranchRef; ref != nil {
		d.DefaultBranch = ref.Name
	}
	pr := out.Data.Repository.PullRequest
	if pr == nil {
		return d, fmt.Errorf("pr #%d not found", number)
	}
	if pr.BaseRef != nil && pr.BaseRef.Compare != nil {
		d.BehindBy = pr.BaseRef.Compare.BehindBy
	}
	// Outdated threads hang off a line the PR has since rewritten, so they are
	// not something anyone still has to answer.
	for _, t := range pr.ReviewThreads.Nodes {
		if !t.IsResolved && !t.IsOutdated {
			d.Unresolved++
		}
	}
	for _, r := range pr.LatestOpinionatedReviews.Nodes {
		d.Reviewers = append(d.Reviewers, Reviewer{
			Login: r.Author.Login, Name: r.Author.Name, State: r.State,
		})
	}
	return d, nil
}
