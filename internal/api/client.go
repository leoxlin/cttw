package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	Socket string
	http   *http.Client
}

func NewClient(socket string) *Client {
	client := http.DefaultClient
	if strings.HasPrefix(socket, "unix://") {
		path := strings.TrimPrefix(socket, "unix://")
		client = &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", path)
				},
			},
			Timeout: 30 * time.Second,
		}
	}
	return &Client{Socket: socket, http: client}
}

func (c *Client) CreateTask(description string) (*TaskResponse, error) {
	body, _ := json.Marshal(CreateTaskRequest{Description: description})
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
	resp, err := c.get("/api/v1/tasks/" + id)
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
	return io.ReadAll(resp.Body)
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
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("daemon %d: %s", resp.StatusCode, string(b))
	}
	return b, nil
}
