package api

import "time"

type CreateTaskRequest struct {
	Description string `json:"description"`
}

type TaskResponse struct {
	ID                string          `json:"id"`
	Description       string          `json:"description"`
	Status            string          `json:"status"`
	RepoOwner         string          `json:"repo_owner"`
	RepoName          string          `json:"repo_name"`
	ParentIssueNumber int             `json:"parent_issue_number"`
	Chunks            []ChunkResponse `json:"chunks,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type ChunkResponse struct {
	ID          string `json:"id"`
	TaskID      string `json:"task_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Branch      string `json:"branch,omitempty"`
	BaseBranch  string `json:"base_branch,omitempty"`
	IssueNumber int    `json:"issue_number,omitempty"`
	PRNumber    int    `json:"pr_number,omitempty"`
	SortOrder   int    `json:"sort_order"`
}
