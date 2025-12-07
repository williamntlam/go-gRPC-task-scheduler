# Job Submission Scripts

Scripts to submit jobs to the task scheduler for testing and observing metrics in Grafana/Prometheus.

## Prerequisites

1. **Install grpcurl** (if not already installed):
   ```bash
   go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
   ```

2. **Start the services**:
   ```bash
   make dev        # Start infrastructure (DB, Redis, Prometheus, Grafana)
   make api        # Start API server (in one terminal)
   make worker     # Start worker (in another terminal)
   ```

3. **Optional: Install jq** (for pretty JSON output):
   ```bash
   sudo apt-get install jq  # Ubuntu/Debian
   brew install jq           # macOS
   ```

## Scripts

### 1. `submit-job.sh` - Submit a Single Job

Submit one job with custom parameters.

**Usage:**
```bash
./scripts/submit-job.sh [type] [priority] [max_attempts] [payload_json]
```

**Examples:**
```bash
# Simple noop job
./scripts/submit-job.sh noop high 3

# HTTP call job
./scripts/submit-job.sh http_call critical 5 '{"url":"https://example.com","method":"GET"}'

# Database transaction job
./scripts/submit-job.sh db_tx default 3 '{"query":"SELECT 1","params":[]}'
```

**Parameters:**
- `type`: `noop`, `http_call`, or `db_tx`
- `priority`: `critical`, `high`, `default`, or `low`
- `max_attempts`: Number (default: 3)
- `payload_json`: JSON string (default: `{}`)

---

### 2. `submit-jobs-bulk.sh` - Submit Many Jobs

Submit multiple jobs of the same type/priority in bulk.

**Usage:**
```bash
./scripts/submit-jobs-bulk.sh [count] [type] [priority] [delay_ms]
```

**Examples:**
```bash
# Submit 100 high-priority noop jobs with 100ms delay
./scripts/submit-jobs-bulk.sh 100 noop high 100

# Submit 50 default-priority jobs, no delay (burst)
./scripts/submit-jobs-bulk.sh 50 http_call default 0

# Submit 20 critical jobs slowly
./scripts/submit-jobs-bulk.sh 20 db_tx critical 500
```

**Parameters:**
- `count`: Number of jobs to submit
- `type`: `noop`, `http_call`, or `db_tx`
- `priority`: `critical`, `high`, `default`, or `low`
- `delay_ms`: Milliseconds between submissions (default: 100)

---

### 3. `submit-jobs-mixed.sh` - Submit Mixed Job Load

Submit a realistic mix of different job types and priorities.

**Usage:**
```bash
./scripts/submit-jobs-mixed.sh [total_count]
```

**Examples:**
```bash
# Submit 100 mixed jobs (20% critical, 30% high, 40% default, 10% low)
./scripts/submit-jobs-mixed.sh 100

# Submit 500 mixed jobs
./scripts/submit-jobs-mixed.sh 500
```

**Distribution:**
- Priorities: 20% critical, 30% high, 40% default, 10% low
- Types: 50% noop, 30% http_call, 20% db_tx

---

### 4. `load-test.sh` - Sustained Load Testing

Submit jobs at a constant rate for a specified duration.

**Usage:**
```bash
./scripts/load-test.sh [rate_per_sec] [duration_sec] [priority]
```

**Examples:**
```bash
# 10 jobs/sec for 60 seconds, high priority
./scripts/load-test.sh 10 60 high

# 50 jobs/sec for 2 minutes, default priority
./scripts/load-test.sh 50 120 default

# 5 jobs/sec for 5 minutes, critical priority
./scripts/load-test.sh 5 300 critical
```

**Parameters:**
- `rate_per_sec`: Jobs per second (default: 10)
- `duration_sec`: How long to run (default: 60)
- `priority`: `critical`, `high`, `default`, or `low`

---

### 5. `get-job.sh` - Check Job Status

Get the status of a specific job.

**Usage:**
```bash
./scripts/get-job.sh [job_id]
```

**Example:**
```bash
./scripts/get-job.sh 550e8400-e29b-41d4-a716-446655440000
```

---

## Testing Scenarios

### Scenario 1: Basic Testing
```bash
# Submit a few jobs
./scripts/submit-job.sh noop high 3
./scripts/submit-job.sh noop default 3
./scripts/submit-job.sh noop low 3

# Check Grafana: http://localhost:3000
# Watch queue depths and processing rates
```

### Scenario 2: Queue Behavior
```bash
# Submit 50 high-priority jobs
./scripts/submit-jobs-bulk.sh 50 noop high 50

# Then submit 50 low-priority jobs
./scripts/submit-jobs-bulk.sh 50 noop low 50

# Observe in Grafana: high-priority jobs should process first
```

### Scenario 3: Load Testing
```bash
# Start a sustained load
./scripts/load-test.sh 20 120 default

# Watch in Grafana:
# - Queue depth should stabilize
# - Processing rate should match submission rate
# - Worker utilization should increase
```

### Scenario 4: Mixed Workload
```bash
# Submit realistic mixed workload
./scripts/submit-jobs-mixed.sh 200

# Observe:
# - Different priorities being processed in order
# - Different job types with different durations
# - Overall system behavior
```

### Scenario 5: Burst Testing
```bash
# Submit many jobs quickly (no delay)
./scripts/submit-jobs-bulk.sh 1000 noop high 0

# Watch:
# - Queue depth spike
# - Processing catch up
# - System recovery
```

---

## Observing Metrics

### Grafana Dashboard
1. Open http://localhost:3000
2. Login: `admin` / `admin`
3. Navigate to "Task Scheduler Dashboard"
4. Watch metrics update in real-time

### Key Metrics to Watch

**Submission Metrics:**
- Jobs Submitted (Rate) - See your submission rate
- Submission Errors (Rate) - Check for errors

**Processing Metrics:**
- Jobs Processed (Rate) - Success/failure/retry rates
- Job Processing Duration (p95) - Performance

**Queue Metrics:**
- Queue Depth by Priority - Are queues backing up?
- Processing Queue Depth - In-flight jobs

**System Health:**
- Worker In-Flight Jobs - Worker utilization
- Jobs Retried (Rate) - Retry activity
- Jobs Deadlettered (Rate) - Permanent failures

### Prometheus Queries

You can also query Prometheus directly at http://localhost:9090:

```promql
# Total jobs submitted
sum(jobs_submitted_total)

# Current queue depth
sum(queue_depth)

# Processing rate
rate(jobs_processed_total[5m])

# Error rate
rate(jobs_submitted_errors_total[5m])
```

---

## Tips

1. **Start Small**: Begin with a few jobs to verify everything works
2. **Watch Dashboards**: Keep Grafana open while submitting jobs
3. **Vary Parameters**: Try different priorities, types, and rates
4. **Observe Patterns**: 
   - How queues behave under load
   - Priority ordering (critical → high → default → low)
   - Retry behavior when jobs fail
5. **Test Failures**: Submit jobs that will fail to see retry/DLQ behavior

---

## Troubleshooting

**Script fails with "connection refused":**
- Make sure API server is running: `make api`
- Check GRPC_PORT environment variable matches your server

**No metrics appearing:**
- Verify Prometheus is scraping: http://localhost:9090/targets
- Check API/Worker metrics endpoints: `curl http://localhost:2112/metrics`

**Jobs not processing:**
- Make sure worker is running: `make worker`
- Check worker logs for errors
- Verify Redis connection
