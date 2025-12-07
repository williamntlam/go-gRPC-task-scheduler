package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/db"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/testutil"
)

var testPoolAttempts *pgxpool.Pool

func init() {
	// Setup test pool for attempts tests
	ctx := context.Background()
	cfg := testutil.GetTestConfig()

	var err error
	testPoolAttempts, err = testutil.SetupTestDB(ctx, cfg)
	if err != nil {
		panic(fmt.Sprintf("Failed to setup test database: %v", err))
	}
}

func setupAttemptsTest(t *testing.T) {
	ctx := context.Background()
	if err := testutil.CleanupTestDB(ctx, testPoolAttempts); err != nil {
		t.Fatalf("Failed to cleanup test database: %v", err)
	}
}

func teardownAttemptsTest(t *testing.T) {
	ctx := context.Background()
	if err := testutil.CleanupTestDB(ctx, testPoolAttempts); err != nil {
		t.Fatalf("Failed to cleanup test database: %v", err)
	}
}
	ctx := context.Background()
	cfg := testutil.GetTestConfig()

	var err error
	testPoolAttempts, err = testutil.SetupTestDB(ctx, cfg)
	if err != nil {
		panic(fmt.Sprintf("Failed to setup test database: %v", err))
	}
	defer testPoolAttempts.Close()

	// Cleanup before running tests
	if err := testutil.CleanupTestDB(ctx, testPoolAttempts); err != nil {
		panic(fmt.Sprintf("Failed to cleanup test database: %v", err))
	}

	code := m.Run()

	// Cleanup after tests
	if err := testutil.CleanupTestDB(ctx, testPoolAttempts); err != nil {
		panic(fmt.Sprintf("Failed to cleanup test database: %v", err))
	}

	os.Exit(code)
}

func TestInsertAttempt(t *testing.T) {
	ctx := context.Background()
	setupAttemptsTest(t)
	defer teardownAttemptsTest(t)

	t.Run("inserts attempt successfully", func(t *testing.T) {
		// Create a job first
		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "running",
			Attempts:    1,
			MaxAttempts: 3,
		}

		_, _, err := db.CreateJob(ctx, testPoolAttempts, job)
		require.NoError(t, err)

		// Insert attempt
		err = db.InsertAttempt(ctx, testPoolAttempts, jobID)
		require.NoError(t, err)

		// Verify attempt was created
		query := `SELECT COUNT(*) FROM task_attempts WHERE task_id = $1 AND finished_at IS NULL`
		var count int
		err = testPoolAttempts.QueryRow(ctx, query, jobID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("allows multiple attempts for same job", func(t *testing.T) {
		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "running",
			Attempts:    2,
			MaxAttempts: 3,
		}

		_, _, err := db.CreateJob(ctx, testPoolAttempts, job)
		require.NoError(t, err)

		// Insert first attempt
		err = db.InsertAttempt(ctx, testPoolAttempts, jobID)
		require.NoError(t, err)

		// Update first attempt to finished
		updateQuery := `UPDATE task_attempts SET finished_at = now() WHERE task_id = $1`
		_, err = testPoolAttempts.Exec(ctx, updateQuery, jobID)
		require.NoError(t, err)

		// Insert second attempt
		err = db.InsertAttempt(ctx, testPoolAttempts, jobID)
		require.NoError(t, err)

		// Verify both attempts exist
		query := `SELECT COUNT(*) FROM task_attempts WHERE task_id = $1`
		var count int
		err = testPoolAttempts.QueryRow(ctx, query, jobID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("handles non-existent job gracefully", func(t *testing.T) {
		nonExistentID := uuid.New()
		// This might fail or succeed depending on foreign key constraints
		err := db.InsertAttempt(ctx, testPoolAttempts, nonExistentID)
		// Either outcome is acceptable - we're testing error handling
		if err != nil {
			assert.Contains(t, err.Error(), "failed to insert task attempt")
		}
	})

	t.Run("handles database errors", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		jobID := uuid.New()
		err := db.InsertAttempt(cancelledCtx, testPoolAttempts, jobID)
		assert.Error(t, err)
	})
}

func TestUpdateAttemptOnSuccess(t *testing.T) {
	ctx := context.Background()
	setupAttemptsTest(t)
	defer teardownAttemptsTest(t)

	t.Run("updates attempt on success", func(t *testing.T) {
		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "running",
			Attempts:    1,
			MaxAttempts: 3,
		}

		_, _, err := db.CreateJob(ctx, testPoolAttempts, job)
		require.NoError(t, err)

		// Insert attempt
		err = db.InsertAttempt(ctx, testPoolAttempts, jobID)
		require.NoError(t, err)

		// Update attempt on success
		err = db.UpdateAttemptOnSuccess(ctx, testPoolAttempts, jobID)
		require.NoError(t, err)

		// Verify attempt was updated
		query := `SELECT ok, finished_at FROM task_attempts WHERE task_id = $1 AND finished_at IS NOT NULL`
		var ok bool
		var finishedAt time.Time
		err = testPoolAttempts.QueryRow(ctx, query, jobID).Scan(&ok, &finishedAt)
		require.NoError(t, err)
		assert.True(t, ok)
		assert.False(t, finishedAt.IsZero())
	})

	t.Run("updates only unfinished attempts", func(t *testing.T) {
		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "running",
			Attempts:    2,
			MaxAttempts: 3,
		}

		_, _, err := db.CreateJob(ctx, testPoolAttempts, job)
		require.NoError(t, err)

		// Insert and finish first attempt
		err = db.InsertAttempt(ctx, testPoolAttempts, jobID)
		require.NoError(t, err)
		err = db.UpdateAttemptOnSuccess(ctx, testPoolAttempts, jobID)
		require.NoError(t, err)

		// Insert second attempt
		err = db.InsertAttempt(ctx, testPoolAttempts, jobID)
		require.NoError(t, err)

		// Update second attempt
		err = db.UpdateAttemptOnSuccess(ctx, testPoolAttempts, jobID)
		require.NoError(t, err)

		// Verify both attempts are marked as finished
		query := `SELECT COUNT(*) FROM task_attempts WHERE task_id = $1 AND finished_at IS NOT NULL AND ok = true`
		var count int
		err = testPoolAttempts.QueryRow(ctx, query, jobID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("handles non-existent job", func(t *testing.T) {
		nonExistentID := uuid.New()
		// Should not error - just updates 0 rows
		err := db.UpdateAttemptOnSuccess(ctx, testPoolAttempts, nonExistentID)
		require.NoError(t, err)
	})

	t.Run("handles database errors", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		jobID := uuid.New()
		err := db.UpdateAttemptOnSuccess(cancelledCtx, testPoolAttempts, jobID)
		assert.Error(t, err)
	})
}

func TestUpdateAttemptOnFailure(t *testing.T) {
	ctx := context.Background()

	t.Run("updates attempt on failure", func(t *testing.T) {
		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "running",
			Attempts:    1,
			MaxAttempts: 3,
		}

		_, _, err := db.CreateJob(ctx, testPoolAttempts, job)
		require.NoError(t, err)

		// Insert attempt
		err = db.InsertAttempt(ctx, testPoolAttempts, jobID)
		require.NoError(t, err)

		// Update attempt on failure
		errorMsg := "test error message"
		err = db.UpdateAttemptOnFailure(ctx, testPoolAttempts, jobID, errorMsg)
		require.NoError(t, err)

		// Verify attempt was updated
		query := `SELECT ok, error, finished_at FROM task_attempts WHERE task_id = $1 AND finished_at IS NOT NULL`
		var ok bool
		var errorText string
		var finishedAt time.Time
		err = testPoolAttempts.QueryRow(ctx, query, jobID).Scan(&ok, &errorText, &finishedAt)
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, errorMsg, errorText)
		assert.False(t, finishedAt.IsZero())
	})

	t.Run("stores error message correctly", func(t *testing.T) {
		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "running",
			Attempts:    1,
			MaxAttempts: 3,
		}

		_, _, err := db.CreateJob(ctx, testPoolAttempts, job)
		require.NoError(t, err)

		err = db.InsertAttempt(ctx, testPoolAttempts, jobID)
		require.NoError(t, err)

		errorMsg := "connection timeout after 30s"
		err = db.UpdateAttemptOnFailure(ctx, testPoolAttempts, jobID, errorMsg)
		require.NoError(t, err)

		// Verify error message
		query := `SELECT error FROM task_attempts WHERE task_id = $1`
		var storedError string
		err = testPoolAttempts.QueryRow(ctx, query, jobID).Scan(&storedError)
		require.NoError(t, err)
		assert.Equal(t, errorMsg, storedError)
	})

	t.Run("handles long error messages", func(t *testing.T) {
		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "running",
			Attempts:    1,
			MaxAttempts: 3,
		}

		_, _, err := db.CreateJob(ctx, testPoolAttempts, job)
		require.NoError(t, err)

		err = db.InsertAttempt(ctx, testPoolAttempts, jobID)
		require.NoError(t, err)

		// Create a long error message
		longError := strings.Repeat("error detail ", 100)
		err = db.UpdateAttemptOnFailure(ctx, testPoolAttempts, jobID, longError)
		require.NoError(t, err)

		// Verify long error was stored
		query := `SELECT error FROM task_attempts WHERE task_id = $1`
		var storedError string
		err = testPoolAttempts.QueryRow(ctx, query, jobID).Scan(&storedError)
		require.NoError(t, err)
		assert.Equal(t, longError, storedError)
	})

	t.Run("handles non-existent job", func(t *testing.T) {
		nonExistentID := uuid.New()
		err := db.UpdateAttemptOnFailure(ctx, testPoolAttempts, nonExistentID, "test error")
		require.NoError(t, err) // Should not error - just updates 0 rows
	})

	t.Run("handles database errors", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		jobID := uuid.New()
		err := db.UpdateAttemptOnFailure(cancelledCtx, testPoolAttempts, jobID, "test error")
		assert.Error(t, err)
	})
}

func TestAttemptLifecycle(t *testing.T) {
	ctx := context.Background()
	setupAttemptsTest(t)
	defer teardownAttemptsTest(t)

	t.Run("complete attempt lifecycle", func(t *testing.T) {
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

		_, _, err := db.CreateJob(ctx, testPoolAttempts, job)
		require.NoError(t, err)

		// 1. Job starts - insert attempt
		err = db.InsertAttempt(ctx, testPoolAttempts, jobID)
		require.NoError(t, err)

		// 2. Job fails - update attempt with error
		err = db.UpdateAttemptOnFailure(ctx, testPoolAttempts, jobID, "first attempt failed")
		require.NoError(t, err)

		// 3. Job retries - insert new attempt
		err = db.InsertAttempt(ctx, testPoolAttempts, jobID)
		require.NoError(t, err)

		// 4. Job succeeds - update attempt on success
		err = db.UpdateAttemptOnSuccess(ctx, testPoolAttempts, jobID)
		require.NoError(t, err)

		// Verify both attempts exist with correct states
		query := `SELECT COUNT(*) FROM task_attempts WHERE task_id = $1`
		var count int
		err = testPoolAttempts.QueryRow(ctx, query, jobID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 2, count)

		// Verify first attempt failed
		query = `SELECT ok FROM task_attempts WHERE task_id = $1 AND error = 'first attempt failed'`
		var ok1 bool
		err = testPoolAttempts.QueryRow(ctx, query, jobID).Scan(&ok1)
		require.NoError(t, err)
		assert.False(t, ok1)

		// Verify second attempt succeeded
		query = `SELECT ok FROM task_attempts WHERE task_id = $1 AND error IS NULL`
		var ok2 bool
		err = testPoolAttempts.QueryRow(ctx, query, jobID).Scan(&ok2)
		require.NoError(t, err)
		assert.True(t, ok2)
	})
}
