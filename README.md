# gRPC Task Scheduler

A distributed, production-ready task scheduling system built with Go, gRPC, CockroachDB, and Redis. This system provides reliable job scheduling, execution, and monitoring with support for priority queues, retries, dead-letter queues, and comprehensive observability.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Features](#features)
- [Technology Stack](#technology-stack)
- [Getting Started](#getting-started)
- [API Documentation](#api-documentation)
- [Configuration](#configuration)
- [Development](#development)
- [Testing](#testing)
- [Monitoring & Observability](#monitoring--observability)
- [Deployment](#deployment)
- [Project Structure](#project-structure)

## Overview

The gRPC Task Scheduler is a distributed job scheduling system that allows you to:

- **Submit jobs** via a gRPC API with different priorities (CRITICAL, HIGH, DEFAULT, LOW)
- **Process jobs** asynchronously using worker pools
- **Monitor job status** in real-time with streaming updates
- **Handle failures** gracefully with automatic retries and exponential backoff
- **Track metrics** with Prometheus and visualize with Grafana
- **Scale horizontally** by running multiple worker instances

The system is designed for reliability, with features like:
- Idempotent job submission
- Job state persistence in CockroachDB
- Priority-based queuing in Redis
- Automatic retry with exponential backoff
- Dead-letter queue for permanently failed jobs
- Graceful shutdown handling

## Architecture

```
┌─────────────┐         ┌──────────────┐         ┌─────────────┐
│   Client    │─────────▶│  API Server │────────▶│ CockroachDB │
│  (gRPC)     │         │  (gRPC)      │         │  (State)    │
└─────────────┘         └──────────────┘         └─────────────┘
                               │                         │
                               │                         │
                               ▼                         │
                        ┌─────────────┐                 │
                        │    Redis    │◀────────────────┘
                        │  (Queues)   │
                        └─────────────┘
                               │
                               │
                               ▼
                        ┌─────────────┐
                        │   Worker    │
                        │  (Process)  │
                        └─────────────┘
                               │
                               ▼
                        ┌─────────────┐
                        │ Prometheus  │
                        │  (Metrics)  │
                        └─────────────┘
```

### Components

1. **API Server** (`cmd/api`): gRPC server that handles job submission, status queries, cancellation, and listing
2. **Worker** (`cmd/worker`): Background process that consumes jobs from Redis queues and executes them
3. **CockroachDB**: Persistent storage for job state, attempts, and metadata
4. **Redis**: Priority-based job queues and processing queue management
5. **Prometheus**: Metrics collection and storage
6. **Grafana**: Metrics visualization and dashboards

### Job Lifecycle

1. **Submission**: Client submits job via gRPC → API validates and stores in DB → Job pushed to Redis priority queue
2. **Processing**: Worker claims job from Redis → Moves to processing queue → Executes handler → Updates state
3. **Completion**: On success → Mark as succeeded; On failure → Retry or move to DLQ
4. **Retry**: Retry pump periodically checks for jobs ready to retry → Requeues them
5. **Recovery**: Reaper process recovers stuck jobs that have been in processing too long

## Features

### Core Features

- ✅ **Priority-based Queuing**: Four priority levels (CRITICAL, HIGH, DEFAULT, LOW)
- ✅ **Job Types**: Support for multiple job handlers (noop, http_call, db_tx)
- ✅ **Idempotency**: Duplicate job submissions return existing job
- ✅ **Retry Logic**: Automatic retries with exponential backoff
- ✅ **Dead Letter Queue**: Failed jobs after max attempts moved to DLQ
- ✅ **Job Cancellation**: Cancel queued or running jobs
- ✅ **Real-time Monitoring**: Stream job status updates via gRPC
- ✅ **Job Listing**: Query jobs with filters (state, priority, type) and pagination

### Observability

- ✅ **Prometheus Metrics**: Comprehensive metrics for jobs, queues, and performance
- ✅ **Health Checks**: `/health` (liveness) and `/ready` (readiness) endpoints
- ✅ **Structured Logging**: Detailed logs for debugging and auditing
- ✅ **Grafana Dashboards**: Pre-configured dashboards for visualization

### Reliability

- ✅ **Graceful Shutdown**: In-flight jobs complete before shutdown
- ✅ **Connection Pooling**: Efficient database and Redis connection management
- ✅ **Error Handling**: Comprehensive error handling with proper gRPC status codes
- ✅ **Stuck Job Recovery**: Automatic recovery of jobs stuck in processing

## Technology Stack

- **Language**: Go 1.24+
- **gRPC**: Protocol Buffers for API definition
- **Database**: CockroachDB (PostgreSQL-compatible)
- **Queue**: Redis 7
- **Metrics**: Prometheus
- **Visualization**: Grafana
- **Testing**: testify, testutil

## Getting Started

### Prerequisites

- Go 1.24 or later
- Docker and Docker Compose
- Make (optional, for convenience commands)

### Quick Start

1. **Clone the repository**:
   ```bash
   git clone <repository-url>
   cd go-gRPC-task-scheduler
   ```

2. **Start infrastructure services**:
   ```bash
   make dev
   # Or manually:
   cd deploy && docker compose up -d
   ```

3. **Initialize database schema**:
   ```bash
   make setup
   # Or manually:
   ./scripts/setup.sh
   ```

4. **Start the API server** (in one terminal):
   ```bash
   make api
   # Or manually:
   go run ./cmd/api
   ```

5. **Start the worker** (in another terminal):
   ```bash
   make worker
   # Or manually:
   go run ./cmd/worker
   ```

6. **Verify services are running**:
   - API Server: `grpcurl -plaintext localhost:8081 list`
   - Metrics: `curl http://localhost:2112/metrics`
   - Health: `curl http://localhost:2112/health`
   - CockroachDB UI: http://localhost:8080
   - Grafana: http://localhost:3000 (admin/admin)
   - Prometheus: http://localhost:9090

### Example: Submit a Job

Using `grpcurl`:

```bash
grpcurl -plaintext -d '{
  "job": {
    "type": "noop",
    "priority": "PRIORITY_HIGH",
    "max_attempts": 3
  }
}' localhost:8081 scheduler.v1.SchedulerService/SubmitJob
```

Using a Go client:

```go
conn, _ := grpc.Dial("localhost:8081", grpc.WithInsecure())
client := schedulerv1.NewSchedulerServiceClient(conn)

resp, _ := client.SubmitJob(ctx, &schedulerv1.SubmitJobRequest{
    Job: &schedulerv1.Job{
        Type:     "noop",
        Priority: schedulerv1.Priority_PRIORITY_HIGH,
    },
})
```

## API Documentation

### gRPC Service: `SchedulerService`

#### SubmitJob

Submit a new job for processing.

**Request**:
```protobuf
message SubmitJobRequest {
    Job job = 1;
}

message Job {
    string job_id = 1;              // Optional: UUID for idempotency
    string type = 2;                // Required: "noop", "http_call", "db_tx"
    Priority priority = 3;           // Required: CRITICAL, HIGH, DEFAULT, LOW
    string payload_json = 4;        // Optional: JSON payload
    int32 max_attempts = 6;         // Optional: default 3
}
```

**Response**:
```protobuf
message SubmitJobResponse {
    string job_id = 1;  // UUID of the created job
}
```

#### GetJob

Get the current status of a job.

**Request**:
```protobuf
message GetJobRequest {
    string job_id = 1;  // UUID of the job
}
```

**Response**:
```protobuf
message JobStatus {
    string job_id = 1;
    JobState state = 2;  // QUEUED, RUNNING, SUCCEEDED, FAILED, DEADLETTER
    int32 attempts = 3;
    string last_error = 4;
    google.protobuf.Timestamp created_at = 5;
    google.protobuf.Timestamp updated_at = 6;
    // ...
}
```

#### WatchJob

Stream real-time job status updates (server streaming).

**Request**:
```protobuf
message WatchJobRequest {
    string job_id = 1;
}
```

**Response**: Stream of `JobEvent` messages

#### CancelJob

Cancel a queued or running job.

**Request**:
```protobuf
message CancelJobRequest {
    string job_id = 1;
}
```

**Response**:
```protobuf
message CancelJobResponse {
    bool cancelled = 1;
}
```

#### ListJobs

List jobs with optional filters and pagination.

**Request**:
```protobuf
message ListJobsRequest {
    JobState state_filter = 1;
    Priority priority_filter = 2;
    string type_filter = 3;
    int32 page_size = 4;
    string page_token = 5;
}
```

**Response**:
```protobuf
message ListJobsResponse {
    repeated JobStatus jobs = 1;
    string next_page_token = 2;
}
```

## Configuration

### Environment Variables

Create a `.env` file or set environment variables:

```bash
# API Server
GRPC_PORT=8081
METRICS_PORT=2112

# Worker
WORKER_POOL_SIZE=10
METRICS_PORT=2113

# Database
COCKROACHDB_HOST=localhost
COCKROACHDB_PORT=26257
COCKROACHDB_USER=root
COCKROACHDB_PASSWORD=
COCKROACHDB_DATABASE=scheduler
COCKROACHDB_SSLMODE=disable

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
```

### Docker Compose Services

The `deploy/docker-compose.yml` includes:

- **CockroachDB**: Port 26257 (SQL), 8080 (Admin UI)
- **Redis**: Port 6379
- **Prometheus**: Port 9090
- **Grafana**: Port 3000 (admin/admin)

## Development

### Project Structure

```
.
├── cmd/
│   ├── api/          # gRPC API server
│   └── worker/       # Worker process
├── internal/
│   ├── db/           # Database operations
│   ├── redis/        # Redis queue operations
│   ├── worker/       # Worker logic
│   ├── metrics/      # Prometheus metrics
│   ├── testutil/     # Test utilities
│   └── utils/        # Utility functions
├── proto/            # Protocol Buffer definitions
├── tests/            # Test suite
├── deploy/           # Docker Compose and configs
└── scripts/          # Setup and teardown scripts
```

### Make Commands

```bash
make dev          # Start infrastructure (Redis, DB, Prometheus, Grafana)
make api          # Run API server
make worker       # Run worker
make setup        # Initialize database schema
make teardown     # Stop all services
make clean        # Stop and remove volumes
make migrate      # Run database migrations
make status       # Check service status
make logs         # View service logs
```

### Building

```bash
# Build API server
go build -o bin/api ./cmd/api

# Build worker
go build -o bin/worker ./cmd/worker
```

## Testing

### Running Tests

```bash
# Run all tests
go test ./tests/... -v

# Run specific test suite
go test ./tests/... -run TestGetJobByID -v

# Run with coverage
go test ./tests/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Test Structure

- **Unit Tests**: Test individual functions in isolation
- **Integration Tests**: Test full workflows (API → DB → Redis → Worker)
- **Test Utilities**: `internal/testutil` provides setup/cleanup helpers

### Test Coverage

The test suite includes:

- ✅ Database operations (CRUD, idempotency)
- ✅ Redis queue operations (push, pop, concurrency)
- ✅ Worker processing (job execution, retries, DLQ)
- ✅ API validation and error handling
- ✅ End-to-end job lifecycle

## Monitoring & Observability

### Prometheus Metrics

The system exposes comprehensive metrics:

**API Metrics**:
- `jobs_submitted_total{priority}` - Jobs submitted by priority
- `jobs_submitted_errors_total{error_type}` - Submission errors
- `grpc_requests_total{method,status}` - gRPC method calls
- `grpc_request_duration_seconds{method}` - API latency

**Worker Metrics**:
- `jobs_processed_total{status}` - Jobs processed (success/failed/retry)
- `job_processing_duration_seconds{type}` - Processing time
- `jobs_retried_total{priority}` - Jobs scheduled for retry
- `jobs_deadlettered_total{priority}` - Jobs moved to DLQ
- `worker_inflight_jobs` - Current in-flight jobs
- `queue_depth{priority}` - Queue depth by priority
- `processing_queue_depth` - Jobs in processing queue

**System Metrics**:
- `retry_pump_jobs_requeued_total{priority}` - Retry pump activity
- `reaper_jobs_recovered_total{priority}` - Stuck jobs recovered

### Health Checks

- **Liveness**: `GET /health` - Service is alive
- **Readiness**: `GET /ready` - Dependencies (DB, Redis) are available

### Grafana Dashboards

Access Grafana at http://localhost:3000 to view:
- Job submission rates
- Processing throughput
- Queue depths
- Error rates
- Processing latency

## Deployment

### Production Considerations

1. **Database**: Use managed CockroachDB or PostgreSQL with proper connection pooling
2. **Redis**: Use Redis Cluster or managed Redis for high availability
3. **TLS**: Enable TLS for gRPC connections
4. **Authentication**: Add gRPC authentication (mTLS, JWT, etc.)
5. **Scaling**: Run multiple worker instances for horizontal scaling
6. **Monitoring**: Set up alerting based on Prometheus metrics
7. **Logging**: Use structured logging (JSON) with log aggregation

### Docker Deployment

```bash
# Build images
docker build -t scheduler-api ./cmd/api
docker build -t scheduler-worker ./cmd/worker

# Run with docker-compose
docker compose -f deploy/docker-compose.yml up -d
```

## Job Handlers

### Supported Job Types

1. **noop**: No-operation job (useful for testing)
2. **http_call**: Make HTTP requests
   ```json
   {
     "url": "https://api.example.com/endpoint",
     "method": "POST",
     "headers": {"Authorization": "Bearer token"},
     "body": "{\"key\": \"value\"}"
   }
   ```
3. **db_tx**: Execute database transactions
   ```json
   {
     "query": "UPDATE users SET status = $1 WHERE id = $2",
     "params": ["active", "123"]
   }
   ```

### Adding Custom Handlers

Extend `internal/worker/worker.go` `executeHandler()` method:

```go
case "custom_type":
    // Parse payload
    // Execute custom logic
    // Return error on failure
```

## Troubleshooting

### Common Issues

1. **Jobs not processing**: Check worker logs, verify Redis connection
2. **Database connection errors**: Verify CockroachDB is running and accessible
3. **High queue depth**: Scale workers or investigate processing bottlenecks
4. **Jobs stuck in processing**: Check reaper logs, verify worker is running

### Debugging

- Enable verbose logging: Set log level in code
- Check metrics: `curl http://localhost:2112/metrics | grep job`
- Inspect queues: `redis-cli LLEN q:high`
- Query database: Connect to CockroachDB UI at http://localhost:8080

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Ensure all tests pass
6. Submit a pull request

## License

[Add your license here]

## Acknowledgments

Built with:
- [gRPC](https://grpc.io/)
- [CockroachDB](https://www.cockroachlabs.com/)
- [Redis](https://redis.io/)
- [Prometheus](https://prometheus.io/)
- [Grafana](https://grafana.com/)
