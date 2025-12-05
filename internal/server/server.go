package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	redisc "github.com/redis/go-redis/v9"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/db"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/redis"
	schedulerv1 "github.com/williamntlam/go-grpc-task-scheduler/proto/scheduler/v1"
)

// Server implements the SchedulerService gRPC interface
// This struct holds the dependencies needed for all service methods
type Server struct {

	dbPool *pgxpool.Pool
	redisClient *redisc.Client
}

// NewServer creates a new Server instance
// This is the constructor function that initializes the Server struct
func NewServer(dbPool *pgxpool.Pool, redisClient *redisc.Client) *Server {

	return &Server{
		dbPool: dbPool,
		redisClient: redisClient,
	}

}

// ============================================================================
// HELPER FUNCTIONS (Implement these first - they'll be used by all methods)
// ============================================================================

// priorityProtoToString converts protobuf Priority enum to database string
// This is needed because:
//   - Protobuf uses: PRIORITY_CRITICAL, PRIORITY_HIGH, PRIORITY_DEFAULT, PRIORITY_LOW
//   - Database/Redis uses: "critical", "high", "default", "low"
// Function signature: func priorityProtoToString(priority schedulerv1.Priority) string
func priorityProtoToString(priority schedulerv1.Priority) string {
	// STEP 3: Implement priority conversion
	// Use a switch statement to map protobuf enum values to strings:
	//   - schedulerv1.Priority_PRIORITY_CRITICAL -> "critical"
	//   - schedulerv1.Priority_PRIORITY_HIGH -> "high"
	//   - schedulerv1.Priority_PRIORITY_DEFAULT -> "default"
	//   - schedulerv1.Priority_PRIORITY_LOW -> "low"
	//   - schedulerv1.Priority_PRIORITY_UNSPECIFIED -> "default" (default fallback)
	// Example:
	//   switch priority {
	//   case schedulerv1.Priority_PRIORITY_CRITICAL:
	//       return "critical"
	//   case schedulerv1.Priority_PRIORITY_HIGH:
	//       return "high"
	//   ... etc
	//   default:
	//       return "default"
	//   }

	// TODO: Implement priority conversion
	switch priority {
	case schedulerv1.Priority_PRIORITY_CRITICAL:
		return "critical"
	case schedulerv1.Priority_PRIORITY_HIGH:
		return "high"
	case schedulerv1.Priority_PRIORITY_DEFAULT:
		return "default"
	case schedulerv1.Priority_PRIORITY_LOW:
		return "low"
	default:
		return "default"
	}
}

// statusStringToProto converts database status string to protobuf JobState enum
// This is needed because:
//   - Database uses: "queued", "running", "succeeded", "failed", "deadletter", "retry", "cancelled"
//   - Protobuf uses: JOB_STATE_QUEUED, JOB_STATE_RUNNING, JOB_STATE_SUCCEEDED, etc.
// Function signature: func statusStringToProto(status string) schedulerv1.JobState
func statusStringToProto(status string) schedulerv1.JobState {
	// STEP 4: Implement status conversion
	// Use a switch statement to map database strings to protobuf enums:
	//   - "queued" -> schedulerv1.JobState_JOB_STATE_QUEUED
	//   - "running" -> schedulerv1.JobState_JOB_STATE_RUNNING
	//   - "succeeded" -> schedulerv1.JobState_JOB_STATE_SUCCEEDED
	//   - "failed" -> schedulerv1.JobState_JOB_STATE_FAILED
	//   - "deadletter" -> schedulerv1.JobState_JOB_STATE_DEADLETTER
	//   - "retry" -> schedulerv1.JobState_JOB_STATE_QUEUED (retry jobs are queued for reprocessing)
	//   - "cancelled" -> schedulerv1.JobState_JOB_STATE_FAILED (or create a new state if needed)
	//   - default -> schedulerv1.JobState_JOB_STATE_UNSPECIFIED
	// Example:
	//   switch status {
	//   case "queued":
	//       return schedulerv1.JobState_JOB_STATE_QUEUED
	//   ... etc
	//   default:
	//       return schedulerv1.JobState_JOB_STATE_UNSPECIFIED
	//   }

	// TODO: Implement status conversion
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
	case "retry":
		return schedulerv1.JobState_JOB_STATE_QUEUED
	case "cancelled":
		return schedulerv1.JobState_JOB_STATE_FAILED
	default:
		return schedulerv1.JobState_JOB_STATE_UNSPECIFIED
	}
}

// convertJobToJobStatus converts a database Job to protobuf JobStatus
// This helper function is used by GetJob and ListJobs
// Function signature: func convertJobToJobStatus(job *db.Job) (*schedulerv1.JobStatus, error)
func convertJobToJobStatus(job *db.Job) (*schedulerv1.JobStatus, error) {
	// STEP 5: Implement job conversion
	// Create a new schedulerv1.JobStatus struct and populate all fields:
	//   - job_id: job.TaskID.String() (convert UUID to string)
	//   - state: use statusStringToProto(job.Status) to convert status
	//   - attempts: int32(job.Attempts) (convert int to int32)
	//   - last_error: "" (you'll need to get this from task_attempts table if needed, or leave empty for now)
	//   - created_at: timestamppb.New(job.CreatedAt) (convert time.Time to protobuf timestamp)
	//   - updated_at: timestamppb.New(job.UpdatedAt)
	//   - started_at: check if job has a started_at field, or query task_attempts table
	//     For now, you can leave started_at and finished_at as nil if not available
	//   - finished_at: similar to started_at
	//
	// Note: If you need started_at/finished_at, you'll need to query task_attempts table
	// For now, you can return nil for these fields
	//
	// Example:
	//   return &schedulerv1.JobStatus{
	//       JobId:    job.TaskID.String(),
	//       State:    statusStringToProto(job.Status),
	//       Attempts: int32(job.Attempts),
	//       LastError: "",
	//       CreatedAt: timestamppb.New(job.CreatedAt),
	//       UpdatedAt: timestamppb.New(job.UpdatedAt),
	//       StartedAt: nil, // TODO: Query from task_attempts if needed
	//       FinishedAt: nil, // TODO: Query from task_attempts if needed
	//   }, nil

	// TODO: Implement job conversion
	return &schedulerv1.JobStatus{
		JobId:     job.TaskID.String(),
		State:     statusStringToProto(job.Status),
		Attempts:   int32(job.Attempts),
		LastError:  "", // TODO: Query from task_attempts table if needed
		CreatedAt:  timestamppb.New(job.CreatedAt),
		UpdatedAt:  timestamppb.New(job.UpdatedAt),
		StartedAt:  nil, // TODO: Query from task_attempts table if needed
		FinishedAt: nil, // TODO: Query from task_attempts table if needed
	}, nil
}

// ============================================================================
// gRPC SERVICE METHODS
// ============================================================================

// SubmitJob creates a new job in the database and pushes it to Redis queue
// This is the main entry point for job submission
func (s *Server) SubmitJob(ctx context.Context, req *schedulerv1.SubmitJobRequest) (*schedulerv1.SubmitJobResponse, error) {
	// STEP 6: Validate the request
	// Check if req.Job is nil
	// If nil, return error: status.Error(codes.InvalidArgument, "job is required")
	// Example:
	//   if req.Job == nil {
	//       return nil, status.Error(codes.InvalidArgument, "job is required")
	//   }

	// TODO: Validate request
	if req.Job == nil {
		return nil, status.Error(codes.InvalidArgument, "job is required")
	}

	// STEP 7: Generate or use provided job ID
	// Check if req.Job.JobId is provided (non-empty string)
	// If provided: parse it as UUID using uuid.Parse()
	//   - If parse fails, return error: status.Error(codes.InvalidArgument, "invalid job_id format")
	// If not provided: generate new UUID using uuid.New()
	// Example:
	//   var jobID uuid.UUID
	//   var err error
	//   if req.Job.JobId != "" {
	//       jobID, err = uuid.Parse(req.Job.JobId)
	//       if err != nil {
	//           return nil, status.Error(codes.InvalidArgument, "invalid job_id format")
	//       }
	//   } else {
	//       jobID = uuid.New()
	//   }

	// TODO: Generate or parse job ID
	var jobID uuid.UUID
	var err error
	if req.Job.JobId != "" {
		jobID, err = uuid.Parse(req.Job.JobId)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid job_id format")
		}
	} else {
		jobID = uuid.New()
	}

	// STEP 8: Validate and convert priority
	// Convert req.Job.Priority to string using priorityProtoToString()
	// Validate that priority is not UNSPECIFIED (or handle it as default)
	// Example:
	//   priorityStr := priorityProtoToString(req.Job.Priority)
	//   if req.Job.Priority == schedulerv1.Priority_PRIORITY_UNSPECIFIED {
	//       priorityStr = "default" // Use default if unspecified
	//   }

	// TODO: Convert and validate priority
	priorityStr := priorityProtoToString(req.Job.Priority)
	if req.Job.Priority == schedulerv1.Priority_PRIORITY_UNSPECIFIED {
		priorityStr = "default"
	}

	// STEP 9: Validate job type
	// Check if req.Job.Type is empty
	// If empty, return error: status.Error(codes.InvalidArgument, "job type is required")
	// Example:
	//   if req.Job.Type == "" {
	//       return nil, status.Error(codes.InvalidArgument, "job type is required")
	//   }

	// TODO: Validate job type
	if req.Job.Type == "" {
		return nil, status.Error(codes.InvalidArgument, "job type is required")
	}

	// STEP 10: Validate payload JSON (if provided)
	// If req.Job.PayloadJson is not empty, validate it's valid JSON
	// Use json.Unmarshal() to test if it's valid JSON
	// If invalid, return error: status.Error(codes.InvalidArgument, "invalid payload_json")
	// Example:
	//   if req.Job.PayloadJson != "" {
	//       var testPayload interface{}
	//       if err := json.Unmarshal([]byte(req.Job.PayloadJson), &testPayload); err != nil {
	//           return nil, status.Error(codes.InvalidArgument, "invalid payload_json: "+err.Error())
	//       }
	//   }

	// TODO: Validate payload JSON
	if req.Job.PayloadJson != "" {
		var testPayload interface{}
		if err := json.Unmarshal([]byte(req.Job.PayloadJson), &testPayload); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid payload_json: "+err.Error())
		}
	}

	// STEP 11: Set default max_attempts if not provided
	// If req.Job.MaxAttempts is 0 or negative, set it to 3 (default)
	// Example:
	//   maxAttempts := int(req.Job.MaxAttempts)
	//   if maxAttempts <= 0 {
	//       maxAttempts = 3
	//   }

	// TODO: Set default max_attempts
	maxAttempts := int(req.Job.MaxAttempts)
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	// STEP 12: Create Job struct for database
	// Create a db.Job struct with:
	//   - TaskID: jobID (UUID)
	//   - Type: req.Job.Type
	//   - Priority: priorityStr
	//   - PayloadJSON: json.RawMessage(req.Job.PayloadJson) (convert string to json.RawMessage)
	//   - Status: "queued" (all new jobs start as queued)
	//   - Attempts: 0 (no attempts yet)
	//   - MaxAttempts: maxAttempts
	//   - NextRunAt: nil (no retry scheduled yet)
	// Example:
	//   dbJob := db.Job{
	//       TaskID:      jobID,
	//       Type:        req.Job.Type,
	//       Priority:    priorityStr,
	//       PayloadJSON: json.RawMessage(req.Job.PayloadJson),
	//       Status:      "queued",
	//       Attempts:    0,
	//       MaxAttempts: maxAttempts,
	//       NextRunAt:  nil,
	//   }

	// TODO: Create db.Job struct
	dbJob := db.Job{
		TaskID:      jobID,
		Type:        req.Job.Type,
		Priority:    priorityStr,
		PayloadJSON: json.RawMessage(req.Job.PayloadJson),
		Status:      "queued",
		Attempts:    0,
		MaxAttempts: maxAttempts,
		NextRunAt:   nil,
	}

	// STEP 13: Insert job into database
	// Call db.CreateJob(ctx, s.dbPool, dbJob)
	// This function handles idempotency (if job_id already exists, returns existing job)
	// Handle errors: if error, return status.Error(codes.Internal, "failed to create job: "+err.Error())
	// Example:
	//   createdJobID, alreadyExists, err := db.CreateJob(ctx, s.dbPool, dbJob)
	//   if err != nil {
	//       log.Printf("Failed to create job: %v", err)
	//       return nil, status.Error(codes.Internal, "failed to create job: "+err.Error())
	//   }
	//   if alreadyExists {
	//       log.Printf("Job %s already exists (idempotency)", createdJobID)
	//   }

	// TODO: Insert job into database
	createdJobID, alreadyExists, err := db.CreateJob(ctx, s.dbPool, dbJob)
	if err != nil {
		log.Printf("Failed to create job: %v", err)
		return nil, status.Error(codes.Internal, "failed to create job: "+err.Error())
	}
	if alreadyExists {
		log.Printf("Job %s already exists (idempotency)", createdJobID)
	}

	// STEP 14: Push job to Redis queue
	// Call redis.PushJob(ctx, s.redisClient, createdJobID, req.Job.Type, priorityStr)
	// Handle errors: log error but don't fail the request (job is already in DB)
	// This is important: even if Redis fails, the job is in the database and can be recovered
	// Example:
	//   if err := redis.PushJob(ctx, s.redisClient, createdJobID, req.Job.Type, priorityStr); err != nil {
	//       log.Printf("Warning: Failed to push job %s to Redis queue: %v", createdJobID, err)
	//       // Continue anyway - job is in DB, can be recovered later
	//   } else {
	//       log.Printf("Job %s pushed to Redis queue %s", createdJobID, priorityStr)
	//   }

	// TODO: Push job to Redis queue
	if err := redis.PushJob(ctx, s.redisClient, createdJobID, req.Job.Type, priorityStr); err != nil {
		log.Printf("Warning: Failed to push job %s to Redis queue: %v", createdJobID, err)
		// Continue anyway - job is in DB, can be recovered later
	} else {
		log.Printf("Job %s pushed to Redis queue %s", createdJobID, priorityStr)
	}

	// STEP 15: Return response
	// Return SubmitJobResponse with the job_id as string
	// Example:
	//   return &schedulerv1.SubmitJobResponse{
	//       JobId: createdJobID.String(),
	//   }, nil

	// TODO: Return response
	return &schedulerv1.SubmitJobResponse{
		JobId: createdJobID.String(),
	}, nil
}

// GetJob retrieves a job by ID and returns its status
func (s *Server) GetJob(ctx context.Context, req *schedulerv1.GetJobRequest) (*schedulerv1.JobStatus, error) {
	// STEP 16: Validate request
	// Check if req.JobId is empty
	// If empty, return error: status.Error(codes.InvalidArgument, "job_id is required")
	// Example:
	//   if req.JobId == "" {
	//       return nil, status.Error(codes.InvalidArgument, "job_id is required")
	//   }

	// TODO: Validate request
	if req.JobId == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}

	// STEP 17: Parse job ID
	// Parse req.JobId as UUID using uuid.Parse()
	// If parse fails, return error: status.Error(codes.InvalidArgument, "invalid job_id format")
	// Example:
	//   jobID, err := uuid.Parse(req.JobId)
	//   if err != nil {
	//       return nil, status.Error(codes.InvalidArgument, "invalid job_id format")
	//   }

	// TODO: Parse job ID
	jobID, err := uuid.Parse(req.JobId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid job_id format")
	}

	// STEP 18: Query database
	// Call db.GetJobByID(ctx, s.dbPool, jobID)
	// Handle errors: if error, return status.Error(codes.Internal, "failed to get job: "+err.Error())
	// If job is nil (not found), return status.Error(codes.NotFound, "job not found")
	// Example:
	//   job, err := db.GetJobByID(ctx, s.dbPool, jobID)
	//   if err != nil {
	//       log.Printf("Failed to get job %s: %v", jobID, err)
	//       return nil, status.Error(codes.Internal, "failed to get job: "+err.Error())
	//   }
	//   if job == nil {
	//       return nil, status.Error(codes.NotFound, "job not found")
	//   }

	// TODO: Query database
	job, err := db.GetJobByID(ctx, s.dbPool, jobID)
	if err != nil {
		log.Printf("Failed to get job %s: %v", jobID, err)
		return nil, status.Error(codes.Internal, "failed to get job: "+err.Error())
	}
	if job == nil {
		return nil, status.Error(codes.NotFound, "job not found")
	}

	// STEP 19: Convert to protobuf
	// Use convertJobToJobStatus(job) to convert database job to protobuf JobStatus
	// Handle errors: if error, return status.Error(codes.Internal, "failed to convert job: "+err.Error())
	// Example:
	//   jobStatus, err := convertJobToJobStatus(job)
	//   if err != nil {
	//       log.Printf("Failed to convert job %s: %v", jobID, err)
	//       return nil, status.Error(codes.Internal, "failed to convert job: "+err.Error())
	//   }

	// TODO: Convert to protobuf
	jobStatus, err := convertJobToJobStatus(job)
	if err != nil {
		log.Printf("Failed to convert job %s: %v", jobID, err)
		return nil, status.Error(codes.Internal, "failed to convert job: "+err.Error())
	}

	// STEP 20: Return response
	// Return the jobStatus
	// Example:
	//   return jobStatus, nil

	// TODO: Return response
	return jobStatus, nil
}

// WatchJob streams job status updates until the job completes or is cancelled
// This is a server streaming RPC - you send multiple responses
func (s *Server) WatchJob(req *schedulerv1.WatchJobRequest, stream schedulerv1.SchedulerService_WatchJobServer) error {
	// STEP 21: Validate request
	// Check if req.JobId is empty
	// If empty, return error: status.Error(codes.InvalidArgument, "job_id is required")
	// Example:
	//   if req.JobId == "" {
	//       return status.Error(codes.InvalidArgument, "job_id is required")
	//   }

	// TODO: Validate request
	if req.JobId == "" {
		return status.Error(codes.InvalidArgument, "job_id is required")
	}

	// STEP 22: Parse job ID
	// Parse req.JobId as UUID using uuid.Parse()
	// If parse fails, return error: status.Error(codes.InvalidArgument, "invalid job_id format")
	// Example:
	//   jobID, err := uuid.Parse(req.JobId)
	//   if err != nil {
	//       return status.Error(codes.InvalidArgument, "invalid job_id format")
	//   }

	// TODO: Parse job ID
	jobID, err := uuid.Parse(req.JobId)
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid job_id format")
	}

	// STEP 23: Check if job exists (initial check)
	// Call db.GetJobByID(ctx, s.dbPool, jobID) using stream.Context()
	// If job is nil, return error: status.Error(codes.NotFound, "job not found")
	// Example:
	//   job, err := db.GetJobByID(stream.Context(), s.dbPool, jobID)
	//   if err != nil {
	//       return status.Error(codes.Internal, "failed to get job: "+err.Error())
	//   }
	//   if job == nil {
	//       return status.Error(codes.NotFound, "job not found")
	//   }

	// TODO: Check if job exists
	job, err := db.GetJobByID(stream.Context(), s.dbPool, jobID)
	if err != nil {
		return status.Error(codes.Internal, "failed to get job: "+err.Error())
	}
	if job == nil {
		return status.Error(codes.NotFound, "job not found")
	}

	// STEP 24: Set up polling
	// Create a ticker that fires every 1 second: time.NewTicker(1 * time.Second)
	// Always defer ticker.Stop() to prevent resource leaks
	// Track the last known status to avoid sending duplicate events
	// Example:
	//   ticker := time.NewTicker(1 * time.Second)
	//   defer ticker.Stop()
	//   lastStatus := ""

	// TODO: Set up polling
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	lastStatus := ""

	// STEP 25: Send initial status
	// Convert job to JobStatus using convertJobToJobStatus(job)
	// Create a JobEvent with:
	//   - job_id: jobID.String()
	//   - state: jobStatus.State
	//   - message: "Job status: " + job.Status
	//   - timestamp: timestamppb.Now()
	//   - attempts: jobStatus.Attempts
	// Send it using stream.Send(jobEvent)
	// Update lastStatus = job.Status
	// Example:
	//   jobStatus, err := convertJobToJobStatus(job)
	//   if err != nil {
	//       return status.Error(codes.Internal, "failed to convert job: "+err.Error())
	//   }
	//   initialEvent := &schedulerv1.JobEvent{
	//       JobId:     jobID.String(),
	//       State:     jobStatus.State,
	//       Message:   "Job status: " + job.Status,
	//       Timestamp: timestamppb.Now(),
	//       Attempts:   jobStatus.Attempts,
	//   }
	//   if err := stream.Send(initialEvent); err != nil {
	//       return err
	//   }
	//   lastStatus = job.Status

	// TODO: Send initial status
	jobStatus, err := convertJobToJobStatus(job)
	if err != nil {
		return status.Error(codes.Internal, "failed to convert job: "+err.Error())
	}
	initialEvent := &schedulerv1.JobEvent{
		JobId:     jobID.String(),
		State:     jobStatus.State,
		Message:   "Job status: " + job.Status,
		Timestamp: timestamppb.Now(),
		Attempts:   jobStatus.Attempts,
	}
	if err := stream.Send(initialEvent); err != nil {
		return err
	}
	lastStatus = job.Status

	// STEP 26: Polling loop
	// Use a for loop with select statement:
	//   - case <-stream.Context().Done(): return nil (client cancelled)
	//   - case <-ticker.C: poll database and send update if status changed
	// Example:
	//   for {
	//       select {
	//       case <-stream.Context().Done():
	//           return nil
	//       case <-ticker.C:
	//           // Poll database and check for status change
	//       }
	//   }

	// TODO: Implement polling loop
	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-ticker.C:
			// STEP 27: Poll database in ticker case
			// Query job again: db.GetJobByID(stream.Context(), s.dbPool, jobID)
			// If error, log and continue (don't break the stream)
			// If job is nil, send error event and return
			// Example:
			//   currentJob, err := db.GetJobByID(stream.Context(), s.dbPool, jobID)
			//   if err != nil {
			//       log.Printf("WatchJob: Error polling job %s: %v", jobID, err)
			//       continue
			//   }
			//   if currentJob == nil {
			//       // Job was deleted
			//       return status.Error(codes.NotFound, "job not found")
			//   }

			// TODO: Poll database
			currentJob, err := db.GetJobByID(stream.Context(), s.dbPool, jobID)
			if err != nil {
				log.Printf("WatchJob: Error polling job %s: %v", jobID, err)
				continue
			}
			if currentJob == nil {
				return status.Error(codes.NotFound, "job not found")
			}

			// STEP 28: Check if status changed
			// Compare currentJob.Status with lastStatus
			// If different, convert to JobStatus and send event
			// Update lastStatus = currentJob.Status
			// Example:
			//   if currentJob.Status != lastStatus {
			//       currentJobStatus, err := convertJobToJobStatus(currentJob)
			//       if err != nil {
			//           log.Printf("WatchJob: Error converting job %s: %v", jobID, err)
			//           continue
			//       }
			//       event := &schedulerv1.JobEvent{
			//           JobId:     jobID.String(),
			//           State:     currentJobStatus.State,
			//           Message:   "Job status changed to: " + currentJob.Status,
			//           Timestamp: timestamppb.Now(),
			//           Attempts:   currentJobStatus.Attempts,
			//       }
			//       if err := stream.Send(event); err != nil {
			//           return err
			//       }
			//       lastStatus = currentJob.Status
			//   }

			// TODO: Check if status changed
			if currentJob.Status != lastStatus {
				currentJobStatus, err := convertJobToJobStatus(currentJob)
				if err != nil {
					log.Printf("WatchJob: Error converting job %s: %v", jobID, err)
					continue
				}
				event := &schedulerv1.JobEvent{
					JobId:     jobID.String(),
					State:     currentJobStatus.State,
					Message:   "Job status changed to: " + currentJob.Status,
					Timestamp: timestamppb.Now(),
					Attempts:   currentJobStatus.Attempts,
				}
				if err := stream.Send(event); err != nil {
					return err
				}
				lastStatus = currentJob.Status
			}

			// STEP 29: Check if job is complete
			// If job status is "succeeded", "failed", or "deadletter", send final event and return
			// This prevents polling forever
			// Example:
			//   if currentJob.Status == "succeeded" || currentJob.Status == "failed" || currentJob.Status == "deadletter" {
			//       // Send final event (already sent above if status changed)
			//       return nil
			//   }

			// TODO: Check if job is complete
			if currentJob.Status == "succeeded" || currentJob.Status == "failed" || currentJob.Status == "deadletter" {
				return nil
			}
		}
	}
}

// CancelJob cancels a job if it's queued or running
func (s *Server) CancelJob(ctx context.Context, req *schedulerv1.CancelJobRequest) (*schedulerv1.CancelJobResponse, error) {
	// STEP 30: Validate request
	// Check if req.JobId is empty
	// If empty, return error: status.Error(codes.InvalidArgument, "job_id is required")
	// Example:
	//   if req.JobId == "" {
	//       return nil, status.Error(codes.InvalidArgument, "job_id is required")
	//   }

	// TODO: Validate request
	if req.JobId == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}

	// STEP 31: Parse job ID
	// Parse req.JobId as UUID using uuid.Parse()
	// If parse fails, return error: status.Error(codes.InvalidArgument, "invalid job_id format")
	// Example:
	//   jobID, err := uuid.Parse(req.JobId)
	//   if err != nil {
	//       return nil, status.Error(codes.InvalidArgument, "invalid job_id format")
	//   }

	// TODO: Parse job ID
	jobID, err := uuid.Parse(req.JobId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid job_id format")
	}

	// STEP 32: Check if job exists
	// Call db.GetJobByID(ctx, s.dbPool, jobID) to get current job state
	// If job is nil, return error: status.Error(codes.NotFound, "job not found")
	// Example:
	//   job, err := db.GetJobByID(ctx, s.dbPool, jobID)
	//   if err != nil {
	//       log.Printf("Failed to get job %s: %v", jobID, err)
	//       return nil, status.Error(codes.Internal, "failed to get job: "+err.Error())
	//   }
	//   if job == nil {
	//       return nil, status.Error(codes.NotFound, "job not found")
	//   }

	// TODO: Check if job exists
	job, err := db.GetJobByID(ctx, s.dbPool, jobID)
	if err != nil {
		log.Printf("Failed to get job %s: %v", jobID, err)
		return nil, status.Error(codes.Internal, "failed to get job: "+err.Error())
	}
	if job == nil {
		return nil, status.Error(codes.NotFound, "job not found")
	}

	// STEP 33: Update database status
	// Call db.CancelJob(ctx, s.dbPool, jobID)
	// This function atomically updates status to 'cancelled' if status is 'queued' or 'running'
	// It returns (bool, error) - bool indicates if job was actually cancelled
	// Handle errors: if error, return status.Error(codes.Internal, "failed to cancel job: "+err.Error())
	// Example:
	//   cancelled, err := db.CancelJob(ctx, s.dbPool, jobID)
	//   if err != nil {
	//       log.Printf("Failed to cancel job %s: %v", jobID, err)
	//       return nil, status.Error(codes.Internal, "failed to cancel job: "+err.Error())
	//   }
	//   if !cancelled {
	//       // Job was not in a cancellable state (already completed or cancelled)
	//       return &schedulerv1.CancelJobResponse{Cancelled: false}, nil
	//   }

	// TODO: Update database status
	cancelled, err := db.CancelJob(ctx, s.dbPool, jobID)
	if err != nil {
		log.Printf("Failed to cancel job %s: %v", jobID, err)
		return nil, status.Error(codes.Internal, "failed to cancel job: "+err.Error())
	}
	if !cancelled {
		return &schedulerv1.CancelJobResponse{Cancelled: false}, nil
	}

	// STEP 34: Remove from Redis queue (if job was queued)
	// Only remove from Redis if job status was "queued" (not "running")
	// If job was "running", it's already being processed by a worker
	// To remove from Redis:
	//   1. Get queue name: redis.GetQueueName(job.Priority)
	//   2. Create job payload: redis.JobPayload{TaskID: jobID.String(), Type: job.Type, Priority: job.Priority}
	//   3. Marshal to JSON: json.Marshal(payload)
	//   4. Use LREM to remove: s.redisClient.LRem(ctx, queueName, 0, payloadJSON)
	//      - LREM removes all matching values (count=0 means remove all)
	// Handle errors: log error but don't fail (job is already cancelled in DB)
	// Example:
	//   if job.Status == "queued" {
	//       queueName := redis.GetQueueName(job.Priority)
	//       if queueName != "" {
	//           payload := redis.JobPayload{
	//               TaskID:   jobID.String(),
	//               Type:     job.Type,
	//               Priority: job.Priority,
	//           }
	//           payloadJSON, err := json.Marshal(payload)
	//           if err == nil {
	//               if err := s.redisClient.LRem(ctx, queueName, 0, string(payloadJSON)).Err(); err != nil {
	//                   log.Printf("Warning: Failed to remove job %s from Redis queue: %v", jobID, err)
	//               } else {
	//                   log.Printf("Removed job %s from Redis queue %s", jobID, queueName)
	//               }
	//           }
	//       }
	//   }

	// TODO: Remove from Redis queue
	if job.Status == "queued" {
		queueName := redis.GetQueueName(job.Priority)
		if queueName != "" {
			payload := redis.JobPayload{
				TaskID:   jobID.String(),
				Type:     job.Type,
				Priority: job.Priority,
			}
			payloadJSON, err := json.Marshal(payload)
			if err == nil {
				if err := s.redisClient.LRem(ctx, queueName, 0, string(payloadJSON)).Err(); err != nil {
					log.Printf("Warning: Failed to remove job %s from Redis queue: %v", jobID, err)
				} else {
					log.Printf("Removed job %s from Redis queue %s", jobID, queueName)
				}
			}
		}
	}

	// STEP 35: Return response
	// Return CancelJobResponse with Cancelled: true
	// Example:
	//   return &schedulerv1.CancelJobResponse{Cancelled: true}, nil

	// TODO: Return response
	return &schedulerv1.CancelJobResponse{Cancelled: true}, nil
}

// ListJobs lists jobs with optional filters and pagination
func (s *Server) ListJobs(ctx context.Context, req *schedulerv1.ListJobsRequest) (*schedulerv1.ListJobsResponse, error) {
	// STEP 36: Build SQL query dynamically
	// Start with base query: "SELECT task_id, type, priority, payload, status, attempts, max_attempts, next_run_at, created_at, updated_at FROM tasks WHERE 1=1"
	// The "WHERE 1=1" trick makes it easier to add conditions
	// Create a slice for arguments: args := []interface{}{}
	// Create an argument position counter: argPos := 1
	// Example:
	//   query := "SELECT task_id, type, priority, payload, status, attempts, max_attempts, next_run_at, created_at, updated_at FROM tasks WHERE 1=1"
	//   args := []interface{}{}
	//   argPos := 1

	// TODO: Build SQL query
	query := "SELECT task_id, type, priority, payload, status, attempts, max_attempts, next_run_at, created_at, updated_at FROM tasks WHERE 1=1"
	args := []interface{}{}
	argPos := 1

	// STEP 37: Add state filter
	// If req.StateFilter is not JOB_STATE_UNSPECIFIED:
	//   - Convert to string using statusStringToProto reverse lookup (or create a helper)
	//   - Actually, you need the reverse: convert JobState enum to database string
	//   - Create a helper: func jobStateToStatusString(state schedulerv1.JobState) string
	//   - Add " AND status = $N" to query
	//   - Append the status string to args
	//   - Increment argPos
	// Example:
	//   if req.StateFilter != schedulerv1.JobState_JOB_STATE_UNSPECIFIED {
	//       statusStr := jobStateToStatusString(req.StateFilter)
	//       if statusStr != "" {
	//           query += fmt.Sprintf(" AND status = $%d", argPos)
	//           args = append(args, statusStr)
	//           argPos++
	//       }
	//   }

	// TODO: Add state filter
	if req.StateFilter != schedulerv1.JobState_JOB_STATE_UNSPECIFIED {
		statusStr := jobStateToStatusString(req.StateFilter)
		if statusStr != "" {
			query += fmt.Sprintf(" AND status = $%d", argPos)
			args = append(args, statusStr)
			argPos++
		}
	}

	// STEP 38: Add priority filter
	// If req.PriorityFilter is not PRIORITY_UNSPECIFIED:
	//   - Convert to string using priorityProtoToString()
	//   - Add " AND priority = $N" to query
	//   - Append the priority string to args
	//   - Increment argPos
	// Example:
	//   if req.PriorityFilter != schedulerv1.Priority_PRIORITY_UNSPECIFIED {
	//       priorityStr := priorityProtoToString(req.PriorityFilter)
	//       query += fmt.Sprintf(" AND priority = $%d", argPos)
	//       args = append(args, priorityStr)
	//       argPos++
	//   }

	// TODO: Add priority filter
	if req.PriorityFilter != schedulerv1.Priority_PRIORITY_UNSPECIFIED {
		priorityStr := priorityProtoToString(req.PriorityFilter)
		query += fmt.Sprintf(" AND priority = $%d", argPos)
		args = append(args, priorityStr)
		argPos++
	}

	// STEP 39: Add type filter
	// If req.TypeFilter is not empty:
	//   - Add " AND type = $N" to query
	//   - Append req.TypeFilter to args
	//   - Increment argPos
	// Example:
	//   if req.TypeFilter != "" {
	//       query += fmt.Sprintf(" AND type = $%d", argPos)
	//       args = append(args, req.TypeFilter)
	//       argPos++
	//   }

	// TODO: Add type filter
	if req.TypeFilter != "" {
		query += fmt.Sprintf(" AND type = $%d", argPos)
		args = append(args, req.TypeFilter)
		argPos++
	}

	// STEP 40: Add ordering
	// Add " ORDER BY created_at DESC" to query (newest first)
	// Example:
	//   query += " ORDER BY created_at DESC"

	// TODO: Add ordering
	query += " ORDER BY created_at DESC"

	// STEP 41: Handle pagination
	// Set default page_size if not provided or invalid:
	//   - If req.PageSize <= 0, set to 50 (default)
	//   - If req.PageSize > 1000, cap at 1000 (max)
	// Add " LIMIT $N" to query
	// Append pageSize to args
	// Increment argPos
	// Example:
	//   pageSize := int(req.PageSize)
	//   if pageSize <= 0 {
	//       pageSize = 50
	//   }
	//   if pageSize > 1000 {
	//       pageSize = 1000
	//   }
	//   query += fmt.Sprintf(" LIMIT $%d", argPos)
	//   args = append(args, pageSize)
	//   argPos++

	// TODO: Handle pagination
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	query += fmt.Sprintf(" LIMIT $%d", argPos)
	args = append(args, pageSize)
	argPos++

	// STEP 42: Handle page_token (offset)
	// If req.PageToken is provided, parse it as an integer (offset)
	// Add " OFFSET $N" to query
	// Append offset to args
	// For now, you can use a simple integer offset (page_token = "0", "50", "100", etc.)
	// In production, you might use cursor-based pagination with UUIDs
	// Example:
	//   offset := 0
	//   if req.PageToken != "" {
	//       // Simple offset-based pagination
	//       // Parse page_token as integer
	//       if parsedOffset, err := strconv.Atoi(req.PageToken); err == nil {
	//           offset = parsedOffset
	//       }
	//   }
	//   query += fmt.Sprintf(" OFFSET $%d", argPos)
	//   args = append(args, offset)

	// TODO: Handle page_token
	offset := 0
	if req.PageToken != "" {
		if parsedOffset, err := strconv.Atoi(req.PageToken); err == nil {
			offset = parsedOffset
		}
	}
	query += fmt.Sprintf(" OFFSET $%d", argPos)
	args = append(args, offset)

	// STEP 43: Execute query
	// Use s.dbPool.Query(ctx, query, args...)
	// Handle errors: if error, return status.Error(codes.Internal, "failed to query jobs: "+err.Error())
	// Always defer rows.Close()
	// Example:
	//   rows, err := s.dbPool.Query(ctx, query, args...)
	//   if err != nil {
	//       log.Printf("Failed to query jobs: %v", err)
	//       return nil, status.Error(codes.Internal, "failed to query jobs: "+err.Error())
	//   }
	//   defer rows.Close()

	// TODO: Execute query
	rows, err := s.dbPool.Query(ctx, query, args...)
	if err != nil {
		log.Printf("Failed to query jobs: %v", err)
		return nil, status.Error(codes.Internal, "failed to query jobs: "+err.Error())
	}
	defer rows.Close()

	// STEP 44: Iterate through results
	// Create a slice to hold jobs: jobs := []*schedulerv1.JobStatus{}
	// Use for rows.Next() loop
	// Scan each row into variables (same fields as GetJobByID)
	// Convert each job to JobStatus using convertJobToJobStatus()
	// Append to jobs slice
	// Handle scan errors: log and continue (don't break)
	// Example:
	//   jobs := []*schedulerv1.JobStatus{}
	//   for rows.Next() {
	//       var job db.Job
	//       var nextRunAt *time.Time
	//       err := rows.Scan(&job.TaskID, &job.Type, &job.Priority, &job.PayloadJSON, &job.Status, &job.Attempts, &job.MaxAttempts, &nextRunAt, &job.CreatedAt, &job.UpdatedAt)
	//       if err != nil {
	//           log.Printf("Error scanning job row: %v", err)
	//           continue
	//       }
	//       job.NextRunAt = nextRunAt
	//       jobStatus, err := convertJobToJobStatus(&job)
	//       if err != nil {
	//           log.Printf("Error converting job: %v", err)
	//           continue
	//       }
	//       jobs = append(jobs, jobStatus)
	//   }

	// TODO: Iterate through results
	jobs := []*schedulerv1.JobStatus{}
	for rows.Next() {
		var job db.Job
		var nextRunAt *time.Time
		err := rows.Scan(&job.TaskID, &job.Type, &job.Priority, &job.PayloadJSON, &job.Status, &job.Attempts, &job.MaxAttempts, &nextRunAt, &job.CreatedAt, &job.UpdatedAt)
		if err != nil {
			log.Printf("Error scanning job row: %v", err)
			continue
		}
		job.NextRunAt = nextRunAt
		jobStatus, err := convertJobToJobStatus(&job)
		if err != nil {
			log.Printf("Error converting job: %v", err)
			continue
		}
		jobs = append(jobs, jobStatus)
	}

	// STEP 45: Check for iteration errors
	// After the loop, check rows.Err()
	// If error, return status.Error(codes.Internal, "error iterating jobs: "+err.Error())
	// Example:
	//   if err := rows.Err(); err != nil {
	//       log.Printf("Error iterating job rows: %v", err)
	//       return nil, status.Error(codes.Internal, "error iterating jobs: "+err.Error())
	//   }

	// TODO: Check for iteration errors
	if err := rows.Err(); err != nil {
		log.Printf("Error iterating job rows: %v", err)
		return nil, status.Error(codes.Internal, "error iterating jobs: "+err.Error())
	}

	// STEP 46: Generate next_page_token
	// If len(jobs) == pageSize, there might be more results
	// Generate next page token: strconv.Itoa(offset + pageSize)
	// If len(jobs) < pageSize, next_page_token should be empty (no more pages)
	// Example:
	//   nextPageToken := ""
	//   if len(jobs) == pageSize {
	//       nextPageToken = strconv.Itoa(offset + pageSize)
	//   }

	// TODO: Generate next_page_token
	nextPageToken := ""
	if len(jobs) == pageSize {
		nextPageToken = strconv.Itoa(offset + pageSize)
	}

	// STEP 47: Return response
	// Return ListJobsResponse with jobs and next_page_token
	// Example:
	//   return &schedulerv1.ListJobsResponse{
	//       Jobs:          jobs,
	//       NextPageToken: nextPageToken,
	//   }, nil

	// TODO: Return response
	return &schedulerv1.ListJobsResponse{
		Jobs:          jobs,
		NextPageToken: nextPageToken,
	}, nil
}

// ============================================================================
// ADDITIONAL HELPER FUNCTION FOR ListJobs
// ============================================================================

// jobStateToStatusString converts protobuf JobState enum to database status string
// This is the reverse of statusStringToProto
// Function signature: func jobStateToStatusString(state schedulerv1.JobState) string
func jobStateToStatusString(state schedulerv1.JobState) string {
	// STEP 48: Implement reverse status conversion
	// Use a switch statement to map protobuf enums to database strings:
	//   - schedulerv1.JobState_JOB_STATE_QUEUED -> "queued"
	//   - schedulerv1.JobState_JOB_STATE_RUNNING -> "running"
	//   - schedulerv1.JobState_JOB_STATE_SUCCEEDED -> "succeeded"
	//   - schedulerv1.JobState_JOB_STATE_FAILED -> "failed"
	//   - schedulerv1.JobState_JOB_STATE_DEADLETTER -> "deadletter"
	//   - default -> "" (empty string means no filter)
	// Example:
	//   switch state {
	//   case schedulerv1.JobState_JOB_STATE_QUEUED:
	//       return "queued"
	//   ... etc
	//   default:
	//       return ""
	//   }

	// TODO: Implement reverse status conversion
	switch state {
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
	default:
		return ""
	}
}
