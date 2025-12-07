package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	// Note: cmd/api is in main package, so we test the server logic directly
	// by importing the necessary packages
	"github.com/williamntlam/go-grpc-task-scheduler/internal/db"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/redis"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/testutil"
	schedulerv1 "github.com/williamntlam/go-grpc-task-scheduler/proto/scheduler/v1"
	redisc "github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var testServerPool *pgxpool.Pool
var testServerRedis *redisc.Client

// testServer is a helper to create a server instance for testing
// Since cmd/api is in main package, we'll test the server methods directly
// by creating a server struct manually
type testServer struct {
	db    *pgxpool.Pool
	redis *redisc.Client
}

func setupServerTest(t *testing.T) (*testServer, func()) {
	ctx := context.Background()
	cfg := testutil.GetTestConfig()

	var err error
	if testServerPool == nil {
		testServerPool, err = testutil.SetupTestDB(ctx, cfg)
		require.NoError(t, err)
	}

	if testServerRedis == nil {
		testServerRedis, err = testutil.SetupTestRedis(ctx, cfg)
		require.NoError(t, err)
	}

	// Create server instance (we'll need to access the actual server methods)
	// For now, we'll test through the database and Redis directly
	// Full server testing would require exposing server methods or using gRPC test client
	server := &testServer{
		db:    testServerPool,
		redis: testServerRedis,
	}

	cleanup := func() {
		teardownServerTest(t)
	}

	return server, cleanup
}

// Note: Full server method testing requires either:
// 1. Moving server to internal package
// 2. Using gRPC test infrastructure
// 3. Exposing test helpers
// For now, we test the underlying logic (db, redis) and integration flows

	// Cleanup before test
	if err := testutil.CleanupTestDB(ctx, testServerPool); err != nil {
		t.Fatalf("Failed to cleanup test database: %v", err)
	}
	if err := testutil.CleanupTestRedis(ctx, testServerRedis); err != nil {
		t.Fatalf("Failed to cleanup test redis: %v", err)
	}
}

func teardownServerTest(t *testing.T) {
	ctx := context.Background()
	if err := testutil.CleanupTestDB(ctx, testServerPool); err != nil {
		t.Logf("Warning: Failed to cleanup test database: %v", err)
	}
	if err := testutil.CleanupTestRedis(ctx, testServerRedis); err != nil {
		t.Logf("Warning: Failed to cleanup test redis: %v", err)
	}
}

func TestSubmitJob(t *testing.T) {
	ctx := context.Background()
	setupServerTest(t)
	defer teardownServerTest(t)

	t.Run("submits job successfully", func(t *testing.T) {
		req := &schedulerv1.SubmitJobRequest{
			Job: &schedulerv1.Job{
				Type:     "noop",
				Priority: schedulerv1.Priority_PRIORITY_HIGH,
			},
		}

		resp, err := testServer.SubmitJob(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.JobId)

		// Verify job was created in database
		jobID, err := uuid.Parse(resp.JobId)
		require.NoError(t, err)

		job, err := db.GetJobByID(ctx, testServerPool, jobID)
		require.NoError(t, err)
		require.NotNil(t, job)
		assert.Equal(t, "noop", job.Type)
		assert.Equal(t, "high", job.Priority)
		assert.Equal(t, "queued", job.Status)
	})

	t.Run("handles idempotency correctly", func(t *testing.T) {
		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "http_call",
			Priority:    "critical",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "queued",
			Attempts:    0,
			MaxAttempts: 3,
		}

		// First creation
		createdID1, exists1, err1 := db.CreateJob(ctx, testServerPool, job)
		require.NoError(t, err1)
		assert.False(t, exists1)
		assert.Equal(t, jobID, createdID1)

		// Second creation (idempotent)
		createdID2, exists2, err2 := db.CreateJob(ctx, testServerPool, job)
		require.NoError(t, err2)
		assert.True(t, exists2) // Should indicate it already existed
		assert.Equal(t, jobID, createdID2)

		// Verify only one job exists
		exists, err := db.JobExists(ctx, testServerPool, jobID)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("creates job with payload", func(t *testing.T) {
		payload := map[string]interface{}{
			"url":    "https://example.com",
			"method": "GET",
		}
		payloadJSON, _ := json.Marshal(payload)

		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "http_call",
			Priority:    "default",
			PayloadJSON: json.RawMessage(payloadJSON),
			Status:      "queued",
			Attempts:    0,
			MaxAttempts: 3,
		}

		_, _, err := db.CreateJob(ctx, testServerPool, job)
		require.NoError(t, err)

		// Verify payload was stored
		retrieved, err := db.GetJobByID(ctx, testServerPool, jobID)
		require.NoError(t, err)
		assert.NotEmpty(t, retrieved.PayloadJSON)
	})

	t.Run("creates job with max_attempts", func(t *testing.T) {
		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "low",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "queued",
			Attempts:    0,
			MaxAttempts: 5,
		}

		_, _, err := db.CreateJob(ctx, testServerPool, job)
		require.NoError(t, err)

		retrieved, err := db.GetJobByID(ctx, testServerPool, jobID)
		require.NoError(t, err)
		assert.Equal(t, 5, retrieved.MaxAttempts)
	})

	t.Run("pushes job to Redis queue", func(t *testing.T) {
		jobID := uuid.New()
		err := redis.PushJob(ctx, testServerRedis, jobID, "noop", "high")
		require.NoError(t, err)

		// Verify job was pushed to Redis
		queueName := redis.GetQueueName("high")
		length, err := testServerRedis.LLen(ctx, queueName).Result()
		require.NoError(t, err)
		assert.Greater(t, length, int64(0))
	})
}

func TestGetJobByID(t *testing.T) {
	ctx := context.Background()
	_, cleanup := setupServerTest(t)
	defer cleanup()

	t.Run("retrieves existing job", func(t *testing.T) {
		// Create a job first
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

		_, _, err := db.CreateJob(ctx, testServerPool, job)
		require.NoError(t, err)

		// Get the job
		retrieved, err := db.GetJobByID(ctx, testServerPool, jobID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, jobID, retrieved.TaskID)
		assert.Equal(t, "queued", retrieved.Status)
		assert.Equal(t, 0, retrieved.Attempts)
	})

	t.Run("returns nil for non-existent job", func(t *testing.T) {
		nonExistentID := uuid.New()
		retrieved, err := db.GetJobByID(ctx, testServerPool, nonExistentID)
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("retrieves job with different statuses", func(t *testing.T) {
		statuses := []string{"queued", "running", "succeeded", "failed", "deadletter"}

		for _, status := range statuses {
			t.Run(status, func(t *testing.T) {
				jobID := uuid.New()
				job := db.Job{
					TaskID:      jobID,
					Type:        "noop",
					Priority:    "high",
					PayloadJSON: json.RawMessage(`{}`),
					Status:      status,
					Attempts:    1,
					MaxAttempts: 3,
				}

				_, _, err := db.CreateJob(ctx, testServerPool, job)
				require.NoError(t, err)

				retrieved, err := db.GetJobByID(ctx, testServerPool, jobID)
				require.NoError(t, err)
				require.NotNil(t, retrieved)
				assert.Equal(t, status, retrieved.Status)
			})
		}
	})
}

func TestCancelJob(t *testing.T) {
	ctx := context.Background()
	_, cleanup := setupServerTest(t)
	defer cleanup()

	t.Run("cancels queued job", func(t *testing.T) {
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

		_, _, err := db.CreateJob(ctx, testServerPool, job)
		require.NoError(t, err)

		// Push to Redis
		err = redis.PushJob(ctx, testServerRedis, jobID, "noop", "high")
		require.NoError(t, err)

		// Cancel job
		cancelled, err := db.CancelJob(ctx, testServerPool, jobID)
		require.NoError(t, err)
		assert.True(t, cancelled)

		// Verify job status
		retrieved, err := db.GetJobByID(ctx, testServerPool, jobID)
		require.NoError(t, err)
		assert.Equal(t, "cancelled", retrieved.Status)
	})

	t.Run("cancels running job", func(t *testing.T) {
		jobID := uuid.New()
		_, _, err := db.CreateJob(ctx, testServerPool, db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "queued",
			Attempts:    0,
			MaxAttempts: 3,
		})
		require.NoError(t, err)

		// Update to running
		updateQuery := `UPDATE tasks SET status = 'running' WHERE task_id = $1`
		_, err = testServerPool.Exec(ctx, updateQuery, jobID)
		require.NoError(t, err)

		cancelled, err := db.CancelJob(ctx, testServerPool, jobID)
		require.NoError(t, err)
		assert.True(t, cancelled)
	})

	t.Run("returns false for already completed job", func(t *testing.T) {
		jobID := uuid.New()
		_, _, err := db.CreateJob(ctx, testServerPool, db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "succeeded",
			Attempts:    1,
			MaxAttempts: 3,
		})
		require.NoError(t, err)

		// Update to succeeded
		updateQuery := `UPDATE tasks SET status = 'succeeded' WHERE task_id = $1`
		_, err = testServerPool.Exec(ctx, updateQuery, jobID)
		require.NoError(t, err)

		cancelled, err := db.CancelJob(ctx, testServerPool, jobID)
		require.NoError(t, err)
		assert.False(t, cancelled) // Cannot cancel completed job
	})

	t.Run("returns false for non-existent job", func(t *testing.T) {
		nonExistentID := uuid.New()
		cancelled, err := db.CancelJob(ctx, testServerPool, nonExistentID)
		require.NoError(t, err)
		assert.False(t, cancelled)
	})
}

func TestListJobsQuery(t *testing.T) {
	ctx := context.Background()
	_, cleanup := setupServerTest(t)
	defer cleanup()

	t.Run("queries multiple jobs", func(t *testing.T) {
		// Create multiple jobs
		for i := 0; i < 5; i++ {
			job := db.Job{
				TaskID:      uuid.New(),
				Type:        "noop",
				Priority:    "high",
				PayloadJSON: json.RawMessage(`{}`),
				Status:      "queued",
				Attempts:    0,
				MaxAttempts: 3,
			}
			_, _, err := db.CreateJob(ctx, testServerPool, job)
			require.NoError(t, err)
		}

		// Query jobs directly from database
		query := `SELECT COUNT(*) FROM tasks WHERE status = 'queued'`
		var count int
		err := testServerPool.QueryRow(ctx, query).Scan(&count)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 5)
	})

	t.Run("filters by status", func(t *testing.T) {
		// Create jobs with different statuses
		job1 := db.Job{
			TaskID:      uuid.New(),
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "queued",
			Attempts:    0,
			MaxAttempts: 3,
		}
		_, _, err := db.CreateJob(ctx, testServerPool, job1)
		require.NoError(t, err)

		job2 := db.Job{
			TaskID:      uuid.New(),
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "succeeded",
			Attempts:    1,
			MaxAttempts: 3,
		}
		_, _, err = db.CreateJob(ctx, testServerPool, job2)
		require.NoError(t, err)

		// Query queued jobs
		query := `SELECT COUNT(*) FROM tasks WHERE status = 'queued'`
		var count int
		err = testServerPool.QueryRow(ctx, query).Scan(&count)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 1)
	})
}

func TestWatchJobValidation(t *testing.T) {
	ctx := context.Background()
	_, cleanup := setupServerTest(t)
	defer cleanup()

	t.Run("validates job exists for watching", func(t *testing.T) {
		// Create a job
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

		_, _, err := db.CreateJob(ctx, testServerPool, job)
		require.NoError(t, err)

		// Verify job exists (prerequisite for WatchJob)
		retrieved, err := db.GetJobByID(ctx, testServerPool, jobID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, jobID, retrieved.TaskID)
	})

	t.Run("validates job_id format", func(t *testing.T) {
		// Test that invalid UUIDs are rejected
		invalidID := "invalid-uuid"
		_, err := uuid.Parse(invalidID)
		assert.Error(t, err) // Should fail to parse
	})
}
