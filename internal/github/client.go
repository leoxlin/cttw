package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Client provides GitHub API operations needed by cttw.
type Client interface {
	CreateIssue(ctx context.Context, owner, repo, title, body string) (int, error)
	CreateSubIssue(ctx context.Context, owner, repo string, parentNumber, childNumber int) error
}

type client struct {
	token   string
	baseURL string
	http    *http.Client
}

// New creates a GitHub client using the production API endpoint.
func New(token string, httpClient *http.Client) Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &client{token: token, baseURL: "https://api.github.com", http: httpClient}
}

func newWithURL(token, baseURL string, httpClient *http.Client) Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &client{token: token, baseURL: baseURL, http: httpClient}
}

func (c *client) do(ctx context.Context, method, path string, body, out any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return resp, fmt.Errorf("github %s %s: %d %s", method, path, resp.StatusCode, string(b))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp, err
		}
	}
	return resp, nil
}

type issueRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type issueResponse struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

func (c *client) CreateIssue(ctx context.Context, owner, repo, title, body string) (int, error) {
	var out issueResponse
	_, err := c.do(ctx, "POST", fmt.Sprintf("/repos/%s/%s/issues", owner, repo),
		issueRequest{Title: title, Body: body}, &out)
	return out.Number, err
}

func (c *client) CreateSubIssue(ctx context.Context, owner, repo string, parentNumber, childNumber int) error {
	// MVP: link via markdown tasklist in parent issue body.
	path := fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, parentNumber)
	var parent issueResponse
	if _, err := c.do(ctx, "GET", path, nil, &parent); err != nil {
		return err
	}
	parent.Body += fmt.Sprintf("\n- [ ] #%d\n", childNumber)
	_, err := c.do(ctx, "PATCH", path, issueRequest{Title: parent.Title, Body: parent.Body}, nil)
	return err
}
