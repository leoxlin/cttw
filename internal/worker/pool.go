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
}

// Start begins polling for jobs and blocks until ctx is cancelled or Stop is
// called.
func (p *Pool) Start(ctx context.Context) {
	if p.NumWorkers <= 0 {
		p.NumWorkers = 2
	}
	if p.Interval <= 0 {
		p.Interval = 5 * time.Second
	}

	ctx, cancel := context.WithCancel(ctx)
	p.mu.Lock()
	p.cancel = cancel
	p.mu.Unlock()

	jobs := make(chan string, p.NumWorkers)
	var wg sync.WaitGroup
	for i := 0; i < p.NumWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.loop(ctx, jobs)
		}()
	}

	ticker := time.NewTicker(p.Interval)
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
			continue
		}
		if err := p.completeJob(ctx, id); err != nil {
			log.Printf("job %s completion update failed: %v", id, err)
		}
	}
}

func (p *Pool) completeJob(ctx context.Context, id string) error {
	j, err := p.Store.GetJob(ctx, id)
	if err != nil {
		return err
	}
	if j.Status == "completed" {
		return nil
	}
	j.Status = "completed"
	now := time.Now().UTC()
	j.CompletedAt = &now
	return p.Store.UpdateJob(ctx, j)
}

// Stop halts the pool. It is safe to call multiple times.
func (p *Pool) Stop() {
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
	}
	p.mu.Unlock()
}
