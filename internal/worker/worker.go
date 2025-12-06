package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"sync" // STEP 1: Added sync package for WaitGroup and Mutex
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
	
	// STEP 2: Graceful shutdown fields
	// - ctx: A cancellable context.Context to signal shutdown
	// - cancel: The CancelFunc to cancel the context
	// - mu: A sync.Mutex to protect concurrent access to stopping flag
	// - wg: A sync.WaitGroup to track in-flight jobs
	// - stopping: A bool flag to prevent multiple stop calls
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	wg       sync.WaitGroup
	stopping bool
}

func NewWorker(dbPool *pgxpool.Pool, redisClient *redisc.Client, poolSize int) *Worker {
	// STEP 3: Initialize the cancellable context in NewWorker
	// Use context.WithCancel(context.Background()) to create ctx and cancel
	// Then include ctx and cancel in the Worker struct initialization
	// Example:
	// ctx, cancel := context.WithCancel(context.Background())
	// return &Worker{
	//     dbPool:      dbPool,
	//     redisClient: redisClient,
	//     poolSize:    poolSize,
	//     ctx:         ctx,
	//     cancel:      cancel,
	// }

	ctx, cancel := context.WithCancel(context.Background())

	return &Worker{
		dbPool: dbPool,
		redisClient: redisClient,
		poolSize: poolSize,
		ctx: ctx,
		cancel: cancel,
	}
}

// checkShutdown checks if the context has been cancelled and logs if so.
// Returns true if shutdown was detected, false otherwise.
func (w *Worker) checkShutdown(ctx context.Context) bool {
	if ctx.Err() != nil {
		log.Println("Worker received shutdown signal, stopping job processing")
		return true
	}
	return false
}

// Start begins processing jobs from Redis queues
func (w *Worker) Start() {
	// STEP 4: Change from context.Background() to use w.ctx
	// Get the worker's context (you'll need to lock/unlock the mutex to read it safely)
	// Example:
	// w.mu.Lock()
	// ctx := w.ctx
	// w.mu.Unlock()
	w.mu.Lock()
	ctx := w.ctx
	w.mu.Unlock()

	log.Printf("[Worker] Starting worker with pool size: %d", w.poolSize)

	// Start reaper process to recover stuck jobs from processing queue
	go w.reaper(ctx)
	go w.retryPump(ctx)
	log.Printf("[Worker] Background processes started: reaper and retry pump")

	for {
		// STEP 5: Add shutdown check at the start of the loop
		// Use select with ctx.Done() to check if shutdown was signaled
		// If ctx.Done() is received, log and return from Start()
		// Example:
		// select {
		// case <-ctx.Done():
		//     log.Println("Worker received shutdown signal, stopping job processing")
		//     return
		// default:
		// }

		if w.checkShutdown(ctx) {
			return
		}
		
		// Step 1: Reliable queue pattern - atomically move job from pending to processing queue
		// Use BRPOPLPUSH to pop from priority queues and push to processing queue in one atomic operation
		// This prevents job loss if worker crashes between pop and claim
		processingQueue := "q:processing"
		var jobPayloadJSON string
		var sourceQueue string
		
		// Try each priority queue in order (critical -> high -> default -> low)
		priorityQueues := []string{"q:critical", "q:high", "q:default", "q:low"}
		found := false
		
		for _, queue := range priorityQueues {
			if w.checkShutdown(ctx) {
				return
			}
			
			// BRPOPLPUSH: atomically pops from source queue and pushes to processing queue
			// Blocks for up to 1 second per queue
			result := w.redisClient.BRPopLPush(ctx, queue, processingQueue, 1*time.Second)
			if err := result.Err(); err != nil {
				if err == redisc.Nil {
					// Queue is empty, try next queue
					continue
				}
				log.Printf("Error in BRPOPLPUSH from %s: %v", queue, err)
				continue
			}
			
			// Successfully moved job to processing queue
			jobPayloadJSON = result.Val()
			sourceQueue = queue
			found = true
			break
		}
		
		// If no job found in any queue, check for shutdown and continue
		if !found {
			if w.checkShutdown(ctx) {
				return
			}
			continue // No jobs available, loop back
		}

		// Step 2: Parse job payload JSON
		var jobPayload redis.JobPayload
		if err := json.Unmarshal([]byte(jobPayloadJSON), &jobPayload); err != nil {
			log.Printf("[Worker] Failed to unmarshal job payload: error: %v", err)
			// Move job back to original queue on parse error
			w.redisClient.LPush(ctx, sourceQueue, jobPayloadJSON)
			w.redisClient.LRem(ctx, processingQueue, 1, jobPayloadJSON)
			continue
		}

		// Step 3: Parse job ID
		jobID, err := uuid.Parse(jobPayload.TaskID)
		if err != nil {
			log.Printf("[Worker] Invalid job ID in payload: task_id=%s, error: %v", jobPayload.TaskID, err)
			// Remove invalid job from processing queue (data corruption, don't retry)
			w.redisClient.LRem(ctx, processingQueue, 1, jobPayloadJSON)
			continue
		}

		log.Printf("[Worker] Processing job: job_id=%s, type=%s, priority=%s", jobID, jobPayload.Type, jobPayload.Priority)

		// STEP 8: Add shutdown check before processing a job
		// After parsing the job ID but before claiming it, check if shutdown was signaled
		// If so, log and return (don't process this job)

		if w.checkShutdown(ctx) {
			return
		}

		// Step 4: Claim job in database (update status to 'running', increment attempts)
		claimed, err := w.claimJob(ctx, jobID)
		if err != nil {
			log.Printf("[Worker] Failed to claim job: job_id=%s, error: %v", jobID, err)
			// Move job back to original queue and remove from processing queue
			w.redisClient.LPush(ctx, sourceQueue, jobPayloadJSON)
			w.redisClient.LRem(ctx, processingQueue, 1, jobPayloadJSON)
			continue
		}
		if !claimed {
			// Job was already claimed by another worker or doesn't exist
			log.Printf("[Worker] Job not available to claim (may have been claimed by another worker): job_id=%s", jobID)
			// Remove from processing queue (another worker is handling it)
			w.redisClient.LRem(ctx, processingQueue, 1, jobPayloadJSON)
			continue
		}

		log.Printf("[Worker] Job claimed successfully: job_id=%s", jobID)

		// Step 5: Get full job details from database
		job, err := db.GetJobByID(ctx, w.dbPool, jobID)
		if err != nil {
			log.Printf("[Worker] Failed to get job from database: job_id=%s, error: %v", jobID, err)
			w.markJobFailed(ctx, jobID, fmt.Sprintf("Failed to get job: %v", err))
			// Remove from processing queue
			w.redisClient.LRem(ctx, processingQueue, 1, jobPayloadJSON)
			continue
		}
		if job == nil {
			log.Printf("[Worker] Job not found in database: job_id=%s", jobID)
			// Remove from processing queue
			w.redisClient.LRem(ctx, processingQueue, 1, jobPayloadJSON)
			continue
		}

		log.Printf("[Worker] Job details retrieved: job_id=%s, status=%s, attempts=%d/%d", jobID, job.Status, job.Attempts, job.MaxAttempts)

		// STEP 9: Track in-flight jobs and execute in goroutine
		// Before executing the handler, call w.wg.Add(1) to increment the WaitGroup
		// Then execute the handler in a goroutine, and use defer w.wg.Done() to decrement when done
		// This allows Stop() to wait for all in-flight jobs to complete
		// Example structure:
		// w.wg.Add(1)
		// go func(job *db.Job) {
		//     defer w.wg.Done()
		//     handlerErr := w.executeHandler(ctx, job)
		//     // ... update job status ...
		// }(job)

		// Step 6: Execute job handler in goroutine and update status
		w.wg.Add(1)
		go func(job *db.Job, payloadJSON string, originalQueue string) {
			defer w.wg.Done()
			
			// Clean up processing queue after job completes (success or failure)
			defer func() {
				// Remove job from processing queue
				w.redisClient.LRem(ctx, processingQueue, 1, payloadJSON)
			}()
			
			log.Printf("[Worker] Executing handler: job_id=%s, type=%s", job.TaskID, job.Type)
			handlerErr := w.executeHandler(ctx, job)

			// Step 7: Update job status based on result
			if handlerErr != nil {
				log.Printf("[Worker] Job execution failed: job_id=%s, error: %v", job.TaskID, handlerErr)
				w.handleJobFailure(ctx, job, handlerErr)
				// Note: Job remains in processing queue until defer removes it
				// If retry is needed, handleJobFailure will update next_run_at in DB
				// A separate process can monitor processing queue for stuck jobs
			} else {
				log.Printf("[Worker] Job execution succeeded: job_id=%s", job.TaskID)
				w.markJobSucceeded(ctx, job.TaskID)
				// Job will be removed from processing queue by defer
			}
		}(job, jobPayloadJSON, sourceQueue)
	}
}

// Stop gracefully stops the worker
func (w *Worker) Stop(ctx context.Context) error {
	// STEP 10: Prevent multiple simultaneous stop calls

	w.mu.Lock()
	if w.stopping {
		w.mu.Unlock()
		return fmt.Errorf("worker is already in the process of stopping")
	}
	w.stopping = true
	w.mu.Unlock()
	
	// STEP 11: Signal workers to stop processing new jobs

	log.Println("Signaling worker to stop processing new jobs.")
	w.cancel()
	
	// STEP 12: Wait for in-flight jobs to complete (with timeout)
	
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("All in-flight jobs completed")
		return nil
	case <-ctx.Done():
		log.Printf("Shutdown timeout reached: %v", ctx.Err())
		return fmt.Errorf("shutdown timeout: %w", ctx.Err())
	}
}

// reaper periodically checks the processing queue for stuck jobs and moves them back to pending queues
// A job is considered "stuck" if it's been in the processing queue longer than the timeout (default: 5 minutes)
func (w *Worker) reaper(ctx context.Context) {
	processingQueue := "q:processing"
	stuckJobTimeout := 5 * time.Minute // Jobs stuck longer than this will be recovered
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	log.Println("[Reaper] Reaper process started: monitoring processing queue for stuck jobs")

	for {
		select {
		case <-ctx.Done():
			log.Println("[Reaper] Reaper process stopping")
			return
		case <-ticker.C:
			w.recoverStuckJobs(ctx, processingQueue, stuckJobTimeout)
		}
	}
}

// recoverStuckJobs checks the processing queue and moves stuck jobs back to their original priority queues
func (w *Worker) recoverStuckJobs(ctx context.Context, processingQueue string, timeout time.Duration) {
	// Get all items from processing queue
	items, err := w.redisClient.LRange(ctx, processingQueue, 0, -1).Result()
	if err != nil {
		log.Printf("[Reaper] Error reading processing queue: error: %v", err)
		return
	}

	if len(items) == 0 {
		return // No jobs in processing queue
	}

	log.Printf("[Reaper] Checking processing queue for stuck jobs: queue=%s, items=%d, timeout=%v", processingQueue, len(items), timeout)
	recoveredCount := 0
	for _, itemJSON := range items {
		// Parse job payload to get priority
		var jobPayload redis.JobPayload
		if err := json.Unmarshal([]byte(itemJSON), &jobPayload); err != nil {
			log.Printf("Reaper: Failed to parse job payload, removing invalid job: %v", err)
			// Remove invalid job from processing queue
			w.redisClient.LRem(ctx, processingQueue, 1, itemJSON)
			continue
		}

		// Check if job is actually stuck by checking database status
		// If job status is 'running' and it's been there a while, it might be stuck
		// For simplicity, we'll move all jobs in processing queue back if they're not actively being processed
		// A more sophisticated approach would track when jobs entered processing queue
		
		// Get the original priority queue name
		originalQueue := redis.GetQueueName(jobPayload.Priority)
		if originalQueue == "" {
			log.Printf("[Reaper] Invalid priority, removing from processing queue: job_id=%s, priority=%s", jobPayload.TaskID, jobPayload.Priority)
			w.redisClient.LRem(ctx, processingQueue, 1, itemJSON)
			continue
		}

		// Check job status in database
		jobID, err := uuid.Parse(jobPayload.TaskID)
		if err != nil {
			log.Printf("[Reaper] Invalid job ID, removing from processing queue: task_id=%s, error: %v", jobPayload.TaskID, err)
			w.redisClient.LRem(ctx, processingQueue, 1, itemJSON)
			continue
		}

		// Check if job is still in 'running' status (might be stuck)
		// If status changed, another worker might have handled it
		job, err := db.GetJobByID(ctx, w.dbPool, jobID)
		if err != nil {
			log.Printf("Reaper: Error checking job %s status: %v", jobID, err)
			// Move back to original queue on error
			w.moveJobBackToQueue(ctx, processingQueue, originalQueue, itemJSON)
			recoveredCount++
			continue
		}

		if job == nil {
			// Job doesn't exist in database, remove from processing queue
			log.Printf("Reaper: Job %s not found in database, removing from processing queue", jobID)
			w.redisClient.LRem(ctx, processingQueue, 1, itemJSON)
			continue
		}

		// If job is still 'running' and has been running for a while, it might be stuck
		// Check if job has been running longer than timeout
		switch job.Status {
		case "running":
			// Check how long job has been running (using updated_at as proxy)
			timeSinceUpdate := time.Since(job.UpdatedAt)
			if timeSinceUpdate > timeout {
				log.Printf("[Reaper] Recovering stuck job: job_id=%s, running_for=%v, moving_back_to=%s", jobID, timeSinceUpdate, originalQueue)
				w.moveJobBackToQueue(ctx, processingQueue, originalQueue, itemJSON)
				recoveredCount++
			}
		case "queued":
			// Job status changed back to queued (shouldn't happen, but recover it)
			log.Printf("[Reaper] Job status is queued but in processing queue, moving back: job_id=%s, queue=%s", jobID, originalQueue)
			w.moveJobBackToQueue(ctx, processingQueue, originalQueue, itemJSON)
			recoveredCount++
		case "succeeded", "failed", "retry":
			// If job status is 'succeeded', 'failed', or 'retry', remove from processing queue
			// (should have been removed by worker, but clean up if not)
			log.Printf("[Reaper] Cleaning up completed job from processing queue: job_id=%s, status=%s", jobID, job.Status)
			w.redisClient.LRem(ctx, processingQueue, 1, itemJSON)
		}
	}

	if recoveredCount > 0 {
		log.Printf("[Reaper] Recovered %d stuck job(s) from processing queue", recoveredCount)
	}
}

// moveJobBackToQueue atomically moves a job from processing queue back to its original priority queue
func (w *Worker) moveJobBackToQueue(ctx context.Context, processingQueue, targetQueue, jobJSON string) {
	// Use LMOVE to atomically move the job
	// LMOVE processingQueue targetQueue RIGHT LEFT (move from right of source to left of target)
	err := w.redisClient.LMove(ctx, processingQueue, targetQueue, "RIGHT", "LEFT").Err()
	if err != nil {
		// Fallback: manual move if LMOVE fails
		log.Printf("Reaper: LMOVE failed, using manual move: %v", err)
		w.redisClient.LPush(ctx, targetQueue, jobJSON)
		w.redisClient.LRem(ctx, processingQueue, 1, jobJSON)
	}
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
	
	// STEP: Insert task attempt record
	// After successfully claiming the job, insert a record into task_attempts table
	// This tracks when the job execution started
	// Call db.InsertAttempt(ctx, w.dbPool, jobID) to insert the attempt
	// Handle errors: log error but don't fail the claim (attempt tracking is for audit/debugging)
	// The job is already claimed, so we continue even if attempt insertion fails
	// Example: if attemptErr := db.InsertAttempt(ctx, w.dbPool, jobID); attemptErr != nil {
	//              log.Printf("Warning: Failed to insert task attempt for job %s: %v", jobID, attemptErr)
	//              // Continue anyway - job is claimed, attempt tracking is secondary
	//          }
	
	if attemptErr := db.InsertAttempt(ctx, w.dbPool, jobID); attemptErr != nil {
		log.Printf("Warning: Failed to insert task attempt for job %s: %v", jobID, attemptErr)
		// Continue anyway - job is claimed, attempt tracking is secondary
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
		log.Printf("[Worker] Failed to mark job as succeeded in database: job_id=%s, error: %v", jobID, err)
		return fmt.Errorf("failed to mark job as succeeded: %w", err)
	}
	
	log.Printf("[Worker] Job marked as succeeded: job_id=%s", jobID)
	
	// STEP: Update task attempt record on success
	// After marking job as succeeded, update the task_attempts record
	// This marks the attempt as completed successfully
	// Call db.UpdateAttemptOnSuccess(ctx, w.dbPool, jobID) to update the attempt
	// Handle errors: log error but don't fail (attempt tracking is for audit/debugging)
	// The job is already marked as succeeded, so we continue even if attempt update fails
	// Example: if attemptErr := db.UpdateAttemptOnSuccess(ctx, w.dbPool, jobID); attemptErr != nil {
	//              log.Printf("Warning: Failed to update task attempt for job %s: %v", jobID, attemptErr)
	//              // Continue anyway - job is succeeded, attempt tracking is secondary
	//          }
	
	if attemptErr := db.UpdateAttemptOnSuccess(ctx, w.dbPool, jobID); attemptErr != nil {
		log.Printf("[Worker] Warning: Failed to update task attempt for job: job_id=%s, error: %v", jobID, attemptErr)
		// Continue anyway - job is succeeded, attempt tracking is secondary
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
		log.Printf("[Worker] Failed to mark job as failed in database: job_id=%s, error: %v", jobID, err)
		return fmt.Errorf("failed to mark job as failed: %w", err)
	}
	
	log.Printf("[Worker] Job marked as failed: job_id=%s, error: %s", jobID, errorMsg)
	return nil
}

// markJobDeadLetter updates job status to 'deadletter' in the database
// This is called when a job has exceeded max_attempts and is moved to DLQ
func (w *Worker) markJobDeadLetter(ctx context.Context, jobID uuid.UUID, errorMsg string) error {
	// STEP 1: Build SQL UPDATE query
	// Query should:
	//   - UPDATE tasks SET status = 'deadletter', updated_at = now()
	//   - WHERE task_id = $1
	// Similar to markJobFailed() but with status = 'deadletter'
	// Example query:
	//   UPDATE tasks 
	//   SET status = 'deadletter', updated_at = now()
	//   WHERE task_id = $1

	query := `
		UPDATE tasks 
		SET status = 'deadletter', updated_at = now()
		WHERE task_id = $1
	`
	// STEP 2: Execute the UPDATE query
	// Use w.dbPool.Exec(ctx, query, jobID) to execute
	// Handle errors: return wrapped error
	// Example: _, err := w.dbPool.Exec(ctx, query, jobID)
	//          if err != nil {
	//              return fmt.Errorf("failed to mark job as deadletter: %w", err)
	//          }

	_, err := w.dbPool.Exec(ctx, query, jobID)
	if err != nil {
		log.Printf("[Worker] Failed to mark job as deadletter in database: job_id=%s, error: %v", jobID, err)
		return fmt.Errorf("failed to mark job as deadletter: %w", err)
	}

	log.Printf("[Worker] Job marked as deadletter: job_id=%s, error: %s", jobID, errorMsg)

	return nil
}

// markJobRetry updates job status to 'retry' and sets next_run_at for retry scheduling
func (w *Worker) markJobRetry(ctx context.Context, jobID uuid.UUID, errorMsg string, nextRunAt time.Time) error {
	query := `
		UPDATE tasks 
		SET status = 'retry', next_run_at = $2, updated_at = now()
		WHERE task_id = $1
	`
	
	_, err := w.dbPool.Exec(ctx, query, jobID, nextRunAt)
	if err != nil {
		log.Printf("[Worker] Failed to mark job as retry in database: job_id=%s, error: %v", jobID, err)
		return fmt.Errorf("failed to mark job as retry: %w", err)
	}
	
	log.Printf("[Worker] Job marked for retry: job_id=%s, next_run_at=%s, error: %s", jobID, nextRunAt.Format(time.RFC3339), errorMsg)
	return nil
}

// handleJobFailure handles job failure, checking if retry is needed
func (w *Worker) handleJobFailure(ctx context.Context, job *db.Job, err error) {
	log.Printf("[Worker] Handling job failure: job_id=%s, attempts=%d/%d, error: %v", job.TaskID, job.Attempts, job.MaxAttempts, err)

	// STEP 1: Check if job should be retried
	// Compare job.Attempts with job.MaxAttempts
	// If job.Attempts < job.MaxAttempts, we should retry
	// Otherwise, mark as permanently failed
	if job.Attempts >= job.MaxAttempts {
		// Max attempts reached - move job to Dead Letter Queue (DLQ)
		// STEP 1: Create Redis job payload for DLQ
		// Create a redis.JobPayload struct with:
		//   - TaskID: job.TaskID.String() (convert UUID to string)
		//   - Type: job.Type
		//   - Priority: job.Priority
		// Example:
		//   payload := redis.JobPayload{
		//       TaskID:   job.TaskID.String(),
		//       Type:     job.Type,
		//       Priority: job.Priority,
		//   }

		// TODO: Implement STEP 1 - Create payload struct
		// payload := redis.JobPayload{
		//     TaskID:   job.TaskID.String(),
		//     Type:     job.Type,
		//     Priority: job.Priority,
		// }

		payload := redis.JobPayload{
			TaskID: job.TaskID.String(),
			Type: job.Type,
			Priority: job.Priority,
		}

		// STEP 2: Marshal payload to JSON
		// Use json.Marshal(payload) to convert struct to JSON bytes
		// Handle marshaling errors: log error but continue (we still want to update DB)
		// Example: payloadJSON, marshalErr := json.Marshal(payload)
		//          if marshalErr != nil {
		//              log.Printf("DLQ: Failed to marshal payload for job %s: %v", job.TaskID, marshalErr)
		//          }

		// Save original error message before it gets shadowed
		originalErrMsg := err.Error()

		payloadJSON, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			log.Printf("[Worker] DLQ: Failed to marshal payload: job_id=%s, error: %v", job.TaskID, marshalErr)
			// Still try to update DB even if marshaling fails
		} else {
			// STEP 3: Get DLQ queue name
			// DLQ queues follow the pattern: "dlq:{priority}"
			// Examples: "dlq:critical", "dlq:high", "dlq:default", "dlq:low"
			// Build the queue name: "dlq:" + job.Priority
			// Example: dlqQueueName := "dlq:" + job.Priority

			dlqQueueName := "dlq:" + job.Priority

			// STEP 4: Push job to DLQ Redis queue
			// Use w.redisClient.LPush(ctx, dlqQueueName, payloadJSON) to push to DLQ
			// Handle errors: log error but continue (we still want to update DB status)
			// Note: Even if Redis push fails, we still update DB to 'deadletter'
			// This ensures the job is tracked in the database even if Redis fails
			// Example: if pushErr := w.redisClient.LPush(ctx, dlqQueueName, payloadJSON).Err(); pushErr != nil {
			//              log.Printf("DLQ: Failed to push job %s to DLQ queue %s: %v", job.TaskID, dlqQueueName, pushErr)
			//          } else {
			//              log.Printf("DLQ: Pushed job %s to DLQ queue %s", job.TaskID, dlqQueueName)
			//          }

			if pushErr := w.redisClient.LPush(ctx, dlqQueueName, payloadJSON).Err(); pushErr != nil {
				log.Printf("[Worker] DLQ: Failed to push job to DLQ queue: job_id=%s, queue=%s, error: %v", job.TaskID, dlqQueueName, pushErr)
			} else {
				log.Printf("[Worker] DLQ: Pushed job to DLQ queue: job_id=%s, queue=%s", job.TaskID, dlqQueueName)
			}
		}

		// STEP 5: Update database status to 'deadletter'
		// Call w.markJobDeadLetter(ctx, job.TaskID, err.Error()) to update DB
		// This function should update status to 'deadletter' (not 'failed')
		// Pass the original error message (err.Error()) for debugging
		// Example: if dbErr := w.markJobDeadLetter(ctx, job.TaskID, err.Error()); dbErr != nil {
		//              log.Printf("DLQ: Failed to mark job %s as deadletter in DB: %v", job.TaskID, dbErr)
		//          }

		if dbErr := w.markJobDeadLetter(ctx, job.TaskID, originalErrMsg); dbErr != nil {
			log.Printf("[Worker] DLQ: Failed to mark job as deadletter in DB: job_id=%s, error: %v", job.TaskID, dbErr)
		}

		// STEP 5.5: Update task attempt record on failure (DLQ path)
		// After marking job as deadletter, update the task_attempts record
		// This marks the attempt as completed with failure
		// Call db.UpdateAttemptOnFailure(ctx, w.dbPool, job.TaskID, originalErrMsg) to update the attempt
		// Handle errors: log error but don't fail (attempt tracking is for audit/debugging)
		// The job is already in DLQ, so we continue even if attempt update fails
		// Example: if attemptErr := db.UpdateAttemptOnFailure(ctx, w.dbPool, job.TaskID, originalErrMsg); attemptErr != nil {
		//              log.Printf("Warning: Failed to update task attempt for job %s: %v", job.TaskID, attemptErr)
		//              // Continue anyway - job is in DLQ, attempt tracking is secondary
		//          }
		
		if attemptErr := db.UpdateAttemptOnFailure(ctx, w.dbPool, job.TaskID, originalErrMsg); attemptErr != nil {
			log.Printf("[Worker] Warning: Failed to update task attempt: job_id=%s, error: %v", job.TaskID, attemptErr)
			// Continue anyway - job is in DLQ, attempt tracking is secondary
		}

		// STEP 6: Log the DLQ event
		// Log that the job has been moved to DLQ with attempt count
		// Example: log.Printf("Job %s moved to DLQ after %d attempts (max: %d)", job.TaskID, job.Attempts, job.MaxAttempts)

		log.Printf("Job %s moved to DLQ after %d attempts (max: %d)", job.TaskID, job.Attempts, job.MaxAttempts)

		// STEP 7: Return (job is now in DLQ, no retry)
		// Return from function - job is permanently failed and in DLQ
		// Example: return
		return
	} 

	// STEP 2: If retry is needed (attempts < max attempts):
	//   a. Calculate exponential backoff delay
	//      - Formula: delay = baseDelay * (2 ^ (attempts - 1))
	//      - Use time.Duration for the delay (e.g., 1 second, 2 seconds, 4 seconds, 8 seconds...)
	//      - Example: baseDelay = 1 * time.Second
	//      - For attempt 1: 1s, attempt 2: 2s, attempt 3: 4s, attempt 4: 8s
	//   b. Calculate next_run_at = current time + delay
	//      - Use time.Now() to get current time
	//      - Add the calculated delay to get next_run_at
	//   c. Log that the job will be retried with attempt count
	//      - Use log.Printf with job.TaskID, job.Attempts, job.MaxAttempts
	//   d. Call a helper function to update job status to 'retry' with next_run_at
	//      - You'll need to create markJobRetry() function (similar to markJobSucceeded/markJobFailed)
	//      - Pass: ctx, job.TaskID, error message, and next_run_at time

	// Calculate exponential backoff: baseDelay * 2^(attempts-1)
	// Use bit shifting for powers of 2: 1 << (attempts-1) = 2^(attempts-1)
	baseDelay := 1 * time.Second
	delay := baseDelay * time.Duration(1<<(job.Attempts-1))
	
	nextRunAt := time.Now().Add(delay)
	job.NextRunAt = &nextRunAt
	
	log.Printf("[Worker] Job scheduled for retry: job_id=%s, attempt=%d/%d, next_run_at=%s, delay=%v", 
		job.TaskID, job.Attempts, job.MaxAttempts, nextRunAt.Format(time.RFC3339), delay)
	w.markJobRetry(ctx, job.TaskID, err.Error(), nextRunAt)

	// STEP 2.5: Update task attempt record on failure (retry path)
	// After marking job for retry, update the task_attempts record
	// This marks the current attempt as completed with failure
	// Call db.UpdateAttemptOnFailure(ctx, w.dbPool, job.TaskID, err.Error()) to update the attempt
	// Handle errors: log error but don't fail (attempt tracking is for audit/debugging)
	// The job is already marked for retry, so we continue even if attempt update fails
	// Example: if attemptErr := db.UpdateAttemptOnFailure(ctx, w.dbPool, job.TaskID, err.Error()); attemptErr != nil {
	//              log.Printf("Warning: Failed to update task attempt for job %s: %v", job.TaskID, attemptErr)
	//              // Continue anyway - job is marked for retry, attempt tracking is secondary
	//          }
	
	if attemptErr := db.UpdateAttemptOnFailure(ctx, w.dbPool, job.TaskID, err.Error()); attemptErr != nil {
		log.Printf("[Worker] Warning: Failed to update task attempt: job_id=%s, error: %v", job.TaskID, attemptErr)
		// Continue anyway - job is marked for retry, attempt tracking is secondary
	}

}

// executeHandler executes the job handler based on job type
func (w *Worker) executeHandler(ctx context.Context, job *db.Job) error {
	// STEP 1: Use a switch statement on job.Type to handle different job types
	// noop, http_capp, db_tx

	switch job.Type {
	case "noop":
		log.Printf("Executing noop handler for job %s", job.TaskID)
		return nil
	case "http_call":
		// STEP 1: Define a struct to parse the HTTP call payload
		// Define struct to parse the HTTP call payload
		log.Printf("[Worker] Executing http_call handler: job_id=%s", job.TaskID)

		type HTTPCallPayload struct {
			URL     string            `json:"url"`
			Method  string            `json:"method"`
			Headers map[string]string `json:"headers"`
			Body    string            `json:"body"`
		}
		
		// Parse job.PayloadJSON into the struct
		var payload HTTPCallPayload
		if err := json.Unmarshal(job.PayloadJSON, &payload); err != nil {
			log.Printf("[Worker] Failed to parse http_call payload: job_id=%s, error: %v", job.TaskID, err)
			return fmt.Errorf("failed to parse http_call payload: %w", err)
		}

		log.Printf("[Worker] Making HTTP request: job_id=%s, method=%s, url=%s", job.TaskID, payload.Method, payload.URL)

		// STEP 2: Create HTTP request
		// Use http.NewRequestWithContext(ctx, method, url, body)
		// Set headers from the parsed payload
		
		request, err := http.NewRequestWithContext(ctx, payload.Method, payload.URL, strings.NewReader(payload.Body))
		if err != nil {
			log.Printf("[Worker] Failed to create HTTP request: job_id=%s, error: %v", job.TaskID, err)
			return fmt.Errorf("failed to create HTTP request: %w", err)
		}
		
		// Set headers from the parsed payload
		for key, value := range payload.Headers {
			request.Header.Set(key, value)
		}

		// STEP 3: Execute the request
		// Create http.Client with timeout (e.g., 30 seconds)
		// Call client.Do(req) to execute the request
		
		client := http.Client{
			Timeout: 30 * time.Second,
		}

		response, err := client.Do(request)
		if err != nil {
			log.Printf("[Worker] HTTP request failed: job_id=%s, error: %v", job.TaskID, err)
			return fmt.Errorf("failed to execute HTTP request: %w", err)
		}
		defer response.Body.Close()

		log.Printf("[Worker] HTTP request completed: job_id=%s, status_code=%d", job.TaskID, response.StatusCode)

		// STEP 4: Check response
		// Check resp.StatusCode - if >= 400, return error
		// Otherwise return nil (success)
		// Don't forget to close resp.Body with defer resp.Body.Close()
		
		if response.StatusCode >= 400 {
			log.Printf("[Worker] HTTP request returned error status: job_id=%s, status_code=%d", job.TaskID, response.StatusCode)
			return fmt.Errorf("HTTP request failed with status code %d", response.StatusCode)
		}

		return nil
	case "db_tx":
		log.Printf("[Worker] Executing db_tx handler: job_id=%s", job.TaskID)
		
		type DBTxPayload struct {
			Query  string        `json:"query"`
			Params []interface{} `json:"params"`
		}

		var payload DBTxPayload
		if err := json.Unmarshal(job.PayloadJSON, &payload); err != nil {
			log.Printf("[Worker] Failed to parse db_tx payload: job_id=%s, error: %v", job.TaskID, err)
			return fmt.Errorf("failed to parse db_tx payload: %w", err)
		}

		if payload.Query == "" {
			log.Printf("[Worker] db_tx payload missing query field: job_id=%s", job.TaskID)
			return fmt.Errorf("db_tx payload missing required field: query")
		}

		log.Printf("[Worker] Executing database transaction: job_id=%s", job.TaskID)
		tx, err := w.dbPool.Begin(ctx)
		if err != nil {
			log.Printf("[Worker] Failed to begin transaction: job_id=%s, error: %v", job.TaskID, err)
			return fmt.Errorf("failed to begin database transaction: %w", err)
		}

		// Defer rollback - will be no-op if commit succeeds
		defer func() {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				// Ignore rollback errors after successful commit
				// Only log if it's an unexpected error
			}
		}()

		_, err = tx.Exec(ctx, payload.Query, payload.Params...)
		if err != nil {
			return fmt.Errorf("failed to execute database query: %w", err)
		}

		if err = tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit database transaction: %w", err)
		}

		return nil
	default:
		return fmt.Errorf("unknown job type: %s", job.Type)
	}
}

// retryPump periodically checks the database for jobs with status='retry' and next_run_at <= now()
// and moves them back to Redis priority queues for reprocessing
func (w *Worker) retryPump(ctx context.Context) {
	// STEP 1: Create a ticker that fires every 500ms
	// Use time.NewTicker(500 * time.Millisecond)
	// This will check for retry jobs 2 times per second
	// Example: ticker := time.NewTicker(500 * time.Millisecond)
	ticker := time.NewTicker(500 * time.Millisecond)

	// STEP 2: Defer ticker cleanup
	// Always defer ticker.Stop() to prevent resource leaks
	// This ensures the ticker is stopped when the function returns
	// Example: defer ticker.Stop()

	defer ticker.Stop()
	// STEP 3: Log that retry pump started
	// Use log.Println() to indicate the retry pump process has started
	// Example: log.Println("Retry pump started: monitoring for retry jobs")

	log.Println("Retry pump started: monitoring for retry jobs")

	// STEP 4: Create infinite loop with select statement
	// Use for { select { ... } } pattern (same as reaper function)
	// This allows us to handle both ticker events and shutdown signals

	// STEP 4.1: Handle shutdown signal
		// case <-ctx.Done():
		//   - Log that retry pump is stopping
		//   - Return from function to exit gracefully
		//   - Example: log.Println("Retry pump stopping")
		//   - Example: return

		// STEP 4.2: Handle ticker event
		// case <-ticker.C:
		//   - Call w.processRetryJobs(ctx) to process retry jobs
		//   - This function will query DB and requeue jobs
		//   - Example: w.processRetryJobs(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("[RetryPump] Retry pump stopping")
			return
		case <-ticker.C:
			w.processRetryJobs(ctx)
		}
	}
	
}

// processRetryJobs queries the database for jobs ready to be retried and requeues them
func (w *Worker) processRetryJobs(ctx context.Context) {
	// STEP 1: Build SQL query to find retry jobs that are ready
	log.Printf("[RetryPump] Checking for jobs ready to retry")
	// Query should:
	//   - Select: task_id, type, priority, payload, attempts, max_attempts
	//   - Filter: status = 'retry' AND next_run_at IS NOT NULL AND next_run_at <= now()
	//   - Order by: next_run_at ASC (process oldest first)
	//   - Limit: 100 (to avoid processing too many at once)
	// Example query:
	//   SELECT task_id, type, priority, payload, attempts, max_attempts
	//   FROM tasks
	//   WHERE status = 'retry' AND next_run_at IS NOT NULL AND next_run_at <= now()
	//   ORDER BY next_run_at ASC
	//   LIMIT 100

	query := `
		SELECT task_id, type, priority, payload, attempts, max_attempts
		FROM tasks
		WHERE status = 'retry' AND next_run_at IS NOT NULL AND next_run_at <= now()
		ORDER BY next_run_at ASC
		LIMIT 100
	`

	// STEP 2: Execute the query
	// Use w.dbPool.Query(ctx, query) to execute the SQL query
	// Handle errors: if query fails, log error and return early
	// Example: rows, err := w.dbPool.Query(ctx, query)
	//          if err != nil { log error and return }

	rows, err := w.dbPool.Query(ctx, query)
	if err != nil {
		log.Printf("[RetryPump] Error querying database for retry jobs: %v", err)
		return
	}

	// STEP 3: Defer rows.Close()
	// Always defer rows.Close() to ensure database connection is released
	// This prevents connection leaks
	// Example: defer rows.Close()
	defer rows.Close()
	
	

	// STEP 4: Check if no rows returned
	// If no jobs are ready to retry, just return (nothing to do)
	// STEP 5: Create variables to track processing
	// - processedCount := 0 (optional, for logging)
	// - Variables to scan into: taskID (uuid.UUID), jobType (string), priority (string),
	//   payloadJSON (json.RawMessage or []byte), attempts (int), maxAttempts (int)

	processedCount := 0
	var taskID uuid.UUID
	var jobType string
	var priority string
	var payloadJSON json.RawMessage
	var attempts int
	var maxAttempts int

	// STEP 6: Iterate through query results
	// Use for rows.Next() { ... } loop
	// This will iterate through each row returned by the query

	for rows.Next() {

		// STEP 6.1: Scan row into variables
		// Use rows.Scan() to read values from the current row
		// Scan into: &taskID, &jobType, &priority, &payloadJSON, &attempts, &maxAttempts
		// Handle scan errors: log error and continue to next row (don't stop processing)
		// Example: err := rows.Scan(&taskID, &jobType, &priority, &payloadJSON, &attempts, &maxAttempts)
		//          if err != nil { log error, continue }

		// STEP 6.2: Check if job is still retryable
		// Compare attempts with maxAttempts
		// If attempts >= maxAttempts:
		//   - Job has exceeded max attempts, mark as permanently failed
		//   - Call w.markJobFailed(ctx, taskID, "Max attempts exceeded")
		//   - Continue to next job (don't requeue)
		// Example: if attempts >= maxAttempts { mark as failed, continue }

		// STEP 6.3: Requeue the job
		// If attempts < maxAttempts, the job is still retryable
		// Call w.requeueRetryJob(ctx, taskID, jobType, priority, payloadJSON)
		// Handle errors: log error but continue processing other jobs
		// Don't return on error - we want to process all ready jobs
		// Example: if err := w.requeueRetryJob(...); err != nil {
		//              log.Printf("Retry pump: Failed to requeue job %s: %v", taskID, err)
		//              continue
		//          }

		// STEP 6.4: Optional - increment processedCount
		// Track how many jobs were successfully requeued for logging

		err := rows.Scan(&taskID, &jobType, &priority, &payloadJSON, &attempts, &maxAttempts)
		if err != nil {
			log.Printf("[RetryPump] Error scanning row: error: %v", err)
			continue
		}

		if attempts >= maxAttempts {
			log.Printf("[RetryPump] Job exceeded max attempts, marking as failed: job_id=%s, attempts=%d/%d", taskID, attempts, maxAttempts)
			w.markJobFailed(ctx, taskID, "Max attempts exceeded")
			continue
		}

		log.Printf("[RetryPump] Requeuing job: job_id=%s, type=%s, priority=%s, attempts=%d/%d", taskID, jobType, priority, attempts, maxAttempts)
		if err := w.requeueRetryJob(ctx, taskID, jobType, priority, payloadJSON); err != nil {
			log.Printf("[RetryPump] Failed to requeue job: job_id=%s, error: %v", taskID, err)
			continue
		}

		processedCount++
	}
		
	// STEP 7: Check for iteration errors
	// After the loop, check rows.Err() for any errors during iteration
	// Handle errors: log and return (or just log, depending on severity)
	// Example: if err := rows.Err(); err != nil {
	//              log.Printf("Retry pump: Error iterating rows: %v", err)
	//              return
	//          }

	if err := rows.Err(); err != nil {
		log.Printf("[RetryPump] Error iterating rows: error: %v", err)
		return
	}

	// STEP 8: Optional - Log summary
	// If you tracked processedCount, log how many jobs were requeued
	// Example: if processedCount > 0 {
	//              log.Printf("Retry pump: Requeued %d job(s)", processedCount)
	//          }
	if processedCount > 0 {
		log.Printf("[RetryPump] Requeued %d job(s) for retry", processedCount)
	}
}

// requeueRetryJob atomically updates job status from 'retry' to 'queued' and pushes it back to Redis queue
func (w *Worker) requeueRetryJob(ctx context.Context, jobID uuid.UUID, jobType, priority string, payloadJSON json.RawMessage) error {
	log.Printf("[RetryPump] Requeuing job: job_id=%s, type=%s, priority=%s", jobID, jobType, priority)
	// STEP 1: Update database status atomically from 'retry' to 'queued'
	// Use UPDATE with WHERE clause to ensure atomicity and prevent race conditions
	// Only update if status is still 'retry' (prevents double-processing if multiple workers run)
	// Query should:
	//   - UPDATE tasks SET status = 'queued', updated_at = now()
	//   - WHERE task_id = $1 AND status = 'retry'
	//   - RETURNING task_id (to verify update succeeded)
	// Example query:
	//   UPDATE tasks 
	//   SET status = 'queued', updated_at = now()
	//   WHERE task_id = $1 AND status = 'retry'
	//   RETURNING task_id

	query := `
		UPDATE tasks
		SET status = 'queued', updated_at = now()
		WHERE task_id = $1 AND status = 'retry'
		RETURNING task_id
	`

	// STEP 2: Execute the UPDATE query
	// Use w.dbPool.QueryRow(ctx, query, jobID) to execute
	// Scan the returned task_id to verify the update succeeded
	// Handle errors:
	//   - If pgx.ErrNoRows: job was already processed by another worker (not an error, return nil)
	//   - Other errors: return wrapped error
	// Example: var updatedTaskID uuid.UUID
	//          err := w.dbPool.QueryRow(ctx, query, jobID).Scan(&updatedTaskID)
	//          if err == pgx.ErrNoRows { return nil } // Already processed
	//          if err != nil { return fmt.Errorf("failed to update job: %w", err) }

	var updatedTaskID uuid.UUID
	err := w.dbPool.QueryRow(ctx, query, jobID).Scan(&updatedTaskID)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return fmt.Errorf("failed to update job: %w", err)
	}
	
	// STEP 3: Push job to Redis queue using the helper function
	// Use redis.PushJob() which handles:
	//   - Getting queue name from priority
	//   - Validating queue name
	//   - Creating JobPayload struct
	//   - Marshaling to JSON
	//   - Pushing to Redis with LPUSH
	// This avoids duplicating code that already exists in redis.PushJob()
	// Note: Even if Redis push fails, the DB is already updated to 'queued'
	// This is acceptable - the job will be in DB and can be picked up later
	// Example: if err := redis.PushJob(ctx, w.redisClient, jobID, jobType, priority); err != nil {
	//              return fmt.Errorf("failed to push job to redis: %w", err)
	//          }

	if err := redis.PushJob(ctx, w.redisClient, jobID, jobType, priority); err != nil {
		log.Printf("[RetryPump] Failed to push job to Redis: job_id=%s, error: %v", jobID, err)
		return fmt.Errorf("failed to push job to redis: %w", err)
	}

	log.Printf("[RetryPump] Job requeued successfully: job_id=%s, queue=%s", jobID, redis.GetQueueName(priority))

	// STEP 7: Return nil on success
	// If all steps succeeded, return nil
	// Example: return nil
	return nil
}