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
	req, err := http.NewRequest("POST", c.url(path), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
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
