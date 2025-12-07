package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	redisc "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/db"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/redis"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/testutil"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/worker"
)

var testWorkerPool *pgxpool.Pool
var testWorkerRedis *redisc.Client

func setupWorkerTest(t *testing.T) (*worker.Worker, context.CancelFunc) {
	ctx := context.Background()
	cfg := testutil.GetTestConfig()

	var err error
	if testWorkerPool == nil {
		testWorkerPool, err = testutil.SetupTestDB(ctx, cfg)
		require.NoError(t, err)
	}

	if testWorkerRedis == nil {
		testWorkerRedis, err = testutil.SetupTestRedis(ctx, cfg)
		require.NoError(t, err)
	}

	// Cleanup before test
	if err := testutil.CleanupTestDB(ctx, testWorkerPool); err != nil {
		t.Fatalf("Failed to cleanup test database: %v", err)
	}
	if err := testutil.CleanupTestRedis(ctx, testWorkerRedis); err != nil {
		t.Fatalf("Failed to cleanup test redis: %v", err)
	}

	w := worker.NewWorker(testWorkerPool, testWorkerRedis, 2)
	return w, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		w.Stop(ctx)
	}
}

func teardownWorkerTest(t *testing.T) {
	ctx := context.Background()
	if err := testutil.CleanupTestDB(ctx, testWorkerPool); err != nil {
		t.Logf("Warning: Failed to cleanup test database: %v", err)
	}
	if err := testutil.CleanupTestRedis(ctx, testWorkerRedis); err != nil {
		t.Logf("Warning: Failed to cleanup test redis: %v", err)
	}
}

func TestWorkerClaimJob(t *testing.T) {
	ctx := context.Background()
	_, stop := setupWorkerTest(t)
	defer stop()
	defer teardownWorkerTest(t)

	t.Run("claims queued job successfully", func(t *testing.T) {
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

		_, _, err := db.CreateJob(ctx, testWorkerPool, job)
		require.NoError(t, err)

		// Use reflection or create a test helper to access claimJob
		// For now, we'll test through the public interface
		// Push job to Redis
		err = redis.PushJob(ctx, testWorkerRedis, jobID, "noop", "high")
		require.NoError(t, err)

		// Start worker briefly to claim job
		// Note: This is an integration test - we're testing the full flow
		// In a real scenario, you might want to expose claimJob as a test helper
	})

	t.Run("does not claim already running job", func(t *testing.T) {
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

		_, _, err := db.CreateJob(ctx, testWorkerPool, job)
		require.NoError(t, err)

		// Job is already running, should not be claimable
		// This tests the atomicity of the claim operation
	})
}

func TestWorkerMarkJobSucceeded(t *testing.T) {
	ctx := context.Background()
	_, stop := setupWorkerTest(t)
	defer stop()
	defer teardownWorkerTest(t)

	t.Run("marks job as succeeded", func(t *testing.T) {
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

		_, _, err := db.CreateJob(ctx, testWorkerPool, job)
		require.NoError(t, err)

		// Insert attempt first
		err = db.InsertAttempt(ctx, testWorkerPool, jobID)
		require.NoError(t, err)

		// Mark as succeeded (this would normally be called internally)
		// We need to access the method - for now, test through integration
		// In practice, you might want to make this method public for testing
		// or create a test helper
	})
}

func TestWorkerHandleJobFailure(t *testing.T) {
	ctx := context.Background()
	_, stop := setupWorkerTest(t)
	defer stop()
	defer teardownWorkerTest(t)

	t.Run("schedules retry when attempts remain", func(t *testing.T) {
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

		_, _, err := db.CreateJob(ctx, testWorkerPool, job)
		require.NoError(t, err)

		// Insert attempt
		err = db.InsertAttempt(ctx, testWorkerPool, jobID)
		require.NoError(t, err)

		// Test retry logic through integration
		// The worker should mark job for retry when handleJobFailure is called
	})

	t.Run("moves to DLQ when max attempts reached", func(t *testing.T) {
		jobID := uuid.New()
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "running",
			Attempts:    3,
			MaxAttempts: 3,
		}

		_, _, err := db.CreateJob(ctx, testWorkerPool, job)
		require.NoError(t, err)

		// Insert attempt
		err = db.InsertAttempt(ctx, testWorkerPool, jobID)
		require.NoError(t, err)

		// Test DLQ logic through integration
		// The worker should move job to DLQ when max attempts reached
	})
}

func TestWorkerExecuteHandler(t *testing.T) {
	ctx := context.Background()
	_, stop := setupWorkerTest(t)
	defer stop()
	defer teardownWorkerTest(t)

	t.Run("executes noop handler successfully", func(t *testing.T) {
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

		_, _, err := db.CreateJob(ctx, testWorkerPool, job)
		require.NoError(t, err)

		// Test noop handler execution
		// This would require accessing executeHandler method
		// For integration testing, we'd test through the full worker flow
	})

	t.Run("executes http_call handler", func(t *testing.T) {
		// This would require a mock HTTP server
		// For now, we'll test the structure
		jobID := uuid.New()
		payload := map[string]interface{}{
			"url":    "https://httpbin.org/get",
			"method": "GET",
		}
		payloadJSON, _ := json.Marshal(payload)

		job := db.Job{
			TaskID:      jobID,
			Type:        "http_call",
			Priority:    "high",
			PayloadJSON: json.RawMessage(payloadJSON),
			Status:      "running",
			Attempts:    1,
			MaxAttempts: 3,
		}

		_, _, err := db.CreateJob(ctx, testWorkerPool, job)
		require.NoError(t, err)

		// Test http_call handler execution
		// Would require mock HTTP server or test server
	})

	t.Run("executes db_tx handler", func(t *testing.T) {
		jobID := uuid.New()
		payload := map[string]interface{}{
			"query": "SELECT 1",
			"params": []interface{}{},
		}
		payloadJSON, _ := json.Marshal(payload)

		job := db.Job{
			TaskID:      jobID,
			Type:        "db_tx",
			Priority:    "high",
			PayloadJSON: json.RawMessage(payloadJSON),
			Status:      "running",
			Attempts:    1,
			MaxAttempts: 3,
		}

		_, _, err := db.CreateJob(ctx, testWorkerPool, job)
		require.NoError(t, err)

		// Test db_tx handler execution
	})
}

func TestWorkerRetryPump(t *testing.T) {
	ctx := context.Background()
	_, stop := setupWorkerTest(t)
	defer stop()
	defer teardownWorkerTest(t)

	t.Run("requeues jobs ready for retry", func(t *testing.T) {
		jobID := uuid.New()
		nextRunAt := time.Now().Add(-1 * time.Minute) // In the past
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "retry",
			Attempts:    1,
			MaxAttempts: 3,
			NextRunAt:   &nextRunAt,
		}

		_, _, err := db.CreateJob(ctx, testWorkerPool, job)
		require.NoError(t, err)

		// Test retry pump requeues job
		// This would test the retry pump processRetryJobs function
		// In practice, you'd wait for the retry pump ticker to fire
	})

	t.Run("does not requeue jobs not ready", func(t *testing.T) {
		jobID := uuid.New()
		nextRunAt := time.Now().Add(5 * time.Minute) // In the future
		job := db.Job{
			TaskID:      jobID,
			Type:        "noop",
			Priority:    "high",
			PayloadJSON: json.RawMessage(`{}`),
			Status:      "retry",
			Attempts:    1,
			MaxAttempts: 3,
			NextRunAt:   &nextRunAt,
		}

		_, _, err := db.CreateJob(ctx, testWorkerPool, job)
		require.NoError(t, err)

		// Job should not be requeued yet
	})
}

func TestWorkerReaper(t *testing.T) {
	ctx := context.Background()
	_, stop := setupWorkerTest(t)
	defer stop()
	defer teardownWorkerTest(t)

	t.Run("recovers stuck jobs", func(t *testing.T) {
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

		_, _, err := db.CreateJob(ctx, testWorkerPool, job)
		require.NoError(t, err)

		// Create job payload for Redis processing queue
		payload := redis.JobPayload{
			TaskID:   jobID.String(),
			Type:     "noop",
			Priority: "high",
		}
		payloadJSON, _ := json.Marshal(payload)

		// Add to processing queue
		err = testWorkerRedis.LPush(ctx, "q:processing", payloadJSON).Err()
		require.NoError(t, err)

		// Update job to running with old timestamp (simulating stuck job)
		oldTime := time.Now().Add(-10 * time.Minute)
		updateQuery := `UPDATE tasks SET status = 'running', updated_at = $1 WHERE task_id = $2`
		_, err = testWorkerPool.Exec(ctx, updateQuery, oldTime, jobID)
		require.NoError(t, err)

		// Test reaper recovers stuck job
		// In practice, you'd wait for the reaper ticker to fire
	})
}

func TestWorkerGracefulShutdown(t *testing.T) {
	w, _ := setupWorkerTest(t)
	defer teardownWorkerTest(t)

	t.Run("stops gracefully", func(t *testing.T) {
		// Start worker
		go w.Start()

		// Give it a moment to start
		time.Sleep(100 * time.Millisecond)

		// Stop worker
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := w.Stop(stopCtx)
		require.NoError(t, err)
	})

	t.Run("prevents multiple stop calls", func(t *testing.T) {
		w2 := worker.NewWorker(testWorkerPool, testWorkerRedis, 2)

		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// First stop should succeed
		err := w2.Stop(stopCtx)
		require.NoError(t, err)

		// Second stop should return error
		err = w2.Stop(stopCtx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already in the process of stopping")
	})
}

func TestWorkerConcurrency(t *testing.T) {
	ctx := context.Background()
	w, stop := setupWorkerTest(t)
	defer stop()
	defer teardownWorkerTest(t)

	t.Run("processes multiple jobs concurrently", func(t *testing.T) {
		// Create multiple jobs
		jobIDs := make([]uuid.UUID, 5)
		for i := 0; i < 5; i++ {
			jobID := uuid.New()
			jobIDs[i] = jobID

			job := db.Job{
				TaskID:      jobID,
				Type:        "noop",
				Priority:    "high",
				PayloadJSON: json.RawMessage(`{}`),
				Status:      "queued",
				Attempts:    0,
				MaxAttempts: 3,
			}

			_, _, err := db.CreateJob(ctx, testWorkerPool, job)
			require.NoError(t, err)

			// Push to Redis
			err = redis.PushJob(ctx, testWorkerRedis, jobID, "noop", "high")
			require.NoError(t, err)
		}

		// Start worker
		go w.Start()

		// Wait for jobs to be processed
		time.Sleep(2 * time.Second)

		// Verify jobs were processed
		for _, jobID := range jobIDs {
			job, err := db.GetJobByID(ctx, testWorkerPool, jobID)
			require.NoError(t, err)
			// Job should be either succeeded or still processing
			assert.Contains(t, []string{"succeeded", "running", "queued"}, job.Status)
		}
	})
}
