package api

import "time"

type CreateProjectRequest struct {
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	LocalDir      string `json:"local_dir"`
	DefaultBranch string `json:"default_branch"`
	CloneURL      string `json:"clone_url,omitempty"`
}

type UpdateProjectRequest struct {
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	LocalDir      string `json:"local_dir"`
	DefaultBranch string `json:"default_branch"`
	CloneURL      string `json:"clone_url,omitempty"`
}

type ProjectResponse struct {
	ID            string    `json:"id"`
	Owner         string    `json:"owner"`
	Name          string    `json:"name"`
	LocalDir      string    `json:"local_dir"`
	DefaultBranch string    `json:"default_branch"`
	CloneURL      string    `json:"clone_url,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

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
	RepoOwner   string         `json:"repo_owner,omitempty"`
	RepoName    string         `json:"repo_name,omitempty"`
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
	IssueNumber int       `json:"issue_number,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
