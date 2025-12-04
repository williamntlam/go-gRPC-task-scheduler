package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InsertAttempt inserts a new task attempt record when a job starts
// This should be called when a job is claimed and starts executing
// Parameters:
//   - ctx: context.Context for cancellation/timeout
//   - pool: database connection pool
//   - taskID: UUID of the task/job
//
// Returns error if insertion fails
func InsertAttempt(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID) error {
	// STEP 1: Build SQL INSERT query
	// Query should:
	//   - INSERT INTO task_attempts (task_id, started_at)
	//   - VALUES ($1, now())
	// Note: started_at is set to now() (current timestamp)
	//       finished_at, ok, and error are NULL initially (will be updated later)
	// Example query:
	//   INSERT INTO task_attempts (task_id, started_at)
	//   VALUES ($1, now())

	query := `
		INSERT INTO task_attempts (task_id, started_at)
		VALUES ($1, now())
	`

	// STEP 2: Execute the INSERT query
	// Use pool.Exec(ctx, query, taskID) to execute
	// Handle errors: return wrapped error
	// Example: _, err := pool.Exec(ctx, query, taskID)
	//          if err != nil {
	//              return fmt.Errorf("failed to insert task attempt: %w", err)
	//          }

	_, err := pool.Exec(ctx, query, taskID)
	if err != nil {
		return fmt.Errorf("failed to insert task attempt: %w", err)
	}

	// STEP 3: Return nil on success
	// Example: return nil
	return nil

}

// UpdateAttemptOnSuccess updates a task attempt record when a job succeeds
// This should be called when a job completes successfully
// Parameters:
	//   - ctx: context.Context for cancellation/timeout
	//   - pool: database connection pool
	//   - taskID: UUID of the task/job
	//
	// Returns error if update fails
	// Note: Updates the most recent attempt (where finished_at IS NULL)
func UpdateAttemptOnSuccess(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID) error {
	// STEP 1: Build SQL UPDATE query
	// Query should:
	//   - UPDATE task_attempts
	//   - SET finished_at = now(), ok = true
	//   - WHERE task_id = $1 AND finished_at IS NULL
	// Note: WHERE finished_at IS NULL ensures we update the current/latest attempt
	//       (the one that was just started and hasn't finished yet)
	// Example query:
	//   UPDATE task_attempts
	//   SET finished_at = now(), ok = true
	//   WHERE task_id = $1 AND finished_at IS NULL

	query := `
		UPDATE task_attempts
		SET finished_at = now(), ok = true
		WHERE task_id = $1 AND finished_at IS NULL
	`

	// STEP 2: Execute the UPDATE query
	// Use pool.Exec(ctx, query, taskID) to execute
	// Handle errors: return wrapped error
	// Example: _, err := pool.Exec(ctx, query, taskID)
	//          if err != nil {
	//              return fmt.Errorf("failed to update task attempt on success: %w", err)
	//          }

	_, err := pool.Exec(ctx, query, taskID)
	if err != nil {
		return fmt.Errorf("failed to update task attempt on success: %w", err)
	}

	// STEP 3: Return nil on success
	// Example: return nil
	
	return nil
}

// UpdateAttemptOnFailure updates a task attempt record when a job fails
// This should be called when a job fails (whether it will retry or go to DLQ)
// Parameters:
	//   - ctx: context.Context for cancellation/timeout
	//   - pool: database connection pool
	//   - taskID: UUID of the task/job
	//   - errorMsg: Error message describing why the job failed
	//
	// Returns error if update fails
	// Note: Updates the most recent attempt (where finished_at IS NULL)
func UpdateAttemptOnFailure(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID, errorMsg string) error {
	// STEP 1: Build SQL UPDATE query
	// Query should:
	//   - UPDATE task_attempts
	//   - SET finished_at = now(), ok = false, error = $2
	//   - WHERE task_id = $1 AND finished_at IS NULL
	// Note: WHERE finished_at IS NULL ensures we update the current/latest attempt
	//       error = $2 stores the error message for debugging
	// Example query:
	//   UPDATE task_attempts
	//   SET finished_at = now(), ok = false, error = $2
	//   WHERE task_id = $1 AND finished_at IS NULL

	// STEP 2: Execute the UPDATE query
	// Use pool.Exec(ctx, query, taskID, errorMsg) to execute
	// Note: errorMsg is the second parameter ($2)
	// Handle errors: return wrapped error
	// Example: _, err := pool.Exec(ctx, query, taskID, errorMsg)
	//          if err != nil {
	//              return fmt.Errorf("failed to update task attempt on failure: %w", err)
	//          }

	// STEP 3: Return nil on success
	// Example: return nil
	
	// TODO: Implement the function body above
	return fmt.Errorf("UpdateAttemptOnFailure not yet implemented")
}

