package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/llin/cttw/internal/store"
)

// Executor performs a single job given its ID.
type Executor interface {
	Execute(ctx context.Context, jobID string) error
}

// Pool polls the store for pending jobs and dispatches them to a fixed number
// of workers.
type Pool struct {
	Worker     Executor
	Store      *store.Store
	NumWorkers int
	Interval   time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// Start begins polling for jobs and blocks until ctx is cancelled or Stop is
// called.
func (p *Pool) Start(ctx context.Context) {
	numWorkers := p.NumWorkers
	if numWorkers <= 0 {
		numWorkers = 2
	}
	interval := p.Interval
	if interval <= 0 {
		interval = 5 * time.Second
	}

	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	p.mu.Lock()
	p.cancel = cancel
	p.done = done
	p.mu.Unlock()

	defer close(done)

	jobs := make(chan string, numWorkers)
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.loop(ctx, jobs)
		}()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		case <-ticker.C:
			job, err := p.Store.NextPendingJob(ctx)
			if err != nil {
				log.Printf("next pending job: %v", err)
				continue
			}
			if job == nil {
				continue
			}
			select {
			case jobs <- job.ID:
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				return
			}
		}
	}
}

func (p *Pool) loop(ctx context.Context, jobs <-chan string) {
	for id := range jobs {
		if err := p.Worker.Execute(ctx, id); err != nil {
			log.Printf("job %s failed: %v", id, err)
			if err := p.handleError(ctx, id, err); err != nil {
				log.Printf("job %s error handling failed: %v", id, err)
			}
			continue
		}
		if err := p.completeJob(ctx, id); err != nil {
			log.Printf("job %s completion update failed: %v", id, err)
		}
	}
}

func (p *Pool) handleError(ctx context.Context, id string, cause error) error {
	// The pool context may be cancelled while we are persisting state; finish
	// the update using a context that ignores cancellation.
	persistCtx := context.WithoutCancel(ctx)
	j, err := p.Store.GetJob(persistCtx, id)
	if err != nil {
		return err
	}
	// If the executor already updated the job (e.g. the real Worker sets it to
	// failed on error), do not override its state.
	if j.Status != "running" {
		return nil
	}
	j.Attempts++
	j.Error = cause.Error()
	if j.Attempts >= j.MaxAttempts {
		j.Status = "failed"
		now := time.Now().UTC()
		j.CompletedAt = &now
	} else {
		j.Status = "pending"
	}
	return p.Store.UpdateJob(persistCtx, j)
}

func (p *Pool) completeJob(ctx context.Context, id string) error {
	// The pool context may be cancelled while we are persisting state; finish
	// the update using a context that ignores cancellation.
	persistCtx := context.WithoutCancel(ctx)
	j, err := p.Store.GetJob(persistCtx, id)
	if err != nil {
		return err
	}
	if j.Status == "completed" {
		return nil
	}
	j.Status = "completed"
	now := time.Now().UTC()
	j.CompletedAt = &now
	return p.Store.UpdateJob(persistCtx, j)
}

// Stop halts the pool and blocks until the workers have stopped. It is safe
// to call multiple times and before Start.
func (p *Pool) Stop() {
	p.mu.Lock()
	cancel := p.cancel
	done := p.done
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}
