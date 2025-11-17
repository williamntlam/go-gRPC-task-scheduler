package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Job represents a job in the database (stored in tasks table)
// Note: Database table is "tasks" but we use "Job" terminology in code
type Job struct {
	TaskID      uuid.UUID       `json:"task_id"`
	Type        string          `json:"type"`
	Priority    string          `json:"priority"`
	PayloadJSON json.RawMessage `json:"payload"`
	Status      string          `json:"status"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"max_attempts"`
	NextRunAt   *time.Time      `json:"next_run_at"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// GetJobByID retrieves a job by its ID
func GetJobByID(ctx context.Context, pool *pgxpool.Pool, jobID uuid.UUID) (*Job, error) {
	query := `
		SELECT task_id, type, priority, payload, status, attempts, max_attempts, 
		       next_run_at, created_at, updated_at
		FROM tasks
		WHERE task_id = $1
	`

	var job Job
	var nextRunAt *time.Time
	err := pool.QueryRow(ctx, query, jobID).Scan(
		&job.TaskID,
		&job.Type,
		&job.Priority,
		&job.PayloadJSON,
		&job.Status,
		&job.Attempts,
		&job.MaxAttempts,
		&nextRunAt,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Job not found
		}
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	job.NextRunAt = nextRunAt
	return &job, nil
}

// CreateJob creates a new job in the database
// Returns the job_id and whether it already existed (for idempotency)
func CreateJob(ctx context.Context, pool *pgxpool.Pool, job Job) (uuid.UUID, bool, error) {
	query := `
		INSERT INTO tasks (task_id, type, priority, payload, status, attempts, max_attempts, next_run_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (task_id) DO NOTHING
		RETURNING task_id
	`

	var taskID uuid.UUID
	err := pool.QueryRow(
		ctx,
		query,
		job.TaskID,
		job.Type,
		job.Priority,
		job.PayloadJSON,
		job.Status,
		job.Attempts,
		job.MaxAttempts,
		job.NextRunAt,
	).Scan(&taskID)

	if err != nil {
		if err == pgx.ErrNoRows {
			// Job already exists (idempotency)
			return job.TaskID, true, nil
		}
		return uuid.Nil, false, fmt.Errorf("failed to create job: %w", err)
	}

	// New job created
	return taskID, false, nil
}

// JobExists checks if a job with the given ID exists
func JobExists(ctx context.Context, pool *pgxpool.Pool, jobID uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM tasks WHERE task_id = $1)`

	var exists bool
	err := pool.QueryRow(ctx, query, jobID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if job exists: %w", err)
	}

	return exists, nil
}

