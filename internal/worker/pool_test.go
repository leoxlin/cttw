package worker

import (
	"context"
	"testing"
	"time"

	"github.com/llin/cttw/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dummyWorker struct{ ran bool }

func (d *dummyWorker) Execute(ctx context.Context, jobID string) error {
	d.ran = true
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

	d := &dummyWorker{}
	pool := &Pool{Worker: d, Store: s, NumWorkers: 1, Interval: 10 * time.Millisecond}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go pool.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	got, _ := s.GetJob(ctx, job.ID)
	assert.Equal(t, "completed", got.Status)
	assert.True(t, d.ran)
}
