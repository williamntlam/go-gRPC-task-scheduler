package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	redisc "github.com/redis/go-redis/v9"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/db"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/redis"
)

// Worker processes jobs from Redis queues
type Worker struct {
	dbPool      *pgxpool.Pool
	redisClient *redisc.Client
	poolSize    int
}

func NewWorker(dbPool *pgxpool.Pool, redisClient *redisc.Client, poolSize int) *Worker {
	return &Worker{
		dbPool:      dbPool,
		redisClient: redisClient,
		poolSize:    poolSize,
	}
}

// Start begins processing jobs from Redis queues
func (w *Worker) Start() {
	ctx := context.Background()

	for {
		// Step 1: Pop job from priority queues (blocks until job available or timeout)
		result, err := w.redisClient.BRPop(ctx, 1*time.Second, "q:critical", "q:high", "q:default", "q:low").Result()
		
		// Handle timeout (no jobs available)
		if err == redisc.Nil {
			continue // Timeout, loop back and try again
		}
		
		// Handle other errors
		if err != nil {
			log.Printf("Error popping from queue: %v", err)
			time.Sleep(1 * time.Second) // Brief pause before retrying
			continue
		}

		// Step 2: Parse job payload JSON from result[1]
		// result[0] = queue name, result[1] = job payload JSON
		if len(result) < 2 {
			log.Printf("Invalid result from BRPOP: %v", result)
			continue
		}

		var jobPayload redis.JobPayload
		if err := json.Unmarshal([]byte(result[1]), &jobPayload); err != nil {
			log.Printf("Failed to unmarshal job payload: %v", err)
			continue
		}

		// Step 3: Parse job ID
		jobID, err := uuid.Parse(jobPayload.TaskID)
		if err != nil {
			log.Printf("Invalid job ID: %v", err)
			continue
		}

		// Step 4: Claim job in database (update status to 'running', increment attempts)
		claimed, err := w.claimJob(ctx, jobID)
		if err != nil {
			log.Printf("Failed to claim job %s: %v", jobID, err)
			continue
		}
		if !claimed {
			
			// Job was already claimed by another worker or doesn't exist
			log.Printf("Job %s was not available to claim (may have been claimed by another worker)", jobID)
			continue
		}

		// Step 5: Get full job details from database
		job, err := db.GetJobByID(ctx, w.dbPool, jobID)
		if err != nil {
			log.Printf("Failed to get job %s: %v", jobID, err)
			w.markJobFailed(ctx, jobID, fmt.Sprintf("Failed to get job: %v", err))
			continue
		}
		if job == nil {
			log.Printf("Job %s not found in database", jobID)
			continue
		}

		// Step 6: Execute job handler 
		handlerErr := w.executeHandler(ctx, job)

		// Step 7: Update job status based on result
		if handlerErr != nil {
			log.Printf("Job %s failed: %v", jobID, handlerErr)
			w.handleJobFailure(ctx, job, handlerErr)
		} else {
			log.Printf("Job %s completed successfully", jobID)
			w.markJobSucceeded(ctx, jobID)
		}
	}
}

// Stop gracefully stops the worker
// This is a skeleton implementation - you'll need to implement graceful shutdown logic
func (w *Worker) Stop(ctx context.Context) error {
	// TODO: Implement graceful shutdown
	// - Signal workers to stop processing new jobs
	// - Wait for in-flight jobs to complete (with timeout)
	// - Clean up resources
	return nil
}

// claimJob atomically updates job status to 'running' and increments attempts
// Returns true if job was successfully claimed, false if already claimed or not found
func (w *Worker) claimJob(ctx context.Context, jobID uuid.UUID) (bool, error) {
	query := `
		UPDATE tasks 
		SET status = 'running', attempts = attempts + 1, updated_at = now()
		WHERE task_id = $1 AND status = 'queued'
		RETURNING task_id
	`
	
	var updatedTaskID uuid.UUID
	err := w.dbPool.QueryRow(ctx, query, jobID).Scan(&updatedTaskID)
	
	if err != nil {
		if err == pgx.ErrNoRows {
			// Job was not in 'queued' status (already claimed or doesn't exist)
			return false, nil
		}
		return false, fmt.Errorf("failed to claim job: %w", err)
	}
	
	return true, nil
}

// markJobSucceeded updates job status to 'succeeded'
func (w *Worker) markJobSucceeded(ctx context.Context, jobID uuid.UUID) error {
	query := `
		UPDATE tasks 
		SET status = 'succeeded', updated_at = now()
		WHERE task_id = $1
	`
	
	_, err := w.dbPool.Exec(ctx, query, jobID)
	if err != nil {
		return fmt.Errorf("failed to mark job as succeeded: %w", err)
	}
	
	return nil
}

// markJobFailed updates job status to 'failed'
func (w *Worker) markJobFailed(ctx context.Context, jobID uuid.UUID, errorMsg string) error {
	query := `
		UPDATE tasks 
		SET status = 'failed', updated_at = now()
		WHERE task_id = $1
	`
	
	_, err := w.dbPool.Exec(ctx, query, jobID)
	if err != nil {
		return fmt.Errorf("failed to mark job as failed: %w", err)
	}
	
	return nil
}

// handleJobFailure handles job failure, checking if retry is needed
func (w *Worker) handleJobFailure(ctx context.Context, job *db.Job, err error) {
	// Check if we should retry
	if job.Attempts < job.MaxAttempts {
		// TODO: Calculate exponential backoff and set next_run_at
		// For now, just mark as failed
		log.Printf("Job %s failed (attempt %d/%d), will retry later", job.TaskID, job.Attempts, job.MaxAttempts)
		// TODO: Update status to 'retry' and set next_run_at
		w.markJobFailed(ctx, job.TaskID, err.Error())
	} else {
		// Max attempts reached, mark as failed
		log.Printf("Job %s failed after %d attempts, marking as failed", job.TaskID, job.Attempts)
		w.markJobFailed(ctx, job.TaskID, err.Error())
	}
}

// executeHandler executes the job handler based on job type
// This is a skeleton - you'll implement actual handlers later
func (w *Worker) executeHandler(ctx context.Context, job *db.Job) error {
	// TODO: Implement job handlers based on job.Type
	// For now, just log and return success for "noop" type
	if job.Type == "noop" {
		log.Printf("Executing noop handler for job %s", job.TaskID)
		return nil // Success
	}
	
	// Unknown handler type
	return fmt.Errorf("unknown job type: %s", job.Type)
}