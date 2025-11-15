package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/williamntlam/go-grpc-task-scheduler/internal/db"
	schedulerv1 "github.com/williamntlam/go-grpc-task-scheduler/proto/scheduler/v1"
	// TODO: Add your imports for Redis, etc.
	// "github.com/redis/go-redis/v9"
)

// Server implements the SchedulerServiceServer interface
type Server struct {
	// Embed UnimplementedSchedulerServiceServer for forward compatibility
	// This ensures your server will work even if new methods are added to the interface
	schedulerv1.UnimplementedSchedulerServiceServer

	// Database connection pool
	db *pgxpool.Pool
	// TODO: Add other dependencies
	// redis *redis.Client
	// logger *zap.Logger
	// metrics *prometheus.Registry
}

// NewServer creates a new Server instance
func NewServer(db *pgxpool.Pool /* redis *redis.Client */) *Server {
	return &Server{
		db: db,
		// TODO: Initialize other dependencies
		// redis: redis,
	}
}

// validateSubmitJobRequest validates a SubmitJobRequest and returns an error if invalid
func validateSubmitJobRequest(req *schedulerv1.SubmitJobRequest) error {
	if req == nil {
		return status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	job := req.Job
	if job == nil {
		return status.Error(codes.InvalidArgument, "job cannot be nil")
	}

	// Validate job type (required)
	if strings.TrimSpace(job.Type) == "" {
		return status.Error(codes.InvalidArgument, "job type is required")
	}

	// Validate priority (must be a valid enum value, not UNSPECIFIED)
	if job.Priority == schedulerv1.Priority_PRIORITY_UNSPECIFIED {
		return status.Error(codes.InvalidArgument, "job priority must be specified (CRITICAL, HIGH, DEFAULT, or LOW)")
	}

	// Validate max_attempts (if provided, must be positive and reasonable)
	if job.MaxAttempts < 0 {
		return status.Error(codes.InvalidArgument, "max_attempts cannot be negative")
	}
	if job.MaxAttempts > 100 {
		return status.Error(codes.InvalidArgument, "max_attempts cannot exceed 100")
	}
	// Default to 3 if not specified (will be handled in business logic)

	// Validate payload_json is valid JSON (if provided)
	if job.PayloadJson != "" {
		var testJSON interface{}
		if err := json.Unmarshal([]byte(job.PayloadJson), &testJSON); err != nil {
			return status.Error(codes.InvalidArgument, fmt.Sprintf("payload_json is not valid JSON: %v", err))
		}

		// Validate payload size (prevent abuse - e.g., max 1MB)
		const maxPayloadSize = 1024 * 1024 // 1MB
		if len(job.PayloadJson) > maxPayloadSize {
			return status.Error(codes.InvalidArgument, fmt.Sprintf("payload_json exceeds maximum size of %d bytes", maxPayloadSize))
		}
	}

	// Note: idempotency_key is no longer used
	// Idempotency is handled by using job_id directly - if job_id exists, return existing job

	return nil
}

// SubmitJob handles job submission requests
func (s *Server) SubmitJob(ctx context.Context, req *schedulerv1.SubmitJobRequest) (*schedulerv1.SubmitJobResponse, error) {
	// 1. Validate request
	if err := validateSubmitJobRequest(req); err != nil {
		return nil, err
	}

	// Extract job data from request
	job := req.Job // This is the Job struct from the request

	// Extract job data from request (will be used when implementing database insert)
	jobType := job.Type             // string (required - e.g., "noop", "http_call")
	priority := job.Priority        // Priority enum (CRITICAL, HIGH, DEFAULT, LOW)
	payloadJSON := job.PayloadJson  // string (optional JSON)
	maxAttempts := job.MaxAttempts  // int32 (optional, defaults to 0 if not set)

	// Determine job_id:
	// 1. If job_id provided by client, use it (enables idempotency - same job_id = same job)
	// 2. Otherwise, generate new UUID
	// 
	// Idempotency: If job_id already exists in tasks table, return existing job
	var jobID uuid.UUID
	if job.JobId != "" {
		// Client provided job_id, parse it
		parsedID, err := uuid.Parse(job.JobId)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "job_id must be a valid UUID")
		}
		jobID = parsedID
	} else {
		// Generate new UUID for the job
		jobID = uuid.New()
	}

	// Check if the jobId is in the database already (idempotency check)
	exists, err := db.JobExists(ctx, s.db, jobID)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to check if job exists: %v", err))
	}

	if exists {
		// Job already exists, return existing job_id (idempotent)
		return &schedulerv1.SubmitJobResponse{
			JobId: jobID.String(),
		}, nil
	}

	// Set default max_attempts if not provided
	if maxAttempts == 0 {
		maxAttempts = 3
	}

	// Convert priority enum to string
	priorityStr := priority.String()
	// Remove "PRIORITY_" prefix if present
	priorityStr = strings.TrimPrefix(priorityStr, "PRIORITY_")
	priorityStr = strings.ToLower(priorityStr)

	// Create job in database
	dbJob := db.Job{
		TaskID:      jobID,
		Type:        jobType,
		Priority:    priorityStr,
		PayloadJSON: json.RawMessage(payloadJSON),
		Status:      "queued",
		Attempts:    0,
		MaxAttempts: int(maxAttempts),
		NextRunAt:   nil,
	}

	createdID, alreadyExists, err := db.CreateJob(ctx, s.db, dbJob)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to create job: %v", err))
	}

	if alreadyExists {
		// Job was created between our check and insert (race condition handled)
		return &schedulerv1.SubmitJobResponse{
			JobId: createdID.String(),
		}, nil
	}

	// TODO: Push to Redis queue (LPUSH q:{priority})

	// Return job_id
	return &schedulerv1.SubmitJobResponse{
		JobId: jobID.String(),
	}, nil
}

// GetJob retrieves the status of a job
func (s *Server) GetJob(ctx context.Context, req *schedulerv1.GetJobRequest) (*schedulerv1.JobStatus, error) {
	// TODO: Implement
	// 1. Query CockroachDB for task by job_id
	// 2. Convert to JobStatus message
	// 3. Return

	return nil, nil
}

// WatchJob streams job status updates
func (s *Server) WatchJob(req *schedulerv1.WatchJobRequest, stream schedulerv1.SchedulerService_WatchJobServer) error {
	// TODO: Implement
	// 1. Poll CockroachDB for job status changes
	// 2. Send JobEvent messages via stream.Send()
	// 3. Continue until job completes or context cancelled

	return nil
}

// CancelJob cancels a running job
func (s *Server) CancelJob(ctx context.Context, req *schedulerv1.CancelJobRequest) (*schedulerv1.CancelJobResponse, error) {
	// TODO: Implement
	// 1. Update task status in CockroachDB to 'cancelled'
	// 2. Remove from Redis queue/leases if still queued/running
	// 3. Return success

	return &schedulerv1.CancelJobResponse{
		Cancelled: false,
	}, nil
}

// ListJobs lists jobs with optional filters
func (s *Server) ListJobs(ctx context.Context, req *schedulerv1.ListJobsRequest) (*schedulerv1.ListJobsResponse, error) {
	// TODO: Implement
	// 1. Build query with filters (state, priority, type)
	// 2. Query CockroachDB with pagination
	// 3. Convert results to JobStatus messages
	// 4. Return with next_page_token

	return &schedulerv1.ListJobsResponse{
		Jobs: []*schedulerv1.JobStatus{},
	}, nil
}

