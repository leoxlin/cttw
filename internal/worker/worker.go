package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/llin/cttw/internal/gitexec"
	"github.com/llin/cttw/internal/github"
	"github.com/llin/cttw/internal/llm"
	"github.com/llin/cttw/internal/store"
)

type Worker struct {
	GH            github.Client
	Git           *gitexec.Runner
	LLM           llm.Client
	Store         *store.Store
	Owner         string
	Repo          string
	DefaultBranch string
}

func (w *Worker) baseBranch() string {
	if w.DefaultBranch != "" {
		return w.DefaultBranch
	}
	return "main"
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

	base := w.baseBranch()
	if chunk.DependsOnChunkID != "" {
		dep, err := w.Store.GetChunk(ctx, chunk.DependsOnChunkID)
		if err == nil && dep.Branch != "" {
			base = dep.Branch
		}
	}
	branch := fmt.Sprintf("cttw/%s/%s", shortID(chunk.TaskID), slug(chunk.Title))
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
		artifact := filepath.Join(w.Git.Dir, fmt.Sprintf("cttw-chunk-%s.md", shortID(chunk.ID)))
		if err := os.WriteFile(artifact, []byte(out), 0644); err != nil {
			return w.fail(ctx, job, chunk, fmt.Errorf("write llm artifact: %w", err))
		}
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

func (w *Worker) fail(ctx context.Context, job *store.Job, chunk *store.Chunk, cause error) error {
	job.Status = "failed"
	job.Error = cause.Error()
	chunk.Status = "failed"
	chunk.Output = cause.Error()

	var errs []error
	if err := w.Store.UpdateJob(ctx, job); err != nil {
		errs = append(errs, fmt.Errorf("persist failed job: %w", err))
	}
	if err := w.Store.UpdateChunk(ctx, chunk); err != nil {
		errs = append(errs, fmt.Errorf("persist failed chunk: %w", err))
	}
	if len(errs) > 0 {
		return errors.Join(append([]error{cause}, errs...)...)
	}
	return cause
}

var slugInvalid = regexp.MustCompile(`[^a-z0-9_]+`)

func slug(s string) string {
	s = strings.ToLower(s)
	s = slugInvalid.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "chunk"
	}
	return s
}

func shortID(id string) string {
	if len(id) < 8 {
		return id
	}
	return id[:8]
}
