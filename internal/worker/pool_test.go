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

type blockingWorker struct {
	started chan string
	release chan struct{}
}

func (b *blockingWorker) Execute(ctx context.Context, jobID string) error {
	b.started <- jobID
	<-b.release
	return nil
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

func TestPool_RecoversRunningJobs(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	task, _ := s.CreateTask(ctx, "x", "o", "r")
	chunk, _ := s.CreateChunk(ctx, store.Chunk{TaskID: task.ID, Title: "c", Description: "d", SortOrder: 1})
	job, _ := s.CreateJob(ctx, store.Job{ChunkID: chunk.ID, Type: "execute"})
	// Simulate an unclean shutdown: mark the job running.
	job.Status = "running"
	require.NoError(t, s.UpdateJob(ctx, job))

	d := &dummyWorker{ran: make(chan struct{})}
	pool := &Pool{Worker: d, Store: s, NumWorkers: 1, Interval: 10 * time.Millisecond}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go pool.Start(ctx)

	select {
	case <-d.ran:
	case <-time.After(time.Second):
		t.Fatal("worker did not run recovered job")
	}
	pool.Stop()

	got, _ := s.GetJob(ctx, job.ID)
	assert.Equal(t, "completed", got.Status)
}

func TestPool_RevertsClaimedJobOnShutdown(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	task, _ := s.CreateTask(ctx, "x", "o", "r")
	chunk, _ := s.CreateChunk(ctx, store.Chunk{TaskID: task.ID, Title: "c", Description: "d", SortOrder: 1})
	job1, _ := s.CreateJob(ctx, store.Job{ChunkID: chunk.ID, Type: "execute"})
	job2, _ := s.CreateJob(ctx, store.Job{ChunkID: chunk.ID, Type: "execute"})
	job3, _ := s.CreateJob(ctx, store.Job{ChunkID: chunk.ID, Type: "execute"})

	release := make(chan struct{})
	started := make(chan string, 1)
	bw := &blockingWorker{started: started, release: release}
	pool := &Pool{Worker: bw, Store: s, NumWorkers: 1, Interval: 10 * time.Millisecond}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go pool.Start(ctx)

	// Wait for the first job to be picked up by the worker.
	select {
	case id := <-started:
		require.Equal(t, job1.ID, id)
	case <-time.After(time.Second):
		t.Fatal("worker did not start first job")
	}

	// Give the dispatcher time to claim and buffer job2, then block on job3.
	time.Sleep(50 * time.Millisecond)

	// Stop the pool while the dispatcher is blocked sending job3.
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(release)
	}()
	pool.Stop()

	// job1 and job2 were dispatched; job3 was claimed but not sent and should
	// have been reverted to pending.
	got1, _ := s.GetJob(ctx, job1.ID)
	got2, _ := s.GetJob(ctx, job2.ID)
	got3, _ := s.GetJob(ctx, job3.ID)
	assert.Equal(t, "completed", got1.Status)
	assert.Equal(t, "completed", got2.Status)
	assert.Equal(t, "pending", got3.Status)
}
