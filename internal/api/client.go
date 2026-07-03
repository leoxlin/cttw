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

func (c *Client) CreateTask(description string) (*TaskResponse, error) {
	body, err := json.Marshal(CreateTaskRequest{Description: description})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	resp, err := c.post("/api/v1/tasks", body)
	if err != nil {
		return nil, err
	}
	var tr TaskResponse
	if err := json.Unmarshal(resp, &tr); err != nil {
		return nil, err
	}
	return &tr, nil
}

func (c *Client) ListTasks() ([]TaskResponse, error) {
	resp, err := c.get("/api/v1/tasks")
	if err != nil {
		return nil, err
	}
	var tasks []TaskResponse
	if err := json.Unmarshal(resp, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (c *Client) GetTask(id string) (*TaskResponse, error) {
	resp, err := c.get("/api/v1/tasks/" + url.PathEscape(id))
	if err != nil {
		return nil, err
	}
	var tr TaskResponse
	if err := json.Unmarshal(resp, &tr); err != nil {
		return nil, err
	}
	return &tr, nil
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
