package metrics

// ============================================================================
// STEP 1: Import Required Packages
// ============================================================================
// You'll need to import:
//   - "github.com/prometheus/client_golang/prometheus" - Core metrics types
//   - "github.com/prometheus/client_golang/prometheus/promauto" - Auto-registration
//
// Example:
//   import (
//       "github.com/prometheus/client_golang/prometheus"
//       "github.com/prometheus/client_golang/prometheus/promauto"
//   )

// ============================================================================
// STEP 2: Understand Metric Types
// ============================================================================
// Before creating metrics, understand the three main types:
//
// 1. Counter: Monotonically increasing value (only goes up)
//    - Use for: total counts (jobs submitted, jobs processed)
//    - Example: jobs_submitted_total
//
// 2. Gauge: Current value that can go up or down
//    - Use for: current state (queue depth, in-flight jobs)
//    - Example: queue_depth
//
// 3. Histogram: Distribution of values over time
//    - Use for: durations, sizes, latencies
//    - Example: job_processing_duration_seconds
//
// All metrics use "Vec" versions (CounterVec, GaugeVec, HistogramVec)
// which allow you to add labels (like priority, status, job_type)

// ============================================================================
// STEP 3: Define Your Metrics Variables
// ============================================================================
// Create a var block with all your metrics.
// Use promauto.New*Vec() functions - they automatically register metrics
// with Prometheus default registry, so you don't need manual registration.
//
// Structure for each metric:
//   var MetricName = promauto.New[Counter|Gauge|Histogram]Vec(
//       prometheus.[Counter|Gauge|Histogram]Opts{
//           Name: "metric_name_in_snake_case",
//           Help: "Description of what this metric tracks",
//           // For Histogram only: Buckets: []float64{...} or prometheus.DefBuckets
//       },
//       []string{"label1", "label2"}, // Labels you'll use when recording
//   )

// ============================================================================
// STEP 4: Create API Server Metrics
// ============================================================================
// For the API server (cmd/api/server.go), you'll want:

// 4.1 JobsSubmitted (CounterVec)
//   - Name: "jobs_submitted_total"
//   - Help: "Total number of jobs submitted to the scheduler"
//   - Labels: []string{"priority"} (critical, high, default, low)
//   - Use: Increment in SubmitJob() after successful job creation
//   - Example usage: JobsSubmitted.WithLabelValues("high").Inc()

// 4.2 JobsSubmittedErrors (CounterVec) - Optional but recommended
//   - Name: "jobs_submitted_errors_total"
//   - Help: "Total number of failed job submissions"
//   - Labels: []string{"error_type"} (validation, database, redis)
//   - Use: Increment in SubmitJob() when errors occur

// 4.3 GRPCRequests (CounterVec) - Optional, for all gRPC methods
//   - Name: "grpc_requests_total"
//   - Help: "Total number of gRPC requests"
//   - Labels: []string{"method", "status"} (method: SubmitJob/GetJob/etc, status: success/error)
//   - Use: Increment at start/end of each gRPC method

// 4.4 GRPCRequestDuration (HistogramVec) - Optional, for latency tracking
//   - Name: "grpc_request_duration_seconds"
//   - Help: "Duration of gRPC requests in seconds"
//   - Labels: []string{"method"}
//   - Buckets: prometheus.DefBuckets (or custom: []float64{0.001, 0.01, 0.1, 0.5, 1, 2, 5})
//   - Use: Measure time in each gRPC method

// ============================================================================
// STEP 5: Create Worker Metrics
// ============================================================================
// For the worker (internal/worker/worker.go), you'll want:

// 5.1 JobsProcessed (CounterVec)
//   - Name: "jobs_processed_total"
//   - Help: "Total number of jobs processed by workers"
//   - Labels: []string{"status"} (success, failed, retry)
//   - Use: Increment when job completes (success) or fails
//   - Example: JobsProcessed.WithLabelValues("success").Inc()

// 5.2 JobDuration (HistogramVec)
//   - Name: "job_processing_duration_seconds"
//   - Help: "Time spent processing individual jobs"
//   - Labels: []string{"job_type"} (noop, http_call, db_tx)
//   - Buckets: prometheus.DefBuckets or custom buckets
//   - Use: Measure time in executeHandler() function
//   - Example: JobDuration.WithLabelValues("http_call").Observe(duration)

// 5.3 QueueDepth (GaugeVec)
//   - Name: "queue_depth"
//   - Help: "Current number of jobs waiting in each priority queue"
//   - Labels: []string{"priority"} (critical, high, default, low)
//   - Use: Update periodically in worker Start() loop or reaper
//   - Example: QueueDepth.WithLabelValues("high").Set(float64(depth))

// 5.4 JobsRetried (CounterVec)
//   - Name: "jobs_retried_total"
//   - Help: "Total number of jobs scheduled for retry"
//   - Labels: []string{"priority"}
//   - Use: Increment in handleJobFailure() when job is retried

// 5.5 JobsDeadlettered (CounterVec)
//   - Name: "jobs_deadlettered_total"
//   - Help: "Total number of jobs moved to dead letter queue"
//   - Labels: []string{"priority"}
//   - Use: Increment in handleJobFailure() when max attempts reached

// 5.6 WorkerInflightJobs (Gauge) - No labels needed
//   - Name: "worker_inflight_jobs"
//   - Help: "Current number of jobs being processed by workers"
//   - Use: Increment when job starts, decrement when job finishes
//   - Example: WorkerInflightJobs.Inc() / WorkerInflightJobs.Dec()

// ============================================================================
// STEP 6: Implementation Notes
// ============================================================================

// 6.1 Naming Conventions
//   - Use snake_case for metric names
//   - Counters should end with "_total"
//   - Duration metrics should end with "_seconds"
//   - Be descriptive: "jobs_submitted_total" not "jobs_total"

// 6.2 Labels
//   - Keep label cardinality LOW (don't use job_id as a label!)
//   - Use labels for dimensions you'll query by (priority, status, type)
//   - Too many unique label combinations = performance problems
//   - Good labels: priority, status, job_type, error_type
//   - Bad labels: job_id, user_id, timestamp

// 6.3 Help Text
//   - Write clear, descriptive help text
//   - This appears in Prometheus UI and documentation
//   - Example: "Total number of jobs submitted to the scheduler" (good)
//   - Example: "Jobs" (bad - too vague)

// 6.4 Histogram Buckets
//   - Buckets define the distribution ranges
//   - prometheus.DefBuckets is good for most cases: [.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10]
//   - For job durations, you might want: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60}
//   - Choose buckets based on your expected values

// 6.5 Using promauto vs Manual Registration
//   - promauto.New*Vec() automatically registers with default registry
//   - This is simpler and recommended for most cases
//   - Manual registration requires calling prometheus.MustRegister() in init()

// ============================================================================
// STEP 7: Example Structure (Reference Only - Don't Copy!)
// ============================================================================
// Here's what the structure looks like (for understanding, not copying):
//
// var (
//     JobsSubmitted = promauto.NewCounterVec(
//         prometheus.CounterOpts{
//             Name: "jobs_submitted_total",
//             Help: "Total number of jobs submitted to the scheduler",
//         },
//         []string{"priority"},
//     )
//
//     JobDuration = promauto.NewHistogramVec(
//         prometheus.HistogramOpts{
//             Name:    "job_processing_duration_seconds",
//             Help:    "Time spent processing jobs in seconds",
//             Buckets: prometheus.DefBuckets,
//         },
//         []string{"job_type"},
//     )
//
//     QueueDepth = promauto.NewGaugeVec(
//         prometheus.GaugeOpts{
//             Name: "queue_depth",
//             Help: "Current number of jobs waiting in queue",
//         },
//         []string{"priority"},
//     )
// )

// ============================================================================
// STEP 8: What to Implement First (Priority Order)
// ============================================================================
// Start with these essential metrics, then add more:

// Priority 1 (Essential):
//   1. JobsSubmitted - Know your throughput
//   2. JobsProcessed (by status) - Know success/failure rates
//   3. JobDuration - Know if jobs are slow

// Priority 2 (Important):
//   4. QueueDepth - Know if queues are backing up
//   5. JobsRetried - Know retry patterns
//   6. JobsDeadlettered - Know failure rates

// Priority 3 (Nice to have):
//   7. WorkerInflightJobs - Know worker utilization
//   8. GRPCRequests/GRPCRequestDuration - Know API performance

// ============================================================================
// STEP 9: After Creating Metrics Here
// ============================================================================
// Once you've defined your metrics in this file:

// 1. Import this package in cmd/api/server.go
//    import "github.com/williamntlam/go-grpc-task-scheduler/internal/metrics"

// 2. Use metrics in your code:
//    metrics.JobsSubmitted.WithLabelValues("high").Inc()
//    metrics.JobDuration.WithLabelValues("http_call").Observe(duration)
//    metrics.QueueDepth.WithLabelValues("critical").Set(float64(depth))

// 3. Expose /metrics endpoint (see METRICS_GUIDE.md Step 3-4)

// ============================================================================
// STEP 10: Testing Your Metrics
// ============================================================================
// After implementing:

// 1. Start your services (make api, make worker)
// 2. Check metrics endpoint: curl http://localhost:2112/metrics
// 3. Submit some jobs
// 4. Check metrics again - values should have changed
// 5. Verify in Prometheus UI (if configured)

// If metrics don't appear:
//   - Check that you're using promauto (auto-registration)
//   - Verify /metrics endpoint is running
//   - Check that you're actually calling the metric methods in your code

// ============================================================================
// NOW: Implement Your Metrics Below
// ============================================================================
// Start by creating the var block and adding your first metric.
// Follow the patterns described above.
// Start simple with JobsSubmitted, then add more as needed.
