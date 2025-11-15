# Implementation Guide - Go gRPC Task Scheduler

## ⚠️ IMPORTANT: Instructions for AI Assistants (Cursor, etc.)

**DO NOT IMPLEMENT CODE FOR THE USER. DO NOT WRITE CODE. DO NOT USE CODE EDITING TOOLS.**

This guide is for **LEARNING PURPOSES**. The user wants to:
- **Code everything themselves** to learn Go
- **Receive guidance, tips, and explanations** only
- **Get help understanding concepts**, not getting code written

### What You SHOULD Do:
✅ Explain Go concepts and patterns  
✅ Answer questions about how things work  
✅ Provide tips and best practices  
✅ Help debug errors (explain what's wrong, don't fix it)  
✅ Clarify documentation or examples  
✅ Suggest approaches without writing code  

### What You MUST NOT Do:
❌ Write any code files  
❌ Use `write`, `search_replace`, or any code editing tools  
❌ Provide complete code implementations  
❌ Fix code errors by editing files  
❌ Create new files or modify existing ones  

**If the user asks you to implement something, remind them this is a learning exercise and guide them with explanations instead.**

---

## For the User: How to Use This Guide

This guide will help you implement the remaining features while learning Go. Each section includes:
- **What to implement**
- **Step-by-step instructions**
- **Go tips and best practices**
- **Code structure hints**
- **Testing suggestions**

**Remember:** Code it yourself! Use this guide for direction, not for copy-pasting. Learning by doing is the best way to master Go.

---

## Table of Contents

1. [Redis Queue Integration](#1-redis-queue-integration)
2. [GetJob Implementation](#2-getjob-implementation)
3. [WatchJob Implementation](#3-watchjob-implementation)
4. [CancelJob Implementation](#4-canceljob-implementation)
5. [ListJobs Implementation](#5-listjobs-implementation)
6. [Worker Implementation](#6-worker-implementation)
7. [Metrics & Observability](#7-metrics--observability)

---

## 1. Redis Queue Integration

### What to Implement
Add Redis client to push jobs to priority queues after database insert.

### Step-by-Step

#### 1.1 Install Redis Client
```bash
go get github.com/redis/go-redis/v9
```

#### 1.2 Create `internal/queue/queue.go`
- Create a `Queue` struct with Redis client
- Add `NewQueue()` function to initialize Redis connection
- Use environment variables: `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD`, `REDIS_DB`

**Go Tips:**
- Use `redis.NewClient()` with `redis.Options{}`
- Test connection with `Ping(ctx)`
- Handle errors properly (return errors, don't panic)

#### 1.3 Add PushJob Method
Create `PushJob(ctx, jobID, priority)` method that:
- Converts priority to queue name: `q:critical`, `q:high`, `q:default`, `q:low`
- Creates JSON payload: `{"task_id": "...", "type": "...", "priority": "..."}`
- Uses `LPUSH` to add to queue: `client.LPush(ctx, queueName, payload)`

**Go Tips:**
- Use `encoding/json.Marshal()` to create JSON
- Use `context.Context` for all Redis operations (enables cancellation)
- Handle Redis errors: `if err != nil { return err }`

#### 1.4 Update Server Struct
- Add `queue *queue.Queue` field to `Server` struct
- Update `NewServer()` to accept queue parameter
- Update `main.go` to initialize queue and pass to server

#### 1.5 Update SubmitJob
After successful database insert, call:
```go
err := s.queue.PushJob(ctx, jobID, priorityStr)
if err != nil {
    // Log error but don't fail the request (job is in DB)
    // Consider: retry mechanism or dead letter queue
}
```

**Go Tips:**
- Use `log.Printf()` or structured logging for errors
- Consider if queue failure should fail the request (probably not - job is already in DB)

### Testing
```bash
# Start Redis
make dev

# Test with grpcurl
grpcurl -plaintext -d '{"job": {"type": "noop", "priority": "PRIORITY_HIGH"}}' \
  localhost:8081 scheduler.v1.SchedulerService.SubmitJob

# Check Redis
docker exec -it redis redis-cli
> LLEN q:high
> LRANGE q:high 0 -1
```

---

## 2. GetJob Implementation

### What to Implement
Query database for job by ID and convert to `JobStatus` protobuf message.

### Step-by-Step

#### 2.1 Use Existing Database Function
You already have `db.GetJobByID()` - use it!

#### 2.2 Convert Database Job to Protobuf JobStatus
The protobuf `JobStatus` has these fields:
- `job_id` (string)
- `state` (JobState enum)
- `attempts` (int32)
- `last_error` (string)
- `created_at`, `updated_at`, `started_at`, `finished_at` (timestamps)

**Go Tips:**
- Convert `uuid.UUID` to string: `jobID.String()`
- Convert string status to enum: `schedulerv1.JobState_JOB_STATE_QUEUED`
- Use `timestamppb.New(time.Time)` to convert Go time to protobuf timestamp
- Handle nil timestamps: check `if job.StartedAt != nil { ... }`

#### 2.3 Handle Not Found
If `db.GetJobByID()` returns `nil`, return gRPC error:
```go
return nil, status.Error(codes.NotFound, "job not found")
```

**Go Tips:**
- Use `google.golang.org/grpc/status` for gRPC errors
- `codes.NotFound` is the appropriate error code

#### 2.4 Map Status String to Enum
Convert database status string to protobuf enum:
```go
var state schedulerv1.JobState
switch dbJob.Status {
case "queued":
    state = schedulerv1.JobState_JOB_STATE_QUEUED
case "running":
    state = schedulerv1.JobState_JOB_STATE_RUNNING
// ... etc
}
```

**Go Tips:**
- Use `switch` statement (more idiomatic than if/else chain)
- Handle unknown status gracefully (maybe default to UNSPECIFIED)

### Testing
```bash
# Submit a job first
grpcurl -plaintext -d '{"job": {"type": "noop", "priority": "PRIORITY_DEFAULT"}}' \
  localhost:8081 scheduler.v1.SchedulerService.SubmitJob

# Get the job (use job_id from response)
grpcurl -plaintext -d '{"job_id": "your-job-id-here"}' \
  localhost:8081 scheduler.v1.SchedulerService.GetJob
```

---

## 3. WatchJob Implementation

### What to Implement
Server streaming that polls database and sends updates when job status changes.

### Step-by-Step

#### 3.1 Understand Server Streaming
- Function signature: `WatchJob(req, stream) error`
- Use `stream.Send()` to send `JobEvent` messages
- Use `stream.Context()` for cancellation

**Go Tips:**
- Server streaming = you send multiple responses
- Client can cancel with context
- Check `stream.Context().Done()` to detect cancellation

#### 3.2 Polling Loop
```go
ticker := time.NewTicker(1 * time.Second) // Poll every second
defer ticker.Stop()

lastStatus := ""
for {
    select {
    case <-stream.Context().Done():
        return nil // Client cancelled
    case <-ticker.C:
        // Poll database
        // If status changed, send event
    }
}
```

**Go Tips:**
- Use `time.NewTicker()` for periodic polling
- Always `defer ticker.Stop()` to clean up
- Use `select` to handle multiple channels (context + ticker)

#### 3.3 Send Events Only on Change
- Store last known status
- Only send `JobEvent` if status changed
- Include timestamp in event

**Go Tips:**
- Use `timestamppb.Now()` for current time
- Compare status before sending (avoid spam)

#### 3.4 Handle Job Completion
- If job is `SUCCEEDED`, `FAILED`, or `DEADLETTER`, send final event and return
- Don't poll forever!

**Go Tips:**
- Use `break` or `return` to exit loop
- Send final event before returning

### Testing
```bash
# Start watching (will stream events)
grpcurl -plaintext -d '{"job_id": "your-job-id"}' \
  localhost:8081 scheduler.v1.SchedulerService.WatchJob
```

---

## 4. CancelJob Implementation

### What to Implement
Update job status to cancelled in database and remove from Redis queue if queued.

### Step-by-Step

#### 4.1 Check Job Exists
Use `db.GetJobByID()` to verify job exists.

#### 4.2 Update Database Status
Create `db.CancelJob(ctx, pool, jobID)` function:
```sql
UPDATE tasks 
SET status = 'cancelled', updated_at = now() 
WHERE task_id = $1 AND status IN ('queued', 'running')
RETURNING task_id
```

**Go Tips:**
- Use `UPDATE ... WHERE` with conditions
- Check if row was updated: `rowsAffected` or check if RETURNING returns a row
- Only cancel if status is `queued` or `running` (can't cancel completed jobs)

#### 4.3 Remove from Redis Queue
If job was queued, remove from Redis:
- Check which queue it's in (based on priority)
- Use `LREM` to remove: `client.LRem(ctx, queueName, 0, jobJSON)`
- Or use `ZREM` if in leases ZSET

**Go Tips:**
- `LREM queueName 0 value` removes all matching values
- Handle case where job is already running (maybe in leases ZSET)

#### 4.4 Return Response
Return `CancelJobResponse{Cancelled: true}` if successful.

**Go Tips:**
- Return `false` if job doesn't exist or already completed
- Use appropriate gRPC error codes

### Testing
```bash
# Submit job
grpcurl -plaintext -d '{"job": {...}}' localhost:8081 scheduler.v1.SchedulerService.SubmitJob

# Cancel it
grpcurl -plaintext -d '{"job_id": "..."}' localhost:8081 scheduler.v1.SchedulerService.CancelJob
```

---

## 5. ListJobs Implementation

### What to Implement
Query database with filters (state, priority, type) and pagination.

### Step-by-Step

#### 5.1 Build Dynamic Query
Start with base query, add `WHERE` clauses based on filters:

```go
query := "SELECT ... FROM tasks WHERE 1=1"
args := []interface{}{}
argPos := 1

if req.StateFilter != schedulerv1.JobState_JOB_STATE_UNSPECIFIED {
    query += fmt.Sprintf(" AND status = $%d", argPos)
    args = append(args, stateToString(req.StateFilter))
    argPos++
}
```

**Go Tips:**
- Use `WHERE 1=1` trick to make adding conditions easier
- Build slice of arguments: `args := []interface{}{}`
- Use `argPos` counter for parameter numbers
- Use `append()` to add to slice

#### 5.2 Handle Pagination
- `page_size`: limit results (default: 50, max: 1000)
- `page_token`: offset for next page (could be UUID or number)

**Go Tips:**
- Use `LIMIT` and `OFFSET` in SQL
- Parse `page_token` (maybe base64 encoded UUID?)
- Generate `next_page_token` for response

#### 5.3 Convert Results
- Query returns multiple rows
- Use `rows.Next()` in a loop
- Convert each row to `JobStatus` protobuf

**Go Tips:**
```go
rows, err := pool.Query(ctx, query, args...)
if err != nil { return nil, err }
defer rows.Close()

var jobs []*schedulerv1.JobStatus
for rows.Next() {
    // Scan row
    // Convert to JobStatus
    jobs = append(jobs, jobStatus)
}
```

#### 5.4 Return Response
```go
return &schedulerv1.ListJobsResponse{
    Jobs: jobs,
    NextPageToken: nextToken,
}, nil
```

### Testing
```bash
# List all jobs
grpcurl -plaintext -d '{}' localhost:8081 scheduler.v1.SchedulerService.ListJobs

# Filter by state
grpcurl -plaintext -d '{"state_filter": "JOB_STATE_QUEUED"}' \
  localhost:8081 scheduler.v1.SchedulerService.ListJobs
```

---

## 6. Worker Implementation

### What to Implement
Consumer that pops jobs from Redis queues and executes them.

### Step-by-Step

#### 6.1 Create `cmd/worker/main.go`
Similar structure to `cmd/api/main.go`:
- Initialize Redis and DB connections
- Create worker instance
- Start worker loop

#### 6.2 Create `internal/worker/worker.go`
- `Worker` struct with: `db`, `queue`, `handlers` map
- `NewWorker()` constructor
- `Start()` method with main loop

#### 6.3 Pop from Redis
Use Lua script or `BRPOP` to atomically pop from priority queues:
```go
// Try critical, high, default, low in order
result, err := client.BRPop(ctx, 1*time.Second, 
    "q:critical", "q:high", "q:default", "q:low").Result()
```

**Go Tips:**
- `BRPOP` blocks until item available (or timeout)
- Returns queue name and value
- Use `time.Second` for timeout

#### 6.4 Update Database Status
When job is claimed:
```sql
UPDATE tasks 
SET status = 'running', attempts = attempts + 1, updated_at = now()
WHERE task_id = $1 AND status = 'queued'
RETURNING task_id
```

**Go Tips:**
- Use transaction for atomicity
- Only update if status is still 'queued' (prevents double-processing)
- Increment attempts counter

#### 6.5 Execute Handler
- Parse job type from payload
- Look up handler in map: `handlers[jobType]`
- Call handler with context and payload

**Go Tips:**
- Use `map[string]HandlerFunc` for handlers
- Handler signature: `func(ctx context.Context, payload []byte) error`
- Use `context.WithTimeout()` for handler timeout

#### 6.6 Update Status on Completion
- Success: `UPDATE tasks SET status = 'succeeded' ...`
- Failure: `UPDATE tasks SET status = 'failed'` or `'retry'`
- Insert into `task_attempts` table

**Go Tips:**
- Use transactions for multiple updates
- Calculate `next_run_at` for retries (exponential backoff)
- Check `attempts >= max_attempts` for DLQ

### Testing
```bash
# Start worker
go run ./cmd/worker

# Submit jobs and watch them get processed
```

---

## 7. Metrics & Observability

### What to Implement
Prometheus metrics for monitoring.

### Step-by-Step

#### 7.1 Install Prometheus Client
```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
```

#### 7.2 Create Metrics
In `internal/metrics/metrics.go`:
```go
var (
    JobsSubmitted = prometheus.NewCounterVec(...)
    JobsCompleted = prometheus.NewCounterVec(...)
    QueueDepth = prometheus.NewGaugeVec(...)
    JobDuration = prometheus.NewHistogramVec(...)
)
```

**Go Tips:**
- Use `prometheus.NewCounterVec` for counts
- Use `prometheus.NewGaugeVec` for current values
- Use `prometheus.NewHistogramVec` for durations
- Register with `prometheus.MustRegister()`

#### 7.3 Expose HTTP Endpoint
In `main.go`:
```go
http.Handle("/metrics", promhttp.Handler())
go http.ListenAndServe(":2112", nil)
```

**Go Tips:**
- Run metrics server in goroutine
- Use standard `/metrics` path

#### 7.4 Instrument Code
Add metric increments:
- `JobsSubmitted.Inc()` in SubmitJob
- `JobsCompleted.WithLabelValues("success").Inc()` in worker
- `QueueDepth.WithLabelValues("high").Set(depth)` when checking queues

---

## Go Best Practices & Tips

### Error Handling
```go
// ✅ Good: Return errors, don't panic
result, err := doSomething()
if err != nil {
    return nil, fmt.Errorf("context: %w", err) // Wrap error
}

// ❌ Bad: Ignore errors
result, _ := doSomething() // Don't do this!
```

### Context Usage
```go
// Always accept context.Context as first parameter
func DoSomething(ctx context.Context, ...) error {
    // Check for cancellation
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        // Continue
    }
}
```

### Defer for Cleanup
```go
rows, err := pool.Query(ctx, query)
if err != nil {
    return err
}
defer rows.Close() // Always close resources
```

### Type Conversions
```go
// UUID to string
uuidStr := jobID.String()

// String to UUID
uuid, err := uuid.Parse(uuidStr)

// Time to protobuf timestamp
ts := timestamppb.New(time.Now())
```

### Working with Slices
```go
// Append to slice
items := []string{}
items = append(items, "new item")

// Iterate
for i, item := range items {
    // i is index, item is value
}

// Just values
for _, item := range items {
    // _ ignores index
}
```

### Working with Maps
```go
// Check if key exists
if handler, ok := handlers[jobType]; ok {
    // handler exists
} else {
    // key doesn't exist
}
```

### JSON Handling
```go
// Marshal (Go struct → JSON)
jsonBytes, err := json.Marshal(data)

// Unmarshal (JSON → Go struct)
err := json.Unmarshal(jsonBytes, &data)
```

---

## Testing Your Implementation

### Manual Testing with grpcurl
```bash
# Submit job
grpcurl -plaintext -d '{"job": {"type": "noop", "priority": "PRIORITY_HIGH"}}' \
  localhost:8081 scheduler.v1.SchedulerService.SubmitJob

# Get job status
grpcurl -plaintext -d '{"job_id": "..."}' \
  localhost:8081 scheduler.v1.SchedulerService.GetJob

# List jobs
grpcurl -plaintext -d '{}' \
  localhost:8081 scheduler.v1.SchedulerService.ListJobs
```

### Check Database
```bash
docker exec -it cockroachdb ./cockroach sql --insecure
> USE scheduler;
> SELECT * FROM tasks;
```

### Check Redis
```bash
docker exec -it redis redis-cli
> LLEN q:high
> LRANGE q:high 0 -1
```

---

## Common Go Patterns You'll Use

### 1. Constructor Pattern
```go
func NewThing(deps ...) *Thing {
    return &Thing{
        deps: deps,
    }
}
```

### 2. Interface for Testability
```go
type Queue interface {
    PushJob(ctx context.Context, jobID uuid.UUID, priority string) error
}
```

### 3. Options Pattern (Advanced)
```go
type Config struct {
    MaxConns int
    Timeout  time.Duration
}

func NewPool(ctx context.Context, opts ...Option) (*Pool, error) {
    cfg := &Config{MaxConns: 25} // defaults
    for _, opt := range opts {
        opt(cfg)
    }
    // ...
}
```

---

## Debugging Tips

1. **Use `log.Printf()`** for debugging (remove before production)
2. **Check error messages** - Go errors are descriptive
3. **Use `fmt.Printf("%+v", obj)`** to print structs
4. **Run `go vet ./...`** to catch common mistakes
5. **Use `go test -v`** for verbose test output

---

## Next Steps

1. Start with Redis integration (simplest)
2. Then implement GetJob (uses existing DB function)
3. Move to WatchJob (learns streaming)
4. Implement CancelJob and ListJobs
5. Finally, build the worker (most complex)

Take your time, test each piece, and don't hesitate to refer back to existing code patterns!

Good luck! 🚀

