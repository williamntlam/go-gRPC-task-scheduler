package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	schedulerv1 "github.com/williamntlam/go-grpc-task-scheduler/proto/scheduler/v1"
	// TODO: Add your imports for DB, Redis, etc.
	// "github.com/jackc/pgx/v5/pgxpool"
	// "github.com/redis/go-redis/v9"
)

// Server implements the SchedulerServiceServer interface
type Server struct {
	// Embed UnimplementedSchedulerServiceServer for forward compatibility
	// This ensures your server will work even if new methods are added to the interface
	schedulerv1.UnimplementedSchedulerServiceServer

	// Add your dependencies here
	// db    *pgxpool.Pool
	// redis *redis.Client
	// logger *zap.Logger
	// metrics *prometheus.Registry
}

// NewServer creates a new Server instance
func NewServer(/* db *pgxpool.Pool, redis *redis.Client */) *Server {
	return &Server{
		// Initialize your dependencies
		// db:    db,
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

	// Validate idempotency_key format (if provided)
	// Idempotency keys should be UUIDs for best practices
	if job.IdempotencyKey != "" {
		key := strings.TrimSpace(job.IdempotencyKey)
		if len(key) == 0 {
			return status.Error(codes.InvalidArgument, "idempotency_key cannot be empty or whitespace")
		}
		// Validate UUID format
		if _, err := uuid.Parse(key); err != nil {
			return status.Error(codes.InvalidArgument, "idempotency_key must be a valid UUID")
		}
	}

	return nil
}

// SubmitJob handles job submission requests
func (s *Server) SubmitJob(ctx context.Context, req *schedulerv1.SubmitJobRequest) (*schedulerv1.SubmitJobResponse, error) {
	// 1. Validate request
	if err := validateSubmitJobRequest(req); err != nil {
		return nil, err
	}

	// TODO: Implement
	// 2. Check idempotency if key provided
	// 3. Insert task into CockroachDB
	// 4. Push to Redis queue (LPUSH q:{priority})
	// 5. Return job_id

	return &schedulerv1.SubmitJobResponse{
		JobId: "placeholder-job-id",
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

