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
	Number      int
	Title       string
	URL         string
	Author      string
	IsDraft     bool
	Review      string // APPROVED | CHANGES_REQUESTED | REVIEW_REQUIRED | ""
	Mergeable   string // MERGEABLE | CONFLICTING | UNKNOWN
	UpdatedAt   time.Time
	HeadRefName string
	BaseRefName string
	CIState     string   // SUCCESS | FAILURE | PENDING | ERROR | "" (none)
	FailedGates []string // names of failing checks, deduped
}

const searchQuery = `
query($q: String!, $n: Int!) {
  search(query: $q, type: ISSUE, first: $n) {
    nodes {
      ... on PullRequest {
        number title url isDraft reviewDecision mergeable updatedAt
        headRefName baseRefName
        author { login }
        commits(last: 1) {
          nodes {
            commit {
              statusCheckRollup {
                state
                contexts(first: 100) {
                  nodes {
                    __typename
                    ... on CheckRun { name conclusion }
                    ... on StatusContext { context state }
                  }
                }
              }
            }
          }
        }
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
	HeadRefName    string    `json:"headRefName"`
	BaseRefName    string    `json:"baseRefName"`
	Author         struct {
		Login string `json:"login"`
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
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

func (n prNode) toPR() PR {
	pr := PR{
		Number:      n.Number,
		Title:       n.Title,
		URL:         n.URL,
		Author:      n.Author.Login,
		IsDraft:     n.IsDraft,
		Review:      n.ReviewDecision,
		Mergeable:   n.Mergeable,
		UpdatedAt:   n.UpdatedAt,
		HeadRefName: n.HeadRefName,
		BaseRefName: n.BaseRefName,
	}
	if len(n.Commits.Nodes) == 0 {
		return pr
	}
	rollup := n.Commits.Nodes[0].Commit.StatusCheckRollup
	if rollup == nil {
		return pr
	}
	pr.CIState = rollup.State
	seen := map[string]bool{}
	for _, ctxNode := range rollup.Contexts.Nodes {
		name, state := ctxNode.Name, ctxNode.Conclusion
		if ctxNode.TypeName == "StatusContext" {
			name, state = ctxNode.Context, ctxNode.State
		}
		if !isFailure(state) {
			continue
		}
		leaf := gateName(name)
		if leaf == "" || seen[leaf] {
			continue
		}
		seen[leaf] = true
		pr.FailedGates = append(pr.FailedGates, leaf)
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
