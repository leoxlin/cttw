package worker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/llin/cttw/internal/gitexec"
	"github.com/llin/cttw/internal/github"
	"github.com/llin/cttw/internal/llm"
	"github.com/llin/cttw/internal/store"
)

type Worker struct {
	GH    github.Client
	Git   *gitexec.Runner
	LLM   llm.Client
	Store *store.Store
	Owner string
	Repo  string
}

func (w *Worker) Execute(ctx context.Context, jobID string) error {
	job, err := w.Store.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	chunk, err := w.Store.GetChunk(ctx, job.ChunkID)
	if err != nil {
		return err
	}

	job.Status = "running"
	job.Attempts++
	now := time.Now().UTC()
	job.StartedAt = &now
	if err := w.Store.UpdateJob(ctx, job); err != nil {
		return err
	}

	base := "main"
	if chunk.DependsOnChunkID != "" {
		dep, err := w.Store.GetChunk(ctx, chunk.DependsOnChunkID)
		if err == nil && dep.Branch != "" {
			base = dep.Branch
		}
	}
	branch := fmt.Sprintf("cttw/%s/%s", chunk.TaskID[:8], slug(chunk.Title))
	chunk.Branch = branch
	chunk.BaseBranch = base
	chunk.Status = "running"
	if err := w.Store.UpdateChunk(ctx, chunk); err != nil {
		return err
	}

	if err := w.Git.CheckoutNew(branch, base); err != nil {
		return w.fail(ctx, job, chunk, err)
	}

	if w.LLM != nil {
		prompt := fmt.Sprintf("Implement this chunk in Go:\n%s\n\n%s", chunk.Title, chunk.Description)
		out, err := w.LLM.Chat(ctx, "", prompt)
		if err != nil {
			return w.fail(ctx, job, chunk, err)
		}
		_ = out
	}

	if err := w.Git.Add("."); err != nil {
		return w.fail(ctx, job, chunk, err)
	}
	if err := w.Git.Commit(fmt.Sprintf("feat: %s", chunk.Title)); err != nil {
		return w.fail(ctx, job, chunk, err)
	}
	if err := w.Git.Push(branch); err != nil {
		return w.fail(ctx, job, chunk, err)
	}

	prNum, err := w.GH.CreatePullRequest(ctx, w.Owner, w.Repo,
		fmt.Sprintf("[cttw] %s", chunk.Title),
		chunk.Description,
		branch, base)
	if err != nil {
		return w.fail(ctx, job, chunk, err)
	}

	chunk.PRNumber = prNum
	chunk.Status = "completed"
	if err := w.Store.UpdateChunk(ctx, chunk); err != nil {
		return err
	}
	job.Status = "completed"
	completed := time.Now().UTC()
	job.CompletedAt = &completed
	return w.Store.UpdateJob(ctx, job)
}

func (w *Worker) fail(ctx context.Context, job *store.Job, chunk *store.Chunk, err error) error {
	job.Status = "failed"
	job.Error = err.Error()
	chunk.Status = "failed"
	chunk.Output = err.Error()
	_ = w.Store.UpdateJob(ctx, job)
	_ = w.Store.UpdateChunk(ctx, chunk)
	return err
}

func slug(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), " ", "-")
}
