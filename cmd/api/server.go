package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/williamntlam/go-grpc-task-scheduler/internal/db"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/redis"
	schedulerv1 "github.com/williamntlam/go-grpc-task-scheduler/proto/scheduler/v1"

	redisc "github.com/redis/go-redis/v9"
)

// Server implements the SchedulerServiceServer interface
type Server struct {
	// Embed UnimplementedSchedulerServiceServer for forward compatibility
	// This ensures your server will work even if new methods are added to the interface
	schedulerv1.UnimplementedSchedulerServiceServer

	// Database connection pool
	db *pgxpool.Pool
	redis *redisc.Client
	// TODO: Add other dependencies
	// redis *redis.Client
	// logger *zap.Logger
	// metrics *prometheus.Registry
}

// NewServer creates a new Server instance
func NewServer(db *pgxpool.Pool, redisClient *redisc.Client) *Server {
	return &Server{
		db:    db,
		redis: redisClient,
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

// statusToJobState converts a database status string to a JobState enum
func statusToJobState(status string) schedulerv1.JobState {
	switch status {
	case "queued":
		return schedulerv1.JobState_JOB_STATE_QUEUED
	case "running":
		return schedulerv1.JobState_JOB_STATE_RUNNING
	case "succeeded":
		return schedulerv1.JobState_JOB_STATE_SUCCEEDED
	case "failed":
		return schedulerv1.JobState_JOB_STATE_FAILED
	case "deadletter":
		return schedulerv1.JobState_JOB_STATE_DEADLETTER
	case "cancelled":
		// Handle cancelled if needed
		return schedulerv1.JobState_JOB_STATE_UNSPECIFIED
	default:
		return schedulerv1.JobState_JOB_STATE_UNSPECIFIED
	}
}

func jobStateToString(status schedulerv1.JobState) string {
	switch status {
	case schedulerv1.JobState_JOB_STATE_QUEUED:
		return "queued"
	case schedulerv1.JobState_JOB_STATE_RUNNING:
		return "running"
	case schedulerv1.JobState_JOB_STATE_SUCCEEDED:
		return "succeeded"
	case schedulerv1.JobState_JOB_STATE_FAILED:
		return "failed"
	case schedulerv1.JobState_JOB_STATE_DEADLETTER:
		return "deadletter"
	case schedulerv1.JobState_JOB_STATE_UNSPECIFIED:
		// Return empty string to indicate "no filter"
		return ""
	default:
		// Unknown state - return empty to skip filtering
		return ""
	}
}

func priorityToString(priority schedulerv1.Priority) string {
	switch(priority) {
	case schedulerv1.Priority_PRIORITY_CRITICAL:
		return "critical"
	case schedulerv1.Priority_PRIORITY_LOW:
		return "low"
	case schedulerv1.Priority_PRIORITY_HIGH:
		return "high"
	case schedulerv1.Priority_PRIORITY_DEFAULT:
		return "default"
	case schedulerv1.Priority_PRIORITY_UNSPECIFIED:
		return ""
	default:
		return ""
	}
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

	// Push to Redis queue (LPUSH q:{priority})
	err = redis.PushJob(ctx, s.redis, jobID, jobType, priorityStr)
	if err != nil {
		// Log error but don't fail the request (job is already in DB)
		// This is a design decision: DB is source of truth, Redis is for processing
		log.Printf("Warning: failed to push job %s to Redis queue: %v", jobID.String(), err)
		// Optionally, you could return an error here if you want queue failures to fail the request
	}

	// Return job_id
	return &schedulerv1.SubmitJobResponse{
		JobId: jobID.String(),
	}, nil
}

// GetJob retrieves the status of a job
func (s *Server) GetJob(ctx context.Context, req *schedulerv1.GetJobRequest) (*schedulerv1.JobStatus, error) {
	// 1. Query CockroachDB for task by job_id
	// 2. Convert to JobStatus message
	// 3. Return

	jobID, err := uuid.Parse(req.JobId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "job_id must be a valid UUID")
	}

	job, err := db.GetJobByID(ctx, s.db, jobID)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get job: %v", err))
	}
	if job == nil {
		return nil, status.Error(codes.NotFound, "job not found")
	} 

	jobStatus := schedulerv1.JobStatus{
		JobId:     job.TaskID.String(),
		State:     statusToJobState(job.Status),
		Attempts:  int32(job.Attempts),
		LastError: "",
		CreatedAt: timestamppb.New(job.CreatedAt),
		UpdatedAt: timestamppb.New(job.UpdatedAt),
		StartedAt: nil,
		FinishedAt: nil,
	}

	return &jobStatus, nil
}

// WatchJob streams job status updates
func (s *Server) WatchJob(req *schedulerv1.WatchJobRequest, stream schedulerv1.SchedulerService_WatchJobServer) error {
	// Step 1: Parse and validate job_id from request
	// - Extract req.JobId (string)
	// - Parse to uuid.UUID using uuid.Parse()
	// - Handle parsing errors (return gRPC error if invalid)
	// - Same pattern as GetJob

	jobID, err := uuid.Parse(req.JobId)
	if err != nil {
		return status.Error(codes.InvalidArgument, "job_id must be a valid UUID")
	}

	// Step 2: Set up polling infrastructure
	// - Create ticker: time.NewTicker(1 * time.Second) or 2 seconds
	// - Always defer ticker.Stop() to clean up resources
	// - Track last known status: lastStatus := ""
	// - Track if first poll: firstPoll := true

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastStatus := ""
	firstPoll := true

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-ticker.C:
			// Time to poll the database and check for status changes
			
			// 1. Poll database using db.GetJobByID()
			//    - Use stream.Context() as the context
			//    - Handle errors (log and continue, or return)
			//    - Handle job not found (send error event and return)

			job, err := db.GetJobByID(stream.Context(), s.db, jobID)

			if err != nil {
				// Database error occurred
				log.Printf("Error polling job %s: %v", jobID.String(), err)
				return fmt.Errorf("failed to poll job: %w", err)
			}

			// Handle job not found
			if job == nil {
				// Job doesn't exist - send error event and return
				errorEvent := &schedulerv1.JobEvent{
					JobId:     jobID.String(),
					State:     schedulerv1.JobState_JOB_STATE_UNSPECIFIED,
					Message:   "Job not found",
					Timestamp: timestamppb.Now(),
					Attempts:  0,
				}
				if err := stream.Send(errorEvent); err != nil {
					return err
				}
				return status.Error(codes.NotFound, "job not found")
			}
						
			// 2. Check if status changed
			//    - Compare current job.Status with lastStatus
			//    - If different (or first poll), proceed to send event
			
			if job.Status != lastStatus || firstPoll {

				// 3. Convert to JobEvent
				//    - Create schedulerv1.JobEvent message
				//    - Set all fields (JobId, State, Message, Timestamp, Attempts)
				
				// 4. Send the event
				//    - Use stream.Send(&jobEvent)
				//    - Handle send errors (return error if stream broken)

				jobEvent := &schedulerv1.JobEvent{
					JobId: job.TaskID.String(),
					State:     statusToJobState(job.Status),
					Message:   "Job status changed to " + job.Status,  // Human-readable message
					Timestamp: timestamppb.Now(),  // Current time
					Attempts:  int32(job.Attempts),
				}

				if err := stream.Send(jobEvent); err != nil {
					log.Printf("Error streaming job %s: %v", jobID.String(), err)
					return fmt.Errorf("failed to stream job: %w", err)
				}

				// 5. Update lastStatus
				//    - lastStatus = job.Status

				lastStatus = job.Status
				firstPoll = false

			}
			
			// 6. Check if job is complete (terminal states)
			//    - If status is "succeeded", "failed", or "deadletter":
			//      * return nil (exit loop, job is done)
			//    - Otherwise, continue polling
			if job.Status == "succeeded" || job.Status == "failed" || job.Status == "deadletter" {
				// Job is complete, exit the polling loop
				return nil
			}
			// Otherwise, continue polling (loop continues)
		}

	}

}

// CancelJob cancels a running job
func (s *Server) CancelJob(ctx context.Context, req *schedulerv1.CancelJobRequest) (*schedulerv1.CancelJobResponse, error) {

	jobId, err := uuid.Parse(req.JobId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "job_id must be a valid UUID")
	}

	job, err := db.GetJobByID(ctx, s.db, jobId)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get job: %v", err))
	}
	if job == nil {
		return nil, status.Error(codes.NotFound, "job not found")
	}

	if job.Status == "succeeded" || job.Status == "failed" || job.Status == "deadletter" || job.Status == "cancelled" {
		return &schedulerv1.CancelJobResponse{
			Cancelled: false,
		}, nil
	}

	cancelled, err := db.CancelJob(ctx, s.db, jobId)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to cancel job: %v", err))
	}

	// If job was queued, remove it from Redis queue
	if cancelled && job.Status == "queued" {
		// Create the same JSON payload that was pushed to Redis
		payload := redis.JobPayload{
			TaskID:   jobId.String(),
			Type:     job.Type,
			Priority: job.Priority,
		}
		payloadJSON, err := json.Marshal(payload)
		if err == nil {
			// Get queue name based on priority
			queueName := redis.GetQueueName(job.Priority)
			if queueName != "" {
				// Remove from Redis queue (LREM removes all matching values)
				// Log error but don't fail - DB is source of truth, job is already cancelled
				if err := s.redis.LRem(ctx, queueName, 0, payloadJSON).Err(); err != nil {
					log.Printf("Warning: failed to remove job %s from Redis queue: %v", jobId.String(), err)
				}
			}
		}
	}

	return &schedulerv1.CancelJobResponse{
		Cancelled: cancelled,
	}, nil
}

// ListJobs lists jobs with optional filters
func (s *Server) ListJobs(ctx context.Context, req *schedulerv1.ListJobsRequest) (*schedulerv1.ListJobsResponse, error) {
	// 1. Build query with filters (state, priority, type)
	
	priorityString := priorityToString(req.PriorityFilter)

	// 2. Query CockroachDB with pagination

	// Handle pagination: default page_size to 50, max 1000
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 50
	}
	if pageSize > 1000 {
		pageSize = 1000
	}

	// Parse page_token as offset (default to 0)
	offset := 0
	if req.PageToken != "" {
		parsedOffset, err := strconv.Atoi(req.PageToken)
		if err == nil {
			offset = parsedOffset
		}
	}

	query := "SELECT task_id, type, priority, payload, status, attempts, max_attempts, next_run_at, created_at, updated_at FROM tasks WHERE 1=1"
	args := []interface{}{}
	argPos := 1

	// Add state filter
	if req.StateFilter != schedulerv1.JobState_JOB_STATE_UNSPECIFIED {
		jobStateString := jobStateToString(req.StateFilter)
		if jobStateString != "" {
			query += fmt.Sprintf(" AND status = $%d", argPos)
			args = append(args, jobStateString)
			argPos++
		}
	}

	// Add priority filter
	if req.PriorityFilter != schedulerv1.Priority_PRIORITY_UNSPECIFIED {
		if priorityString != "" {
			query += fmt.Sprintf(" AND priority = $%d", argPos)
			args = append(args, priorityString)
			argPos++
		}
	}

	// Add type filter
	if req.TypeFilter != "" {
		query += fmt.Sprintf(" AND type = $%d", argPos)
		args = append(args, req.TypeFilter)
		argPos++
	}

	query += " ORDER BY created_at DESC"
	query += fmt.Sprintf(" LIMIT $%d", argPos)
	args = append(args, pageSize)
	argPos++
	query += fmt.Sprintf(" OFFSET $%d", argPos)
	args = append(args, offset)

	// 3. Execute query and convert results to JobStatus messages
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to query jobs: %v", err))
	}
	defer rows.Close()

	var jobs []*schedulerv1.JobStatus
	for rows.Next() {
		var job db.Job
		var nextRunAt *time.Time
		err := rows.Scan(
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
			return nil, status.Error(codes.Internal, fmt.Sprintf("failed to scan job: %v", err))
		}
		// Note: nextRunAt is scanned but not used since JobStatus protobuf doesn't include it

		// Convert to protobuf JobStatus
		jobStatus := &schedulerv1.JobStatus{
			JobId:     job.TaskID.String(),
			State:     statusToJobState(job.Status),
			Attempts:  int32(job.Attempts),
			LastError: "",
			CreatedAt: timestamppb.New(job.CreatedAt),
			UpdatedAt: timestamppb.New(job.UpdatedAt),
		}
		jobs = append(jobs, jobStatus)
	}

	if err := rows.Err(); err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("error iterating rows: %v", err))
	}

	// 4. Generate next_page_token
	nextPageToken := ""
	if len(jobs) == pageSize {
		// There might be more results
		nextPageToken = strconv.Itoa(offset + pageSize)
	}

	return &schedulerv1.ListJobsResponse{
		Jobs:          jobs,
		NextPageToken: nextPageToken,
	}, nil
}

