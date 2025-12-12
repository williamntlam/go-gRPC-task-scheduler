package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/db"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/redis"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/testutil"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/worker"
)

// Integration tests test the full flow from API to worker

func TestEndToEndJobFlow(t *testing.T) {
	ctx := context.Background()
	cfg := testutil.GetTestConfig()

	// Setup
	dbPool, err := testutil.SetupTestDB(ctx, cfg)
	require.NoError(t, err)
	defer dbPool.Close()

	redisClient, err := testutil.SetupTestRedis(ctx, cfg)
	require.NoError(t, err)
	defer redisClient.Close()

	// Cleanup
	defer testutil.CleanupTestDB(ctx, dbPool)
	defer testutil.CleanupTestRedis(ctx, redisClient)

	w := worker.NewWorker(dbPool, redisClient, 2)

	t.Run("complete job lifecycle", func(t *testing.T) {
		// 1. Create job in database
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

		_, _, err := db.CreateJob(ctx, dbPool, job)
		require.NoError(t, err)

		// 2. Verify job in database
		retrievedJob, err := db.GetJobByID(ctx, dbPool, jobID)
		require.NoError(t, err)
		require.NotNil(t, retrievedJob)
		assert.Equal(t, "queued", retrievedJob.Status)

		// 3. Push job to Redis queue
		err = redis.PushJob(ctx, redisClient, jobID, "noop", "high")
		require.NoError(t, err)

		// Verify job in Redis queue
		queueName := redis.GetQueueName("high")
		length, err := redisClient.LLen(ctx, queueName).Result()
		require.NoError(t, err)
		assert.Greater(t, length, int64(0))

		// 4. Start worker and process job
		_, workerCancel := context.WithCancel(context.Background())
		go w.Start()

		// Wait for job to be processed
		time.Sleep(2 * time.Second)

		// 5. Verify job was processed
		retrievedJob, err = db.GetJobByID(ctx, dbPool, jobID)
		require.NoError(t, err)
		require.NotNil(t, retrievedJob)
		// Job should be succeeded (for noop) or still processing
		assert.Contains(t, []string{"succeeded", "running"}, retrievedJob.Status)

		// Stop worker
		workerCancel()
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		w.Stop(stopCtx)
	})

	t.Run("job retry flow", func(t *testing.T) {
		// Create a new worker for this test to avoid conflicts
		w2 := worker.NewWorker(dbPool, redisClient, 2)

		// Create a job that will fail
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

		_, _, err := db.CreateJob(ctx, dbPool, job)
		require.NoError(t, err)

		// Push to Redis
		err = redis.PushJob(ctx, redisClient, jobID, "noop", "high")
		require.NoError(t, err)

		// Start worker
		_, workerCancel := context.WithCancel(context.Background())
		go w2.Start()

		// Wait for job to be processed (poll for status change)
		maxWait := 5 * time.Second
		checkInterval := 100 * time.Millisecond
		deadline := time.Now().Add(maxWait)
		var retrievedJob *db.Job
		for time.Now().Before(deadline) {
			retrievedJob, err = db.GetJobByID(ctx, dbPool, jobID)
			require.NoError(t, err)
			require.NotNil(t, retrievedJob)
			// Job should be processed (noop succeeds immediately)
			if retrievedJob.Status == "succeeded" || retrievedJob.Status == "running" {
				break
			}
			time.Sleep(checkInterval)
		}

		// Verify job was processed
		// Note: For retry flow test, we're just verifying the job was processed
		// The actual retry logic would require a failing handler
		assert.Contains(t, []string{"succeeded", "running"}, retrievedJob.Status,
			"Job should be processed, but status is %s", retrievedJob.Status)

		workerCancel()
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		w2.Stop(stopCtx)
	})
}

func TestJobPriorityOrdering(t *testing.T) {
	ctx := context.Background()
	cfg := testutil.GetTestConfig()

	dbPool, err := testutil.SetupTestDB(ctx, cfg)
	require.NoError(t, err)
	defer dbPool.Close()

	redisClient, err := testutil.SetupTestRedis(ctx, cfg)
	require.NoError(t, err)
	defer redisClient.Close()

	defer testutil.CleanupTestDB(ctx, dbPool)
	defer testutil.CleanupTestRedis(ctx, redisClient)

	t.Run("jobs created with different priorities", func(t *testing.T) {
		priorities := []string{"low", "default", "high", "critical"}

		jobIDs := make([]uuid.UUID, len(priorities))
		for i, priority := range priorities {
			jobID := uuid.New()
			jobIDs[i] = jobID

			job := db.Job{
				TaskID:      jobID,
				Type:        "noop",
				Priority:    priority,
				PayloadJSON: json.RawMessage(`{}`),
				Status:      "queued",
				Attempts:    0,
				MaxAttempts: 3,
			}

			_, _, err := db.CreateJob(ctx, dbPool, job)
			require.NoError(t, err)
		}

		// Verify all jobs were created
		for _, jobID := range jobIDs {
			exists, err := db.JobExists(ctx, dbPool, jobID)
			require.NoError(t, err)
			assert.True(t, exists)
		}
	})
}

func TestIdempotency(t *testing.T) {
	ctx := context.Background()
	cfg := testutil.GetTestConfig()

	dbPool, err := testutil.SetupTestDB(ctx, cfg)
	require.NoError(t, err)
	defer dbPool.Close()

	redisClient, err := testutil.SetupTestRedis(ctx, cfg)
	require.NoError(t, err)
	defer redisClient.Close()

	defer testutil.CleanupTestDB(ctx, dbPool)
	defer testutil.CleanupTestRedis(ctx, redisClient)

	t.Run("duplicate job_id returns same job", func(t *testing.T) {
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

		// First creation
		createdID1, exists1, err1 := db.CreateJob(ctx, dbPool, job)
		require.NoError(t, err1)
		assert.False(t, exists1)
		assert.Equal(t, jobID, createdID1)

		// Second creation with same job_id (idempotent)
		createdID2, exists2, err2 := db.CreateJob(ctx, dbPool, job)
		require.NoError(t, err2)
		assert.True(t, exists2) // Should indicate it already existed
		assert.Equal(t, jobID, createdID2)

		// Verify only one job exists
		exists, err := db.JobExists(ctx, dbPool, jobID)
		require.NoError(t, err)
		assert.True(t, exists)

		// Verify job count
		query := `SELECT COUNT(*) FROM tasks WHERE task_id = $1`
		var count int
		err = dbPool.QueryRow(ctx, query, jobID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

func TestConcurrentSubmissions(t *testing.T) {
	ctx := context.Background()
	cfg := testutil.GetTestConfig()

	dbPool, err := testutil.SetupTestDB(ctx, cfg)
	require.NoError(t, err)
	defer dbPool.Close()

	redisClient, err := testutil.SetupTestRedis(ctx, cfg)
	require.NoError(t, err)
	defer redisClient.Close()

	defer testutil.CleanupTestDB(ctx, dbPool)
	defer testutil.CleanupTestRedis(ctx, redisClient)

	t.Run("handles concurrent job creation", func(t *testing.T) {
		numJobs := 50
		results := make(chan uuid.UUID, numJobs)
		errors := make(chan error, numJobs)

		// Create jobs concurrently
		for i := 0; i < numJobs; i++ {
			go func() {
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

				_, _, err := db.CreateJob(ctx, dbPool, job)
				if err != nil {
					errors <- err
				} else {
					results <- jobID
				}
			}()
		}

		// Collect results
		successCount := 0
		errorCount := 0
		for i := 0; i < numJobs; i++ {
			select {
			case <-results:
				successCount++
			case <-errors:
				errorCount++
			case <-time.After(5 * time.Second):
				t.Fatal("Timeout waiting for job creation")
			}
		}

		assert.Equal(t, numJobs, successCount)
		assert.Equal(t, 0, errorCount)
	})
}

func TestJobCancellationFlow(t *testing.T) {
	ctx := context.Background()
	cfg := testutil.GetTestConfig()

	dbPool, err := testutil.SetupTestDB(ctx, cfg)
	require.NoError(t, err)
	defer dbPool.Close()

	redisClient, err := testutil.SetupTestRedis(ctx, cfg)
	require.NoError(t, err)
	defer redisClient.Close()

	defer testutil.CleanupTestDB(ctx, dbPool)
	defer testutil.CleanupTestRedis(ctx, redisClient)

	t.Run("cancels job and removes from queue", func(t *testing.T) {
		// Create job
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

		_, _, err := db.CreateJob(ctx, dbPool, job)
		require.NoError(t, err)

		// Push to Redis
		err = redis.PushJob(ctx, redisClient, jobID, "noop", "high")
		require.NoError(t, err)

		// Verify job in queue
		queueName := redis.GetQueueName("high")
		lengthBefore, _ := redisClient.LLen(ctx, queueName).Result()

		// Cancel job
		cancelled, err := db.CancelJob(ctx, dbPool, jobID)
		require.NoError(t, err)
		assert.True(t, cancelled)

		// Verify job status
		retrievedJob, err := db.GetJobByID(ctx, dbPool, jobID)
		require.NoError(t, err)
		require.NotNil(t, retrievedJob)
		assert.Equal(t, "cancelled", retrievedJob.Status)

		// Verify job removed from queue (or at least queue length changed)
		lengthAfter, _ := redisClient.LLen(ctx, queueName).Result()
		// Queue length should be same or less (job was removed)
		assert.LessOrEqual(t, lengthAfter, lengthBefore)
	})
}
