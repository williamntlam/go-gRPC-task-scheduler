
# High‑Level Implementation Guide — Go gRPC Task Scheduler (Redis + Workers/Consumers)

**Updated:** 2025-11-12  
**Stack:** Go • gRPC • Redis • CockrachDB • Prometheus/Grafana • Docker

This is a concise, step‑by‑step plan to build a **producer/consumer** task scheduler with **gRPC ingestion**, **Redis priority queues**, and **Go workers**. It layers in critical **system‑design concepts**: idempotency, retries, visibility timeouts, DLQs, backpressure, rate limits, circuit breakers, fan‑out/fan‑in, observability, and rollouts.

---

## 0) Outcome (What you’ll have)
- A **gRPC API** to submit jobs and stream status.
- **Redis** queues (critical/high/default/low) + **leases ZSET** for visibility timeouts.
- **Go workers/consumers** that process tasks with bounded concurrency.
- **Retry pump** + **Dead Letter Queues**.
- **CockrachDB** as the source of truth for task state.
- **Prometheus** metrics + **Grafana** dashboard.
- Clean separation so you can scale **producers** and **consumers** independently.

---

## 1) Define gRPC Contracts (Producer/Client → Gateway)
Create a protobuf in `proto/scheduler/v1/scheduler.proto`:

- **Service**
  - `SubmitJob(Job)` → `SubmitJobResponse { job_id }`
  - `GetJob(GetJobRequest)` → `JobStatus`
  - `WatchJob(WatchJobRequest)` → `stream JobEvent` (server streaming for progress)
- **Messages**
  - `Job { job_id (optional on submit), type, priority, payload_json, idempotency_key }`
  - `JobStatus { state: QUEUED|RUNNING|SUCCEEDED|FAILED|DEADLETTER, attempts, last_error }`

> Generate Go stubs; enable interceptors for tracing/metrics. Prefer **deadline** and **retry** settings in the client (gRPC channel config).

---

## 2) Stand Up Core Infra (Docker Compose)
- **Redis 7** (default config is fine for dev)
- **CockrachDB** (holds tasks/attempts/idempotency)
- **Prometheus** (scrapes :2112 and :2113)
- **Grafana** (pre‑provision one dashboard JSON)

> Keep secrets in env files; use separate networks. Add a Makefile with `make dev`, `make api`, `make worker`, `make migrate`.

---

## 3) CockrachDB Schema (Authoritative State)
Tables:
- `tasks(task_id, type, priority, payload JSONB, status, attempts, max_attempts, next_run_at, created_at, updated_at)`
- `task_attempts(task_id, started_at, finished_at, ok, error)`
- `idempotency_keys(idempotency_key PRIMARY KEY → task_id)`

Indexes:
- `status`, `next_run_at`, and `(idempotency_key)`.

> **Idempotency**: on `SubmitJob`, upsert the key; return existing `task_id` if conflict.

---

## 4) Redis Keys & Fair Priority Pop
- Queues: `q:critical`, `q:high`, `q:default`, `q:low` (LISTs)
- Leases: `leases` (ZSET; score = lease_deadline_ms; member = task_id)
- Retries: `retry:z:{priority}` (ZSET; score = ready_at_ms)
- Dead letters: `dlq:{priority}` (LIST of JSON payloads)

**Lua script** `pop_next.lua` (atomic fair pop):
1) RPOP from `q:critical` → `q:high` → `q:default` → `q:low`.
2) If found, decode `{ task_id,... }`, compute `deadline = now + lease_ms`.
3) `ZADD leases deadline task_id` (visibility timeout lease).
4) Return the popped payload as JSON.

> Ensures single‑consumer leasing and **at‑least‑once** semantics.

---

## 5) gRPC Gateway (Producer) Flow
- Validate request; **coerce/guard** `type`, `priority`, `max_attempts`.
- **Insert task** row (`status=queued`), create mapping for `idempotency_key` if provided.
- Push **lean payload** to Redis: `{ task_id, type, priority }` via `LPUSH q:{priority}`.
- Return `{ job_id }` immediately.
- For `WatchJob`, stream updates by polling CockrachDB or subscribing to internal events.

**Concepts used**: **idempotency**, **schema validation**, **timeouts**, **auth** (optional JWT/HMAC), **rate limiting** per API key (token bucket, Redis counters).

---

## 6) Worker/Consumer Design (Go)
- **Process model**: each `worker` binary runs **N goroutines** (bounded by `WORKER_POOL`) and uses the Lua pop script for atomic claim.
- On pop:
  1) Mark `status=running`, `attempts += 1` (CockrachDB).
  2) Run handler with **context timeout = visibility timeout**.
  3) On **success**: `ZREM leases task_id`, set `status=succeeded`.
  4) On **error**: compute backoff, move to retry ZSET or DLQ, set `status` accordingly.

**Concepts used**: **bounded concurrency**, **backpressure** (do not exceed pool size), **visibility timeout**, **retry with exponential backoff + jitter**, **DLQ isolation**.

---

## 7) Retry Pump & Reaper
- **Retry Pump** (ticker every 500ms):
  - For each `retry:z:{priority}`, `ZRANGEBYSCORE` up to now → `LPUSH q:{priority}`, then `ZREMRANGEBYSCORE` to remove.
- **Reaper** (ticker every 2s):
  - `ZRangeByScore leases` for expired deadlines → requeue to original priority (lookup in DB) → `ZREM leases`.
  - Increments **reaped** metric; **do not** reset `attempts`.

**Concepts used**: **at‑least‑once delivery**, **time‑based coordination**, **eventual consistency**.

---

## 8) Core Handlers (to test the system)
Implement idempotent handlers in `internal/worker/handlers.go`:
- `noop` → immediate success.
- `flaky(n)` → fail the first *n* attempts, then succeed.
- `always_fail` → always error → DLQ after `max_attempts`.
- `sleep(ms)` → sleep > visibility to force a **reap**.
- `http_call(url)` → external I/O with **circuit breaker** + **timeouts**.
- `db_tx(unique_key)` → `INSERT ... ON CONFLICT DO NOTHING` to prove **idempotent effects**.

**Concepts used**: **idempotent side‑effects**, **circuit breakers**, **timeouts**, **retries**.

---

## 9) Observability (Metrics, Traces, Logs)
- **Prometheus** (API :2112, Worker :2113):
  - Counters: `tasks_enqueued_total`, `dequeued_total`, `succeeded_total`, `failed_total`, `retried_total`, `reaped_total`.
  - Gauges: `queue_depth{{priority}}`, `retry_depth{{priority}}`, `dlq_depth{{priority}}`, `inflight_leases`.
  - Histograms: `task_run_seconds{{type}}`, `enqueue_to_start_seconds{{priority}}`.
- **Grafana**: p95 run time, queue depth, DLQ size, throughput, inflight.
- **Tracing** (optional): OTel spans for gRPC request → worker execution; **propagate trace_id** in Redis message headers.
- **Structured logs** (zap/zerolog): include `task_id`, `trace_id`, `queue`, `handler` fields.

**Concepts used**: **SLOs**, **RED/USE metrics**, **p‑quantiles**, **golden signals**.

---

## 10) Fan‑Out / Fan‑In (Optional Upgrade)
- **Fan‑out**: one job creates *k* sub‑tasks (e.g., `video:1080p`, `video:720p`, `thumbs`), each enqueued to Redis.
- **Fan‑in aggregator**: track completion via CockrachDB rows or Redis keys (`job:{id}:done_count`). When all sub‑tasks succeed → mark parent `SUCCEEDED`. Timeouts push parent to `REVIEW` or `FAILED`.

**Concepts used**: **workflow orchestration**, **barriers**, **join/aggregation**.

---

## 11) Reliability & Safety
- **At‑least‑once** semantics by design; guard side‑effects with idempotency.
- **Backpressure**: keep a bounded worker pool; use **prefetch** limits (don’t pop if pool full).
- **Rate limits** on producer gRPC to prevent queue explosions.
- **DLQs** per priority with a **replay tool** (move back to a queue after you fix the bug/data).
- **Schema guardrails**: reject oversized payloads; validate types and enums.

---

## 12) Testing & Load
- Provide a `cmd/load` program to submit mixed jobs:
  - 80% `noop`, 10% `flaky(2)`, 5% `always_fail`, 5% `sleep(>vis)`.
- Watch: queue depth, success/failed ratios, retries, p95 runtimes.
- Kill/restart workers to validate **reaper** recovery.

**Concepts used**: **failure injection**, **chaos testing lite**, **warm/cold restarts**.

---

## 13) Deployment & Rollouts
- Containerize API + Worker.
- Use **readiness/liveness** probes.
- Configuration via env (12‑factor).
- Rolling restarts to prove **no task loss beyond at‑least‑once**.
- For Kubernetes: HPA on `queue_depth` or `run_seconds p95`.

**Concepts used**: **blue/green or rolling** deploys, **graceful shutdown** (drain leases), **config as code**.

---

## 14) Security & Multi‑Tenancy (Later)
- gRPC **mTLS** or JWT for authn; **RBAC** for management endpoints.
- **Tenant key prefixes** (`tenant:{id}:q:*`) with per‑tenant quotas.
- **Audit logs** to CockrachDB for admin actions (replay DLQ, force retry).

---

## 15) Stretch Concepts (When Ready)
- **Outbox / Inbox** patterns for exactly‑once effects to other systems.
- **Sagas** for multi‑step workflows w/ compensation.
- **Priority aging** to avoid starvation.
- **Adaptive concurrency** (auto‑tune pool from CPU/latency).

---

## 16) Minimal Repo Skeleton
```
/cmd
  /api         # gRPC server + Redis producer + metrics
  /worker      # consumer loop + handlers + retry pump + reaper + metrics
  /load        # load generator
/internal
  /config      # env → Config
  /db          # CockrachDB helpers
  /queue       # Redis client + Lua scripts
  /worker      # handlers, backoff, circuit breaker
  /metrics     # Prometheus registration
/proto/scheduler/v1
/migrations
/deploy        # docker-compose.yml, prometheus.yml, grafana/
/Makefile
```

---

## 17) Quick Bring‑Up Checklist
1. `make dev` → Redis, CockrachDB, Prometheus, Grafana up.
2. `make migrate` → apply SQL.
3. Start gateway: `make api` (gRPC on :8081, metrics :2112).
4. Start worker: `make worker` (metrics :2113).
5. Submit jobs with `grpcurl` or a client stub.
6. Watch Grafana: queue depth falls; p95 stabilizes.
7. Kill worker → confirm **reaper** requeues leases → restart → jobs complete.


# Add-On: DAG Workflows, Autoscaling, and Advanced Reliability
**Scope:** Extends the gRPC + Redis scheduler with **DAG orchestration**, **autoscaling**, and **production-grade reliability** patterns.

---

## 1) DAG Workflows (Fan‑Out / Fan‑In / Dependencies)

### 1.1 Concepts
- **DAG**: Jobs modeled as nodes with directed edges (dependencies). No cycles.
- **Node**: A task template (`type`, `priority`, `payload`, `max_attempts`, `timeout_ms`).
- **Run**: A concrete execution of a DAG (root job) with per-node runtime state.
- **States**: `PENDING → READY → RUNNING → SUCCEEDED / FAILED / CANCELED / SKIPPED`.
- **Policies**: `all_success` (default) or `any_success` for fan‑in gates; `on_fail = stop|continue|compensate`.

### 1.2 Minimal Schema
```
dag_definitions(dag_id, name, version, created_at)
dag_nodes(node_id, dag_id, name, type, priority, payload, max_attempts, timeout_ms, on_fail)
dag_edges(dag_id, from_node_id, to_node_id)

dag_runs(run_id, dag_id, status, created_at, updated_at)
dag_node_runs(node_run_id, run_id, node_id, status, attempts, started_at, finished_at, last_error)
```

### 1.3 Readiness Rule
A node becomes **READY** when all of its upstream dependencies are `SUCCEEDED` (or policy allows otherwise).

### 1.4 Dispatcher (Orchestrator) Flow
1. **Topological scan** of `dag_node_runs` for a given `run_id`.
2. For each `PENDING` node whose deps are satisfied → mark `READY`.
3. Enqueue `{ run_id, node_id, type, priority }` to `q:{priority}` (Redis).
4. When a worker finishes a node:
   - Update `dag_node_runs.status` to `SUCCEEDED` (or `FAILED`).
   - Orchestrator re-scans downstream nodes. If all terminal nodes succeed → `dag_runs.status = SUCCEEDED`.

> The orchestrator can be a separate service that wakes periodically or reacts to a message bus event (`node.done`).

### 1.5 Idempotency & Exactly‑Once Effects per Node
- Every node handler must be **idempotent**: protect side effects with unique keys (`INSERT ... ON CONFLICT DO NOTHING`), external APIs with natural keys, or a **dedupe table**.
- Store a **handler_result_hash** to short‑circuit repeats where safe.

### 1.6 Compensation / Sagas (Optional)
Add `compensation_type` for nodes that need rollback. On upstream failure with `on_fail=compensate`:
- Enqueue compensating nodes in reverse topological order.
- Track separate `compensation_runs` tables or a `mode=COMPENSATING` flag on `dag_runs`.

### 1.7 Concurrency Controls
- **Per‑DAG concurrency**: limit concurrent nodes per run (`max_parallelism`).
- **Per‑Node concurrency**: limit concurrent executions of a node template across the fleet (use Redis semaphore `ZSET` or token bucket).

### 1.8 gRPC API (Orchestrator)
- `StartDAG(StartDAGRequest)` → `StartDAGResponse { run_id }`
- `GetDAGRun(GetDAGRunRequest)` → `DAGRunStatus { overall_state, node_states[] }`
- `WatchDAGRun(WatchDAGRunRequest)` → `stream DAGEvent { node_id, status, percent }`

---

## 2) Autoscaling (HPA/KEDA + Backpressure)

### 2.1 Signals
- **Queue depth** (per priority): `LLEN q:critical|high|default|low`
- **Leases inflight**: `ZCARD leases`
- **Latency SLO**: p95 `task_run_seconds`
- **Retry pressure**: size of `retry:z:*`

### 2.2 Horizontal Pod Autoscaler (HPA)
- Export queue depth to Prometheus: `scheduler_queue_depth{priority}`.
- HPA with `external.metrics.k8s.io` via Prometheus Adapter:
  - Scale `worker` deployment by `queue_depth` (per priority) and `p95 run time`.
  - Example: target `queue_depth/default > 1000` → replicas += 1 every 30s (cooldown).

### 2.3 KEDA (Simpler)
- Use **Redis scaler** on `q:default` length and `q:critical` length.
- Define `minReplicaCount`, `maxReplicaCount`, `pollingInterval`, `cooldownPeriod`.
- Advantage: no custom adapters; uses Redis directly.

### 2.4 Adaptive Concurrency (Inside Worker)
- Dynamically adjust `WORKER_POOL` based on:
  - CPU > 85%: decrement pool; CPU < 40%: increment pool (bounded).
  - Or based on target p95 runtime and failure rate.

### 2.5 Backpressure & Load Shedding
- **Producer** rate limits: token bucket per API key/tenant.
- **Drop or degrade** low‑priority jobs under overload (aging, demotion thresholds).
- **Admission control**: reject new tasks if `queue_depth` + `retry_depth` exceed ceilings; return `429`/`RESOURCE_EXHAUSTED` gRPC status with retry‑after.

---

## 3) Advanced Reliability (Beyond At‑Least‑Once)

### 3.1 Transactional Outbox / Inbox
- When a handler emits events or writes to other systems, write to a local **outbox** table **in the same DB transaction** as state changes.
- A background relay reads outbox rows and publishes to Kafka/Redis with dedupe keys, marking rows as delivered.
- **Inbox** table on the consumer side deduplicates incoming events by `message_id`, ensuring idempotent processing.

### 3.2 Exactly‑Once Effect Semantics (Practical)
- Combine **at‑least‑once transport** with **idempotent handlers** + **dedupe tables**.
- Use `idempotency_keys(idempotency_key UNIQUE)` around the smallest side effect boundary.

### 3.3 Circuit Breakers & Bulkheads
- **Per‑handler** breaker (e.g., `http_call`): open on 50% failures / 20 requests window; half‑open probes after backoff.
- **Bulkhead pools**: isolate slow I/O handlers from CPU‑bound ones (separate goroutine pools, separate deployments).

### 3.4 Timeouts & Deadlines
- gRPC server/client deadlines; per‑handler timeouts; lease >= handler timeout + small buffer.
- Cancel contexts on shutdown; drain leases gracefully.

### 3.5 Multi‑Region / DR (Later)
- **Active/Active** with regional Redis and per‑region queues; keep **DAG orchestration regional**; replicate CockrachDB via logical replication.
- **Failover**: promote a read‑replica; re‑seed `retry:z:*` and `leases` carefully.
- **Shard by tenant** to cap blast radius.

### 3.6 Chaos & Game Days
- Fault injection flags in handlers: random 500s, timeouts, sleeps.
- Periodic drills: kill workers, pause Redis, degrade DB; verify SLO / recovery.

---

## 4) DAG Orchestrator — Pseudocode

```go
func tickOrchestrator(runID int64) {
  // 1) load all node runs
  nodes := loadNodeRuns(runID)

  // 2) find READY set
  for _, n := range nodes {
    if n.Status != PENDING { continue }
    if depsSatisfied(n, nodes) && underParallelism(runID) {
      markReady(n)
      enqueue(n) // LPUSH q:{priority} with {run_id,node_id,type}
    }
  }

  // 3) terminal evaluation
  if allTerminalsSucceeded(nodes) { markRunSucceeded(runID); return }
  if anyTerminalFailed(nodes)     { markRunFailed(runID); return }
}
```

**depsSatisfied** checks all upstream nodes `SUCCEEDED` (or policy).

---

## 5) Redis Keys for DAG State (Optional Cache)
```
dag:{run_id}:ready                # SET of node_ids
dag:{run_id}:done                 # SET of node_ids
dag:{run_id}:deps:{node_id}       # SET of upstream node_ids
dag:{run_id}:pending_count        # COUNTER
```
> Source of truth remains CockrachDB; Redis is just a speed cache.

---

## 6) gRPC Additions (DAG Control Plane)
```
service Orchestrator {
  rpc StartDAG(StartDAGRequest) returns (StartDAGResponse);
  rpc GetDAGRun(GetDAGRunRequest) returns (DAGRunStatus);
  rpc WatchDAGRun(WatchDAGRunRequest) returns (stream DAGEvent);
  rpc CancelDAG(CancelDAGRequest) returns (CancelDAGResponse);
}
```
- **Cancel** sets remaining `PENDING/READY/RUNNING` to `CANCELED` (+ stop scheduling).

---

## 7) Prometheus SLOs & Alerts (Examples)

### 7.1 SLO Targets
- Availability (API): 99.9%
- p95 handler runtime: < 500ms (default), < 2s (I/O)
- Error budget: 0.1% failed tasks per day

### 7.2 Alerts
- **Queue Saturation**: `scheduler_queue_depth > 10k for 5m`
- **High Failure Ratio**: `rate(failed_total[10m]) / (rate(failed_total[10m]) + rate(succeeded_total[10m])) > 0.2`
- **Retry Storm**: `increase(retried_total[5m]) > 5000`
- **Slow p95**: `histogram_quantile(0.95, sum(rate(task_run_seconds_bucket[5m])) by (le,type)) > 2`

---

## 8) Deployment Patterns

### 8.1 Separate Deployments
- `gateway` (gRPC API)
- `orchestrator` (DAG scheduler)
- `worker-cpu`
- `worker-io`
- `worker-dag-agg` (optional)

### 8.2 Rollouts
- Blue/Green or Canary (limit DAG types in canary).
- Quorum‑based migration: orchestrator vN only schedules DAGs vN; existing runs continue under vN‑1.

### 8.3 Config
- All knobs via env (12‑factor): pool sizes, TTLs, backoff caps, queue names, SLOs.

---

## 9) Quick Implementation Checklist

- [ ] Add DAG tables & migrations
- [ ] Build orchestrator: readiness scan + enqueue + terminal evaluation
- [ ] Extend worker payload to include `{run_id,node_id}`
- [ ] Add per‑node policies: `max_attempts`, `timeout_ms`, `on_fail`
- [ ] Build compensation hooks (optional)
- [ ] Ship Redis semaphore or per‑node concurrency limits
- [ ] Provision Prometheus rules for DAG metrics
- [ ] Add KEDA/HPA autoscaling manifests
- [ ] Add rate‑limit middleware for gRPC
- [ ] Implement outbox/inbox tables for cross‑system events
- [ ] Run chaos tests and validate SLOs

---

## 10) Minimal Code Touchpoints

- `internal/orchestrator/scan.go`: depsSatisfied, underParallelism, enqueue
- `internal/orchestrator/terminal.go`: allTerminalsSucceeded/Failed
- `cmd/orchestrator/main.go`: ticker loop + gRPC server for DAG APIs
- `internal/worker/handlers.go`: make node handlers idempotent + compensations
- `internal/metrics/dag.go`: per‑node stage counters, run gauges
- `deploy/keda/*.yaml`: Redis scalers (queue depth)
- `deploy/hpa/*.yaml`: HPA based on Prometheus Adapter metrics

---

## 11) Example: Fan‑Out Transcode DAG
```
ingest → probe ─┬→ transcode-1080p ─┬→ package
                ├→ transcode-720p  ─┤
                └→ thumbnails      ─┘
```
- Orchestrator enqueues 3 child nodes after `probe` success.
- `package` waits for all 3 `SUCCEEDED` (fan‑in).

---

## 12) Final Notes
- Keep **handlers simple and idempotent**; push orchestration complexity into the orchestrator.
- Autoscale from **queue depth** and **latency**, not CPU alone.
- Prefer **at‑least‑once** + idempotency over naive exactly‑once claims.
- Test with **failures** on purpose; practice recovery runbooks.

This add‑on gives you a credible, resume‑ready architecture that mirrors real production systems.

