package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultRequestTimeout = 30 * time.Second

type Client struct {
	Socket string
	http   *http.Client
}

func NewClient(socket string) *Client {
	transport := &http.Transport{}
	if strings.HasPrefix(socket, "unix://") {
		path := strings.TrimPrefix(socket, "unix://")
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		}
	}
	return &Client{
		Socket: socket,
		http:   &http.Client{Transport: transport, Timeout: defaultRequestTimeout},
	}
}

func (c *Client) CreateProject(req CreateProjectRequest) (*ProjectResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	resp, err := c.post("/api/v1/projects", body)
	if err != nil {
		return nil, err
	}
	var project ProjectResponse
	if err := json.Unmarshal(resp, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

func (c *Client) ListProjects() ([]ProjectResponse, error) {
	resp, err := c.get("/api/v1/projects")
	if err != nil {
		return nil, err
	}
	var projects []ProjectResponse
	if err := json.Unmarshal(resp, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (c *Client) GetProject(id string) (*ProjectResponse, error) {
	resp, err := c.get("/api/v1/projects/" + url.PathEscape(id))
	if err != nil {
		return nil, err
	}
	var project ProjectResponse
	if err := json.Unmarshal(resp, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

func (c *Client) UpdateProject(id string, req UpdateProjectRequest) (*ProjectResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	resp, err := c.put("/api/v1/projects/"+url.PathEscape(id), body)
	if err != nil {
		return nil, err
	}
	var project ProjectResponse
	if err := json.Unmarshal(resp, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

func (c *Client) DeleteProject(id string) error {
	_, err := c.delete("/api/v1/projects/" + url.PathEscape(id))
	return err
}

func (c *Client) CreateProblem(owner, repo, description string) (*ProblemResponse, error) {
	body, err := json.Marshal(CreateProblemRequest{Owner: owner, Repo: repo, Description: description})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	resp, err := c.post("/api/v1/problems", body)
	if err != nil {
		return nil, err
	}
	var pr ProblemResponse
	if err := json.Unmarshal(resp, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func (c *Client) UpdateProblem(id, description string) (*ProblemResponse, error) {
	body, err := json.Marshal(UpdateProblemRequest{Description: description})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	resp, err := c.patch("/api/v1/problems/"+url.PathEscape(id), body)
	if err != nil {
		return nil, err
	}
	var pr ProblemResponse
	if err := json.Unmarshal(resp, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func (c *Client) ListProblems() ([]ProblemResponse, error) {
	resp, err := c.get("/api/v1/problems")
	if err != nil {
		return nil, err
	}
	var problems []ProblemResponse
	if err := json.Unmarshal(resp, &problems); err != nil {
		return nil, err
	}
	return problems, nil
}

func (c *Client) GetProblem(id string) (*ProblemResponse, error) {
	resp, err := c.get("/api/v1/problems/" + url.PathEscape(id))
	if err != nil {
		return nil, err
	}
	var pr ProblemResponse
	if err := json.Unmarshal(resp, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func (c *Client) Shutdown() error {
	_, err := c.post("/api/v1/shutdown", nil)
	return err
}

func (c *Client) Status() error {
	_, err := c.get("/api/v1/status")
	return err
}

func (c *Client) url(path string) string {
	if strings.HasPrefix(c.Socket, "unix://") {
		return "http://unix" + path
	}
	return "http://" + c.Socket + path
}

func (c *Client) get(path string) ([]byte, error) {
	req, err := http.NewRequest("GET", c.url(path), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("daemon %d: %s", resp.StatusCode, string(b))
	}
	return b, nil
}

func (c *Client) post(path string, body []byte) ([]byte, error) {
	return c.do("POST", path, body)
}

func (c *Client) patch(path string, body []byte) ([]byte, error) {
	return c.do("PATCH", path, body)
}

func (c *Client) put(path string, body []byte) ([]byte, error) {
	return c.do("PUT", path, body)
}

func (c *Client) delete(path string) ([]byte, error) {
	return c.do("DELETE", path, nil)
}

func (c *Client) do(method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(method, c.url(path), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("daemon %d: %s", resp.StatusCode, string(b))
	}
	return b, nil
}
