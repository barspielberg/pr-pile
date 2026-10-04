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

type PR struct {
	Number       int
	Title        string
	URL          string
	Author       string // login; the identifier you actually @-mention
	AuthorName   string // display name, null for 36% of this board: see docs/pr-detail.md §5.2
	IsDraft      bool
	Review       string // APPROVED | CHANGES_REQUESTED | REVIEW_REQUIRED | ""
	Mergeable    string // MERGEABLE | CONFLICTING | UNKNOWN
	UpdatedAt    time.Time
	CreatedAt    time.Time
	HeadRefName  string
	BaseRefName  string
	Additions    int
	Deletions    int
	ChangedFiles int
	CIState      string   // SUCCESS | FAILURE | PENDING | ERROR | "" (none)
	FailedGates  []string // names of failing checks, deduped
	PendingGates []string // names of still-running checks, deduped
	PassedCount  int      // passing checks are counted, not named: see docs/checks-page.md

	// Skipped is the largest bucket on this board (47% of contexts) and means
	// a job's path filter did not match, which is a fact about the workflow
	// rather than about the PR. Counted so the overlay can reconcile its
	// total against GitHub's, never listed.
	SkippedCount int
}

// prFields is everything a row draws. The watch poll asks for the same fields
// so a watched PR parses into the same PR the board holds.
const prFields = `
        number title url isDraft reviewDecision mergeable updatedAt createdAt
        headRefName baseRefName
        additions deletions changedFiles
        author { login ... on User { name } }
        commits(last: 1) {
          nodes {
            commit {
              statusCheckRollup {
                state
                contexts(first: 100) {
                  nodes {
                    __typename
                    ... on CheckRun { name status conclusion }
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
	Additions      int       `json:"additions"`
	Deletions      int       `json:"deletions"`
	ChangedFiles   int       `json:"changedFiles"`
	Author         struct {
		Login string `json:"login"`
		Name  string `json:"name"`
	} `json:"author"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State    string `json:"state"`
					Contexts struct {
						Nodes []struct {
							TypeName   string `json:"__typename"`
							Name       string `json:"name"`
							Status     string `json:"status"`
							Conclusion string `json:"conclusion"`
							Context    string `json:"context"`
							State      string `json:"state"`
						} `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
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
	return got, nil
}

func (n prNode) toPR() PR {
	pr := PR{
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
	if len(n.Commits.Nodes) == 0 {
		return pr
	}
	rollup := n.Commits.Nodes[0].Commit.StatusCheckRollup
	if rollup == nil {
		return pr
	}
	pr.CIState = rollup.State
	// Deduped per bucket rather than globally: the same leaf name legitimately
	// appears once as a skipped reusable-workflow stub and once as the real
	// run, and dropping the second would lose the one that matters.
	failed := map[string]bool{}
	pending := map[string]bool{}
	passed := map[string]bool{}
	for _, ctxNode := range rollup.Contexts.Nodes {
		name, state := ctxNode.Name, ctxNode.Conclusion
		if ctxNode.TypeName == "StatusContext" {
			name, state = ctxNode.Context, ctxNode.State
		} else if state == "" {
			// A CheckRun that has not finished carries a null conclusion, so
			// its status is the only thing that says it is still running.
			state = ctxNode.Status
		}
		if state == "SKIPPED" {
			pr.SkippedCount++
			continue
		}
		leaf := gateName(name)
		if leaf == "" {
			continue
		}
		switch {
		case isFailure(state):
			if !failed[leaf] {
				failed[leaf] = true
				pr.FailedGates = append(pr.FailedGates, leaf)
			}
		case isPending(state):
			if !pending[leaf] {
				pending[leaf] = true
				pr.PendingGates = append(pr.PendingGates, leaf)
			}
		case isSuccess(state):
			if !passed[leaf] {
				passed[leaf] = true
				pr.PassedCount++
			}
		}
	}
	return pr
}

func isFailure(state string) bool {
	switch state {
	case "FAILURE", "TIMED_OUT", "CANCELLED", "ERROR", "ACTION_REQUIRED":
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
	Number int // tags the response so a stale one can be dropped

	BehindBy      int
	Unresolved    int
	Reviewers     []Reviewer
	DefaultBranch string
}

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
