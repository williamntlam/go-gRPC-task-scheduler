package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	redisc "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/redis"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/testutil"
)

var testRedisClient *redisc.Client

func init() {
	// Setup Redis client for tests
	ctx := context.Background()
	cfg := testutil.GetTestConfig()

	var err error
	testRedisClient, err = testutil.SetupTestRedis(ctx, cfg)
	if err != nil {
		panic(fmt.Sprintf("Failed to setup test redis: %v", err))
	}
}

func setupRedisTest(t *testing.T) {
	ctx := context.Background()
	if err := testutil.CleanupTestRedis(ctx, testRedisClient); err != nil {
		t.Fatalf("Failed to cleanup test redis: %v", err)
	}
}

func teardownRedisTest(t *testing.T) {
	ctx := context.Background()
	if err := testutil.CleanupTestRedis(ctx, testRedisClient); err != nil {
		t.Logf("Warning: Failed to cleanup test redis: %v", err)
	}
}

func TestGetQueueName(t *testing.T) {
	tests := []struct {
		name     string
		priority string
		expected string
	}{
		{"critical priority", "critical", "q:critical"},
		{"high priority", "high", "q:high"},
		{"default priority", "default", "q:default"},
		{"low priority", "low", "q:low"},
		{"invalid priority", "invalid", ""},
		{"empty priority", "", ""},
		{"mixed case", "HIGH", ""}, // Case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := redis.GetQueueName(tt.priority)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPushJob(t *testing.T) {
	ctx := context.Background()
	setupRedisTest(t)
	defer teardownRedisTest(t)

	t.Run("pushes job to correct queue", func(t *testing.T) {
		jobID := uuid.New()
		jobType := "noop"
		priority := "high"

		err := redis.PushJob(ctx, testRedisClient, jobID, jobType, priority)
		require.NoError(t, err)

		// Verify job was pushed to correct queue
		queueName := redis.GetQueueName(priority)
		length, err := testRedisClient.LLen(ctx, queueName).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(1), length)

		// Verify payload content
		payloadJSON, err := testRedisClient.RPop(ctx, queueName).Result()
		require.NoError(t, err)

		var payload redis.JobPayload
		err = json.Unmarshal([]byte(payloadJSON), &payload)
		require.NoError(t, err)
		assert.Equal(t, jobID.String(), payload.TaskID)
		assert.Equal(t, jobType, payload.Type)
		assert.Equal(t, priority, payload.Priority)
	})

	t.Run("pushes to different priority queues", func(t *testing.T) {
		priorities := []string{"critical", "high", "default", "low"}

		for i, priority := range priorities {
			jobID := uuid.New()
			err := redis.PushJob(ctx, testRedisClient, jobID, "noop", priority)
			require.NoError(t, err)

			queueName := redis.GetQueueName(priority)
			length, err := testRedisClient.LLen(ctx, queueName).Result()
			require.NoError(t, err)
			assert.Equal(t, int64(i+1), length)
		}
	})

	t.Run("returns error for invalid priority", func(t *testing.T) {
		jobID := uuid.New()
		err := redis.PushJob(ctx, testRedisClient, jobID, "noop", "invalid")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid priority")
	})

	t.Run("handles multiple jobs in same queue", func(t *testing.T) {
		priority := "default"
		queueName := redis.GetQueueName(priority)

		// Push multiple jobs
		for i := 0; i < 5; i++ {
			jobID := uuid.New()
			err := redis.PushJob(ctx, testRedisClient, jobID, "noop", priority)
			require.NoError(t, err)
		}

		// Verify all jobs are in queue
		length, err := testRedisClient.LLen(ctx, queueName).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(5), length)
	})

	t.Run("handles Redis connection errors", func(t *testing.T) {
		// Create invalid client
		invalidClient := redisc.NewClient(&redisc.Options{
			Addr: "localhost:9999", // Invalid address
		})
		defer invalidClient.Close()

		jobID := uuid.New()
		err := redis.PushJob(ctx, invalidClient, jobID, "noop", "high")
		assert.Error(t, err)
	})

	t.Run("handles context cancellation", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		jobID := uuid.New()
		err := redis.PushJob(cancelledCtx, testRedisClient, jobID, "noop", "high")
		assert.Error(t, err)
	})

	t.Run("preserves job payload structure", func(t *testing.T) {
		jobID := uuid.New()
		jobType := "http_call"
		priority := "critical"

		err := redis.PushJob(ctx, testRedisClient, jobID, jobType, priority)
		require.NoError(t, err)

		queueName := redis.GetQueueName(priority)
		payloadJSON, err := testRedisClient.RPop(ctx, queueName).Result()
		require.NoError(t, err)

		// Verify JSON structure
		var payload redis.JobPayload
		err = json.Unmarshal([]byte(payloadJSON), &payload)
		require.NoError(t, err)

		// Verify all fields are present
		assert.NotEmpty(t, payload.TaskID)
		assert.NotEmpty(t, payload.Type)
		assert.NotEmpty(t, payload.Priority)
		assert.Equal(t, jobID.String(), payload.TaskID)
		assert.Equal(t, jobType, payload.Type)
		assert.Equal(t, priority, payload.Priority)
	})
}

func TestPushJobConcurrency(t *testing.T) {
	ctx := context.Background()
	setupRedisTest(t)
	defer teardownRedisTest(t)

	t.Run("handles concurrent pushes to same queue", func(t *testing.T) {
		priority := "high"
		queueName := redis.GetQueueName(priority)
		numJobs := 100

		// Push jobs concurrently
		errors := make(chan error, numJobs)
		for i := 0; i < numJobs; i++ {
			go func() {
				jobID := uuid.New()
				err := redis.PushJob(ctx, testRedisClient, jobID, "noop", priority)
				errors <- err
			}()
		}

		// Collect errors
		errorCount := 0
		for i := 0; i < numJobs; i++ {
			if err := <-errors; err != nil {
				errorCount++
			}
		}

		assert.Equal(t, 0, errorCount, "No errors should occur during concurrent pushes")

		// Verify all jobs are in queue
		length, err := testRedisClient.LLen(ctx, queueName).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(numJobs), length)
	})

	t.Run("handles concurrent pushes to different queues", func(t *testing.T) {
		priorities := []string{"critical", "high", "default", "low"}
		jobsPerQueue := 25

		errors := make(chan error, len(priorities)*jobsPerQueue)
		for _, priority := range priorities {
			for i := 0; i < jobsPerQueue; i++ {
				go func(p string) {
					jobID := uuid.New()
					err := redis.PushJob(ctx, testRedisClient, jobID, "noop", p)
					errors <- err
				}(priority)
			}
		}

		// Collect errors
		errorCount := 0
		for i := 0; i < len(priorities)*jobsPerQueue; i++ {
			if err := <-errors; err != nil {
				errorCount++
			}
		}

		assert.Equal(t, 0, errorCount)

		// Verify each queue has correct number of jobs
		for _, priority := range priorities {
			queueName := redis.GetQueueName(priority)
			length, err := testRedisClient.LLen(ctx, queueName).Result()
			require.NoError(t, err)
			assert.Equal(t, int64(jobsPerQueue), length)
		}
	})
}

func TestJobPayload(t *testing.T) {
	t.Run("marshals and unmarshals correctly", func(t *testing.T) {
		payload := redis.JobPayload{
			TaskID:   "123e4567-e89b-12d3-a456-426614174000",
			Type:     "http_call",
			Priority: "high",
		}

		jsonData, err := json.Marshal(payload)
		require.NoError(t, err)

		var unmarshaled redis.JobPayload
		err = json.Unmarshal(jsonData, &unmarshaled)
		require.NoError(t, err)

		assert.Equal(t, payload.TaskID, unmarshaled.TaskID)
		assert.Equal(t, payload.Type, unmarshaled.Type)
		assert.Equal(t, payload.Priority, unmarshaled.Priority)
	})

	t.Run("handles empty fields", func(t *testing.T) {
		payload := redis.JobPayload{
			TaskID:   "",
			Type:     "",
			Priority: "",
		}

		jsonData, err := json.Marshal(payload)
		require.NoError(t, err)

		var unmarshaled redis.JobPayload
		err = json.Unmarshal(jsonData, &unmarshaled)
		require.NoError(t, err)

		assert.Empty(t, unmarshaled.TaskID)
		assert.Empty(t, unmarshaled.Type)
		assert.Empty(t, unmarshaled.Priority)
	})
}
