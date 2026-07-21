package gitea

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Repository represents a Gitea repository.
type Repository struct {
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	CloneURL    string `json:"clone_url"`
}

// Branch represents a branch in a Gitea repository.
type Branch struct {
	Name          string     `json:"name"`
	CommitID      string     `json:"commit_id"`
	CommitMessage string     `json:"commit_message"`
	CommitTime    *time.Time `json:"commit_time,omitempty"`
}

// Tag represents a tag in a Gitea repository.
type Tag struct {
	Name          string     `json:"name"`
	CommitID      string     `json:"commit_id"`
	CommitMessage string     `json:"commit_message"`
	CommitTime    *time.Time `json:"commit_time,omitempty"`
}

// GiteaClient defines the interface for interacting with the Gitea API.
type GiteaClient interface {
	ListRepos() ([]Repository, error)
	ListBranches(owner, repo string) ([]Branch, error)
	ListTags(owner, repo string) ([]Tag, error)
}

// giteaClient is the concrete implementation of GiteaClient.
type giteaClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewGiteaClient creates a new GiteaClient with the given base URL and access token.
func NewGiteaClient(baseURL, token string) GiteaClient {
	return &giteaClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// giteaRepoResponse represents the JSON structure returned by the Gitea repos/search API.
type giteaRepoResponse struct {
	Data []giteaRepo `json:"data"`
	OK   bool        `json:"ok"`
}

// giteaRepo represents a single repository in the Gitea API response.
type giteaRepo struct {
	Name        string    `json:"name"`
	FullName    string    `json:"full_name"`
	Description string    `json:"description"`
	CloneURL    string    `json:"clone_url"`
	Owner       giteaUser `json:"owner"`
}

// giteaUser represents the owner object in a Gitea repo response.
type giteaUser struct {
	Login string `json:"login"`
}

// giteaBranch represents a single branch in the Gitea API response.
type giteaBranch struct {
	Name   string      `json:"name"`
	Commit giteaCommit `json:"commit"`
}

// giteaCommit represents the commit object in a Gitea branch response.
type giteaCommit struct {
	ID        string             `json:"id"`
	SHA       string             `json:"sha"`
	Message   string             `json:"message"`
	Timestamp time.Time          `json:"timestamp"`
	Created   time.Time          `json:"created"`
	Commit    giteaCommitDetails `json:"commit"`
	Author    giteaSignature     `json:"author"`
	Committer giteaSignature     `json:"committer"`
}

type giteaCommitDetails struct {
	Message   string         `json:"message"`
	Author    giteaSignature `json:"author"`
	Committer giteaSignature `json:"committer"`
}

type giteaSignature struct {
	Date time.Time `json:"date"`
}

// giteaTag represents a single tag in the Gitea API response.
type giteaTag struct {
	Name   string      `json:"name"`
	Commit giteaCommit `json:"commit"`
}

// ListRepos fetches all repositories from the Gitea instance.
// It paginates through results using limit and page parameters.
func (c *giteaClient) ListRepos() ([]Repository, error) {
	var allRepos []Repository
	page := 1
	limit := 50

	for {
		url := fmt.Sprintf("%s/api/v1/repos/search?limit=%d&page=%d", c.baseURL, limit, page)

		body, err := c.doRequest(url)
		if err != nil {
			return nil, fmt.Errorf("failed to list repositories: %w", err)
		}

		var resp giteaRepoResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("failed to parse repository list response: %w", err)
		}

		for _, r := range resp.Data {
			allRepos = append(allRepos, Repository{
				Owner:       r.Owner.Login,
				Name:        r.Name,
				FullName:    r.FullName,
				Description: r.Description,
				CloneURL:    r.CloneURL,
			})
		}

		// If we got fewer results than the limit, we've reached the last page.
		if len(resp.Data) < limit {
			break
		}
		page++
	}

	return allRepos, nil
}

// ListBranches fetches all branches for the specified repository.
func (c *giteaClient) ListBranches(owner, repo string) ([]Branch, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/branches", c.baseURL, owner, repo)

	body, err := c.doRequest(url)
	if err != nil {
		return nil, fmt.Errorf("failed to list branches for %s/%s: %w", owner, repo, err)
	}

	var giteaBranches []giteaBranch
	if err := json.Unmarshal(body, &giteaBranches); err != nil {
		return nil, fmt.Errorf("failed to parse branch list response for %s/%s: %w", owner, repo, err)
	}

	branches := make([]Branch, 0, len(giteaBranches))
	for _, b := range giteaBranches {
		c.hydrateCommit(owner, repo, &b.Commit)
		branches = append(branches, Branch{
			Name:          b.Name,
			CommitID:      b.Commit.id(),
			CommitMessage: b.Commit.commitMessage(),
			CommitTime:    b.Commit.commitTime(),
		})
	}

	return branches, nil
}

// ListTags fetches all tags for the specified repository.
func (c *giteaClient) ListTags(owner, repo string) ([]Tag, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/tags", c.baseURL, owner, repo)

	body, err := c.doRequest(url)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags for %s/%s: %w", owner, repo, err)
	}

	var giteaTags []giteaTag
	if err := json.Unmarshal(body, &giteaTags); err != nil {
		return nil, fmt.Errorf("failed to parse tag list response for %s/%s: %w", owner, repo, err)
	}

	tags := make([]Tag, 0, len(giteaTags))
	for _, t := range giteaTags {
		c.hydrateCommit(owner, repo, &t.Commit)
		tags = append(tags, Tag{
			Name:          t.Name,
			CommitID:      t.Commit.id(),
			CommitMessage: t.Commit.commitMessage(),
			CommitTime:    t.Commit.commitTime(),
		})
	}

	return tags, nil
}

func (c *giteaClient) hydrateCommit(owner, repo string, commit *giteaCommit) {
	if commit == nil || commit.id() == "" || (commit.commitMessage() != "" && commit.commitTime() != nil) {
		return
	}

	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/git/commits/%s", c.baseURL, owner, repo, commit.id())
	body, err := c.doRequest(url)
	if err != nil {
		return
	}

	var detail giteaCommit
	if err := json.Unmarshal(body, &detail); err != nil {
		return
	}
	if detail.ID == "" {
		detail.ID = commit.ID
	}
	if detail.SHA == "" {
		detail.SHA = commit.SHA
	}
	*commit = detail
}

func (c giteaCommit) id() string {
	if c.ID != "" {
		return c.ID
	}
	return c.SHA
}

func (c giteaCommit) commitMessage() string {
	if c.Commit.Message != "" {
		return c.Commit.Message
	}
	return c.Message
}

func (c giteaCommit) commitTime() *time.Time {
	for _, t := range []time.Time{
		c.Commit.Committer.Date,
		c.Commit.Author.Date,
		c.Committer.Date,
		c.Author.Date,
		c.Timestamp,
		c.Created,
	} {
		if !t.IsZero() {
			return &t
		}
	}
	return nil
}

// doRequest performs an authenticated GET request to the given URL and returns the response body.
func (c *giteaClient) doRequest(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", url, err)
	}

	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body from %s: %w", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gitea API returned status %d for %s: %s", resp.StatusCode, url, string(body))
	}

	return body, nil
}
