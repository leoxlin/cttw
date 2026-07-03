package coordinator

import (
	"context"
	"fmt"

	"github.com/llin/cttw/internal/github"
	"github.com/llin/cttw/internal/llm"
	"github.com/llin/cttw/internal/store"
)

type Coordinator struct {
	LLM   llm.Client
	GH    github.Client
	Store *store.Store
	Owner string
	Repo  string
}

func (c *Coordinator) StartTask(ctx context.Context, description string) (*store.Task, error) {
	task, err := c.Store.CreateTask(ctx, description, c.Owner, c.Repo)
	if err != nil {
		return nil, err
	}

	plans, err := llm.DecomposeTask(ctx, c.LLM, description)
	if err != nil {
		return nil, err
	}

	parentNum, err := c.GH.CreateIssue(ctx, c.Owner, c.Repo, fmt.Sprintf("[cttw] %s", description),
		"Task tracked by cttw.")
	if err != nil {
		return nil, err
	}
	task.ParentIssueNumber = parentNum
	task.Status = "ready"
	if err := c.Store.UpdateTask(ctx, task); err != nil {
		return nil, err
	}

	chunkIDs := make([]string, len(plans))
	for i, p := range plans {
		chunk := store.Chunk{
			TaskID:      task.ID,
			Title:       p.Title,
			Description: p.Description,
			SortOrder:   i,
		}
		if p.DependsOn >= 0 && p.DependsOn < i {
			chunk.DependsOnChunkID = chunkIDs[p.DependsOn]
		}
		created, err := c.Store.CreateChunk(ctx, chunk)
		if err != nil {
			return nil, err
		}
		chunkIDs[i] = created.ID

		num, err := c.GH.CreateIssue(ctx, c.Owner, c.Repo, p.Title, p.Description)
		if err != nil {
			return nil, err
		}
		created.IssueNumber = num
		if err := c.Store.UpdateChunk(ctx, created); err != nil {
			return nil, err
		}
		if err := c.GH.CreateSubIssue(ctx, c.Owner, c.Repo, parentNum, num); err != nil {
			return nil, err
		}
		if _, err := c.Store.CreateJob(ctx, store.Job{ChunkID: created.ID, Type: "execute"}); err != nil {
			return nil, err
		}
	}
	return task, nil
}
