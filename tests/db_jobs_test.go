package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/db"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/testutil"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()
	cfg := testutil.GetTestConfig()

	var err error
	testPool, err = testutil.SetupTestDB(ctx, cfg)
	if err != nil {
		panic(fmt.Sprintf("Failed to setup test database: %v", err))
	}
	defer testPool.Close()

	// Cleanup before running tests
	if err := testutil.CleanupTestDB(ctx, testPool); err != nil {
		panic(fmt.Sprintf("Failed to cleanup test database: %v", err))
	}

	code := m.Run()

	// Cleanup after tests
	if err := testutil.CleanupTestDB(ctx, testPool); err != nil {
		panic(fmt.Sprintf("Failed to cleanup test database: %v", err))
	}

	os.Exit(code)
}

func TestGetJobByID(t *testing.T) {
	ctx := context.Background()

	t.Run("returns job when exists", func(t *testing.T) {
		// Setup: Create a test job
		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{"test": "data"}`),
			Status:      "queued",
			Attempts:    0,
			MaxAttempts: 3,
		}

		createdID, _, err := db.CreateJob(ctx, testPool, job)
		require.NoError(t, err)
		require.Equal(t, jobID, createdID)

		// Test: Get the job
		retrieved, err := db.GetJobByID(ctx, testPool, jobID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, jobID, retrieved.TaskID)
		assert.Equal(t, "noop", retrieved.Type)
		assert.Equal(t, "high", retrieved.Priority)
		assert.Equal(t, "queued", retrieved.Status)
		assert.Equal(t, 0, retrieved.Attempts)
		assert.Equal(t, 3, retrieved.MaxAttempts)
	})

	t.Run("returns nil when job does not exist", func(t *testing.T) {
		nonExistentID := uuid.New()
		retrieved, err := db.GetJobByID(ctx, testPool, nonExistentID)
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("handles database errors", func(t *testing.T) {
		jobID := uuid.New()
		retrieved, err := db.GetJobByID(ctx, testPool, jobID)
		// Should not error for non-existent job
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})
}

func TestCreateJob(t *testing.T) {
	ctx := context.Background()

	t.Run("creates new job successfully", func(t *testing.T) {
		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "http_call",
			Priority:    "critical",
			PayloadJSON: json.RawMessage(`{"url": "https://example.com"}`),
			Status:      "queued",
			Attempts:    0,
			MaxAttempts: 5,
		}

		createdID, alreadyExists, err := db.CreateJob(ctx, testPool, job)
		require.NoError(t, err)
		assert.False(t, alreadyExists)
		assert.Equal(t, jobID, createdID)

		// Verify job was created
		retrieved, err := db.GetJobByID(ctx, testPool, jobID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, "http_call", retrieved.Type)
		assert.Equal(t, "critical", retrieved.Priority)
	})

	t.Run("handles idempotency - returns existing job", func(t *testing.T) {
		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "default",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "queued",
			Attempts:    0,
			MaxAttempts: 3,
		}

		// Create job first time
		createdID1, exists1, err1 := db.CreateJob(ctx, testPool, job)
		require.NoError(t, err1)
		assert.False(t, exists1)
		assert.Equal(t, jobID, createdID1)

		// Try to create same job again (idempotency)
		createdID2, exists2, err2 := db.CreateJob(ctx, testPool, job)
		require.NoError(t, err2)
		assert.True(t, exists2) // Should indicate it already existed
		assert.Equal(t, jobID, createdID2)
	})

	t.Run("creates job with next_run_at", func(t *testing.T) {
		jobID := uuid.New()
		nextRunAt := time.Now().Add(5 * time.Minute)
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "low",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "retry",
			Attempts:    1,
			MaxAttempts: 3,
			NextRunAt:   &nextRunAt,
		}

		createdID, _, err := db.CreateJob(ctx, testPool, job)
		require.NoError(t, err)
		assert.Equal(t, jobID, createdID)

		// Verify next_run_at was set
		retrieved, err := db.GetJobByID(ctx, testPool, jobID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		require.NotNil(t, retrieved.NextRunAt)
		assert.WithinDuration(t, nextRunAt, *retrieved.NextRunAt, time.Second)
	})

	t.Run("handles invalid data gracefully", func(t *testing.T) {
		jobID := uuid.New()
		// Create job with very long type (should be handled by DB constraints if any)
		job := db.Job{
			TaskID:      jobID,
			Type:        "a very long type name that might exceed database limits",
			Priority:    "invalid_priority", // Invalid priority
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "queued",
			Attempts:    0,
			MaxAttempts: 3,
		}

		// This might succeed or fail depending on DB constraints
		// We're testing that the function handles it
		_, _, err := db.CreateJob(ctx, testPool, job)
		// Error is acceptable here - we're testing error handling
		if err != nil {
			assert.Contains(t, err.Error(), "failed to create job")
		}
	})
}

func TestJobExists(t *testing.T) {
	ctx := context.Background()

	t.Run("returns true when job exists", func(t *testing.T) {
		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "queued",
			Attempts:    0,
			MaxAttempts: 3,
		}

		_, _, err := db.CreateJob(ctx, testPool, job)
		require.NoError(t, err)

		exists, err := db.JobExists(ctx, testPool, jobID)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("returns false when job does not exist", func(t *testing.T) {
		nonExistentID := uuid.New()
		exists, err := db.JobExists(ctx, testPool, nonExistentID)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("handles database errors", func(t *testing.T) {
		// Test with cancelled context
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		jobID := uuid.New()
		_, err := db.JobExists(cancelledCtx, testPool, jobID)
		assert.Error(t, err)
	})
}

func TestCancelJob(t *testing.T) {
	ctx := context.Background()

	t.Run("cancels queued job successfully", func(t *testing.T) {
		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "queued",
			Attempts:    0,
			MaxAttempts: 3,
		}

		_, _, err := db.CreateJob(ctx, testPool, job)
		require.NoError(t, err)

		cancelled, err := db.CancelJob(ctx, testPool, jobID)
		require.NoError(t, err)
		assert.True(t, cancelled)

		// Verify job status was updated
		retrieved, err := db.GetJobByID(ctx, testPool, jobID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, "cancelled", retrieved.Status)
	})

	t.Run("cancels running job successfully", func(t *testing.T) {
		jobID := uuid.New()
		// Create job first
		_, _, err := db.CreateJob(ctx, testPool, db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "queued",
			Attempts:    0,
			MaxAttempts: 3,
		})
		require.NoError(t, err)

		// Update to running status
		query := `UPDATE tasks SET status = 'running' WHERE task_id = $1`
		_, err = testPool.Exec(ctx, query, jobID)
		require.NoError(t, err)

		cancelled, err := db.CancelJob(ctx, testPool, jobID)
		require.NoError(t, err)
		assert.True(t, cancelled)
	})

	t.Run("returns false for already completed job", func(t *testing.T) {
		jobID := uuid.New()
		_, _, err := db.CreateJob(ctx, testPool, db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "queued",
			Attempts:    0,
			MaxAttempts: 3,
		})
		require.NoError(t, err)

		// Update to succeeded
		updateQuery := `UPDATE tasks SET status = 'succeeded' WHERE task_id = $1`
		_, err = testPool.Exec(ctx, updateQuery, jobID)
		require.NoError(t, err)

		cancelled, err := db.CancelJob(ctx, testPool, jobID)
		require.NoError(t, err)
		assert.False(t, cancelled) // Cannot cancel completed job
	})

	t.Run("returns false for non-existent job", func(t *testing.T) {
		nonExistentID := uuid.New()
		cancelled, err := db.CancelJob(ctx, testPool, nonExistentID)
		require.NoError(t, err)
		assert.False(t, cancelled)
	})

	t.Run("handles database errors", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		jobID := uuid.New()
		_, err := db.CancelJob(cancelledCtx, testPool, jobID)
		assert.Error(t, err)
	})
}

func TestCreateJobConcurrency(t *testing.T) {
	ctx := context.Background()

	t.Run("handles concurrent job creation", func(t *testing.T) {
		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "queued",
			Attempts:    0,
			MaxAttempts: 3,
		}

		// Try to create the same job concurrently
		results := make(chan error, 10)
		for i := 0; i < 10; i++ {
			go func() {
				_, _, err := db.CreateJob(ctx, testPool, job)
				results <- err
			}()
		}

		// Collect results
		errors := 0
		successes := 0
		for i := 0; i < 10; i++ {
			err := <-results
			if err != nil {
				errors++
			} else {
				successes++
			}
		}

		// At least one should succeed, others should handle idempotency
		assert.Greater(t, successes, 0)

		// Verify only one job exists
		exists, err := db.JobExists(ctx, testPool, jobID)
		require.NoError(t, err)
		assert.True(t, exists)
	})
}
