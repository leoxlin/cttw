package api

import "time"

type CreateProblemRequest struct {
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	Description string `json:"description"`
}

type UpdateProblemRequest struct {
	Description string `json:"description"`
}

type ProblemResponse struct {
	ID          string         `json:"id"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	RepoID      string         `json:"repo_id"`
	IssueNumber int            `json:"issue_number"`
	Tasks       []TaskResponse `json:"tasks,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type TaskResponse struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	PRNumber    int       `json:"pr_number,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
