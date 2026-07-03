package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/llin/cttw/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dummyWorker struct{ ran chan struct{} }

func (d *dummyWorker) Execute(ctx context.Context, jobID string) error {
	close(d.ran)
	return nil
}

type failingWorker struct {
	calls int
}

func (f *failingWorker) Execute(ctx context.Context, jobID string) error {
	f.calls++
	return errors.New("boom")
}

func TestPool_RunsPendingJob(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	task, _ := s.CreateTask(ctx, "x", "o", "r")
	chunk, _ := s.CreateChunk(ctx, store.Chunk{TaskID: task.ID, Title: "c", Description: "d", SortOrder: 1})
	job, _ := s.CreateJob(ctx, store.Job{ChunkID: chunk.ID, Type: "execute"})

	d := &dummyWorker{ran: make(chan struct{})}
	pool := &Pool{Worker: d, Store: s, NumWorkers: 1, Interval: 10 * time.Millisecond}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go pool.Start(ctx)

	select {
	case <-d.ran:
	case <-time.After(time.Second):
		t.Fatal("worker did not run")
	}
	pool.Stop()

	got, _ := s.GetJob(ctx, job.ID)
	assert.Equal(t, "completed", got.Status)
}

func TestPool_RetriesAndFailsJob(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	task, _ := s.CreateTask(ctx, "x", "o", "r")
	chunk, _ := s.CreateChunk(ctx, store.Chunk{TaskID: task.ID, Title: "c", Description: "d", SortOrder: 1})
	job, _ := s.CreateJob(ctx, store.Job{ChunkID: chunk.ID, Type: "execute", MaxAttempts: 2})

	fw := &failingWorker{}
	pool := &Pool{Worker: fw, Store: s, NumWorkers: 1, Interval: 10 * time.Millisecond}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go pool.Start(ctx)

	require.Eventually(t, func() bool {
		got, err := s.GetJob(ctx, job.ID)
		require.NoError(t, err)
		return got.Status == "failed"
	}, time.Second, 20*time.Millisecond)
	pool.Stop()

	assert.GreaterOrEqual(t, fw.calls, 2)
	got, _ := s.GetJob(ctx, job.ID)
	assert.Equal(t, "failed", got.Status)
	assert.Equal(t, 2, got.Attempts)
	assert.Contains(t, got.Error, "boom")
}
