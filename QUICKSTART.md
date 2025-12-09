# Quick Start Guide

A step-by-step guide to start the task scheduler, submit jobs, and stop everything.

## Prerequisites

- Go 1.24 or later
- Docker and Docker Compose
- Make (optional, but recommended)
- `grpcurl` (for job submission scripts)
  ```bash
  go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
  ```

## Starting the System

### Step 1: Start Infrastructure (Terminal 1)

Start all infrastructure services (CockroachDB, Redis, Prometheus, Grafana):

```bash
make dev
```

Wait a few seconds for services to be ready. You should see:
- ✅ CockroachDB UI: http://localhost:8080
- ✅ Grafana: http://localhost:3000 (admin/admin)
- ✅ Prometheus: http://localhost:9090

### Step 2: Initialize Database (One-time setup)

In the same terminal or a new one:

```bash
make setup
```

This creates the database schema. **Only needed once** (or after `make clean`).

### Step 3: Start API Server (Terminal 2)

**Option A: Direct Go execution (fast iteration, recommended for development)**
```bash
make api
```

**Option B: Docker (consistent environment)**
```bash
make docker-run-api
```

Keep this terminal open (or run in background with Docker). The API server runs on port **8081**.

### Step 4: Start Worker (Terminal 3)

**Option A: Direct Go execution (fast iteration, recommended for development)**
```bash
make worker
```

**Option B: Docker (consistent environment)**
```bash
make docker-run-worker
```

Keep this terminal open (or run in background with Docker). The worker will start processing jobs from the queues.

### Step 5: Verify Everything is Running

Check service status:

```bash
make status
```

You should see all services running:
- `cockroachdb` - Database
- `redis` - Queue
- `prometheus` - Metrics
- `grafana` - Dashboards

---

## Submitting Jobs

Once everything is running, you can submit jobs from **any terminal**.

### Quick Test Jobs

Submit a few test jobs (one of each priority):

```bash
make test-jobs
```

### Submit Mixed Workload

Submit a realistic mix of job types and priorities:

```bash
make submit-mixed COUNT=200
```

### Submit Bulk Jobs

Submit many jobs of the same type:

```bash
# 100 high-priority noop jobs
make submit-bulk COUNT=100 TYPE=noop PRIORITY=high

# 50 default-priority jobs with custom delay
make submit-bulk COUNT=50 TYPE=http_call PRIORITY=default DELAY=50
```

### Submit Single Job

Submit one job with custom parameters:

```bash
# Simple noop job
make submit-job TYPE=noop PRIORITY=high

# HTTP call job
make submit-job TYPE=http_call PRIORITY=critical PAYLOAD='{"url":"https://example.com","method":"GET"}'

# Database transaction job
make submit-job TYPE=db_tx PRIORITY=default PAYLOAD='{"query":"SELECT 1","params":[]}'
```

### Load Testing

Run a sustained load test:

```bash
# 10 jobs/sec for 60 seconds, default priority
make load-test RATE=10 DURATION=60 PRIORITY=default

# 50 jobs/sec for 2 minutes, high priority
make load-test RATE=50 DURATION=120 PRIORITY=high
```

### Quick Load Test

Quick test with 100 high-priority jobs:

```bash
make quick-load
```

### Check Job Status

Get the status of a specific job:

```bash
make get-job JOB_ID=550e8400-e29b-41d4-a716-446655440000
```

---

## Monitoring

### Grafana Dashboard

1. Open http://localhost:3000
2. Login: `admin` / `admin`
3. Navigate to **"Task Scheduler Dashboard"**
4. Watch metrics update in real-time

**Key Metrics:**
- **Jobs Submitted (Rate)** - Submission throughput
- **Jobs Processed (Rate)** - Processing throughput by status
- **Queue Depth by Priority** - Current queue sizes
- **Job Processing Duration** - Performance metrics
- **Worker In-Flight Jobs** - Worker utilization
- **Error Rates** - Submission and processing errors

### Prometheus

Query metrics directly at http://localhost:9090

Example queries:
```promql
# Total jobs submitted
sum(jobs_submitted_total)

# Current queue depth
sum(queue_depth)

# Processing rate
rate(jobs_processed_total[5m])
```

### View Logs

View infrastructure logs:

```bash
make logs
```

---

## Stopping Everything

### Stop API Server and Worker

**If using direct Go execution:**
In Terminal 2 and Terminal 3, press **Ctrl+C** to stop the API server and worker.

**If using Docker:**
```bash
make docker-stop
# Or individually:
make docker-stop-api
make docker-stop-worker
```

### Stop Infrastructure

Stop all infrastructure services (keeps data):

```bash
make down
```

### Stop and Remove Everything (Clean Slate)

Stop services and remove all volumes (deletes all data):

```bash
make clean
```

**⚠️ Warning:** This will delete all data in CockroachDB, Redis, and Prometheus!

---

## Complete Example Workflow

**Using Direct Go Execution (Fast Development):**
```bash
# Terminal 1: Start infrastructure
make dev
make setup

# Terminal 2: Start API server
make api

# Terminal 3: Start worker
make worker

# Terminal 4: Submit jobs and test
make test-jobs
make submit-mixed COUNT=200
make load-test RATE=20 DURATION=60 PRIORITY=high

# Open browser: http://localhost:3000 to watch metrics

# When done: Stop everything
# Ctrl+C in Terminal 2 and 3
make down
```

**Using Docker (Consistent Environment):**
```bash
# Terminal 1: Start infrastructure
make dev
make setup

# Terminal 2: Start API server in Docker
make docker-run-api

# Terminal 3: Start worker in Docker
make docker-run-worker

# Terminal 4: Submit jobs and test
make test-jobs
make submit-mixed COUNT=200

# View logs: docker logs -f scheduler-api (or scheduler-worker)

# When done: Stop everything
make docker-stop
make down
```

---

## Troubleshooting

### "Connection refused" when submitting jobs

- Make sure API server is running: `make api`
- Check if it's listening: `curl http://localhost:2112/health`

### Jobs not processing

- Make sure worker is running: `make worker`
- Check worker logs for errors
- Verify Redis connection: `docker exec redis redis-cli ping`

### No metrics in Grafana

- Verify Prometheus is scraping: http://localhost:9090/targets
- Check API/Worker metrics: `curl http://localhost:2112/metrics`
- Restart Grafana: `docker compose restart grafana` (in `deploy/` directory)

### Services won't start

- Check if ports are already in use: `make status`
- View logs: `make logs`
- Try stopping everything first: `make down`

### Database connection errors

- Make sure CockroachDB is running: `make status`
- Wait a few seconds after `make dev` before running `make setup`
- Check CockroachDB UI: http://localhost:8080

---

## Next Steps

- Read the full [README.md](README.md) for detailed documentation
- Check [scripts/README.md](scripts/README.md) for advanced job submission options
- Explore the Grafana dashboard to understand system behavior
- Try different job types, priorities, and load patterns

---

## Quick Reference

| Command | Description |
|---------|-------------|
| `make dev` | Start infrastructure services |
| `make setup` | Initialize database (one-time) |
| `make api` | Start API server |
| `make worker` | Start worker |
| `make test-jobs` | Submit test jobs |
| `make submit-mixed COUNT=200` | Submit mixed workload |
| `make submit-bulk COUNT=100 TYPE=noop PRIORITY=high` | Submit bulk jobs |
| `make load-test RATE=10 DURATION=60 PRIORITY=default` | Run load test |
| `make status` | Check service status |
| `make logs` | View service logs |
| `make down` | Stop infrastructure |
| `make clean` | Stop and remove everything |
