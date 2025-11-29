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
		
		// Step 1: Pop job from priority queues (blocks until job available or timeout)
		result, err := w.redisClient.BRPop(ctx, 1*time.Second, "q:critical", "q:high", "q:default", "q:low").Result()
		
		// Handle timeout (no jobs available)
		if err == redisc.Nil {
			// STEP 6: Check for shutdown signal during timeout
			// Add a select statement here to check ctx.Done() before continuing
			// This allows the worker to exit quickly even when no jobs are available
			if w.checkShutdown(ctx) {
				return
			}

			continue // Timeout, loop back and try again
		}
		
		// Handle other errors
		if err != nil {
			log.Printf("Error popping from queue: %v", err)
			// STEP 7: Check for shutdown signal before retrying
			// Replace time.Sleep with a select that checks ctx.Done()
			// Example:
			// select {
			// case <-ctx.Done():
			//     log.Println("Worker received shutdown signal, stopping job processing")
			//     return
			// case <-time.After(1 * time.Second):
			//     continue
			// }
			
			if w.checkShutdown(ctx) {
				return
			}

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

		// STEP 8: Add shutdown check before processing a job
		// After parsing the job ID but before claiming it, check if shutdown was signaled
		// If so, log and return (don't process this job)

		if w.checkShutdown(ctx) {
			return
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
		go func(job *db.Job) {
			defer w.wg.Done()
			handlerErr := w.executeHandler(ctx, job)

			// Step 7: Update job status based on result
			if handlerErr != nil {
				log.Printf("Job %s failed: %v", job.TaskID, handlerErr)
				w.handleJobFailure(ctx, job, handlerErr)
			} else {
				log.Printf("Job %s completed successfully", job.TaskID)
				w.markJobSucceeded(ctx, job.TaskID)
			}
		}(job)
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

// markJobRetry updates job status to 'retry' and sets next_run_at for retry scheduling
func (w *Worker) markJobRetry(ctx context.Context, jobID uuid.UUID, errorMsg string, nextRunAt time.Time) error {
	query := `
		UPDATE tasks 
		SET status = 'retry', next_run_at = $2, updated_at = now()
		WHERE task_id = $1
	`
	
	_, err := w.dbPool.Exec(ctx, query, jobID, nextRunAt)
	if err != nil {
		return fmt.Errorf("failed to mark job as retry: %w", err)
	}
	
	return nil
}

// handleJobFailure handles job failure, checking if retry is needed
func (w *Worker) handleJobFailure(ctx context.Context, job *db.Job, err error) {
	// STEP 1: Check if job should be retried
	// Compare job.Attempts with job.MaxAttempts
	// If job.Attempts < job.MaxAttempts, we should retry
	// Otherwise, mark as permanently failed
	if job.Attempts >= job.MaxAttempts {
		// Max attempts reached, mark as permanently failed
		log.Printf("Job %s failed after %d attempts, marking as failed", job.TaskID, job.Attempts)
		w.markJobFailed(ctx, job.TaskID, err.Error())
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
	
	log.Printf("Job %s failed (attempt %d/%d), will retry later at %s", job.TaskID, job.Attempts, job.MaxAttempts, nextRunAt.Format(time.RFC3339))
	w.markJobRetry(ctx, job.TaskID, err.Error(), nextRunAt)

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
		log.Printf("Executing http_call handler for job %s", job.TaskID)

		type HTTPCallPayload struct {
			URL     string            `json:"url"`
			Method  string            `json:"method"`
			Headers map[string]string `json:"headers"`
			Body    string            `json:"body"`
		}
		
		// Parse job.PayloadJSON into the struct
		var payload HTTPCallPayload
		if err := json.Unmarshal(job.PayloadJSON, &payload); err != nil {
			return fmt.Errorf("failed to parse http_call payload: %w", err)
		}

		// STEP 2: Create HTTP request
		// Use http.NewRequestWithContext(ctx, method, url, body)
		// Set headers from the parsed payload
		
		request, err := http.NewRequestWithContext(ctx, payload.Method, payload.URL, strings.NewReader(payload.Body))
		if err != nil {
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
			return fmt.Errorf("failed to execute HTTP request: %w", err)
		}
		defer response.Body.Close()

		// STEP 4: Check response
		// Check resp.StatusCode - if >= 400, return error
		// Otherwise return nil (success)
		// Don't forget to close resp.Body with defer resp.Body.Close()
		
		if response.StatusCode >= 400 {
			return fmt.Errorf("HTTP request failed with status code %d", response.StatusCode)
		}

		return nil
	case "db_tx":
		// STEP 1: Define a struct to parse the database transaction payload
		// Example: type DBTxPayload struct { Query string, Params []interface{} }
		// Parse job.PayloadJSON into this struct using json.Unmarshal
		
		// STEP 2: Begin database transaction
		// Use w.dbPool.Begin(ctx) to start a transaction
		// This returns a pgx.Tx object
		
		// STEP 3: Execute query within transaction
		// Use tx.Exec(ctx, query, params...) or tx.QueryRow(ctx, query, params...)
		// Based on the parsed payload
		
		// STEP 4: Commit or rollback
		// If successful: call tx.Commit(ctx)
		// If error: call tx.Rollback(ctx) and return the error
		// Return nil if commit succeeds
		
		log.Printf("Executing db_tx handler for job %s", job.TaskID)
		return fmt.Errorf("db_tx handler not yet implemented")
	default:
		return fmt.Errorf("unknown job type: %s", job.Type)
	}
	// STEP 2: Implement "noop" handler case (no operation - for testing)
	//   a. Add case "noop": in your switch statement
	//   b. Log: log.Printf("Executing noop handler for job %s", job.TaskID)
	//   c. Return nil (success) - this handler does nothing, just for testing
	
	// STEP 3: Handle unknown/default case
	//   a. Add default: case in switch statement
	//   b. Return error: return fmt.Errorf("unknown job type: %s", job.Type)
	//   This catches any job types that don't have a handler implemented
	
	// STEP 4: (Future) To add more handlers, add new cases to the switch:
	//   Example for "http_call":
	//   case "http_call":
	//       // a. Parse job.PayloadJSON to extract URL, method, headers, body
	//       //    Use json.Unmarshal to parse into a struct
	//       // b. Create HTTP request using http.NewRequest or http.Client
	//       // c. Execute the request
	//       // d. Check response status code
	//       // e. Return error if status code indicates failure, nil if success
	//   
	//   Example for "db_tx":
	//   case "db_tx":
	//       // a. Parse job.PayloadJSON to extract SQL query and parameters
	//       // b. Begin transaction: w.dbPool.Begin(ctx)
	//       // c. Execute query within transaction
	//       // d. Commit transaction if successful, rollback on error
	//       // e. Return error if failed, nil if succeeded
	//   
	//   Each handler should:
	//     - Parse job.PayloadJSON using json.Unmarshal
	//     - Execute the handler-specific logic
	//     - Handle errors appropriately
	//     - Return error if failed, nil if succeeded
	
	// TODO: Implement the switch statement and handlers as described above
	// For now, return error for unknown type (you'll replace this with your implementation)
	
}