# Code Review & Improvement Recommendations

This document contains recommendations for improving the codebase based on a comprehensive review.

## 🔴 Critical Issues

### 1. **SQL Injection Risk in `db_tx` Handler**
**Location**: `internal/worker/worker.go:914`

**Issue**: The `db_tx` job handler executes raw SQL queries from user input without proper sanitization.

```go
_, err = tx.Exec(ctx, payload.Query, payload.Params...)
```

**Risk**: While parameters are passed separately, the query itself is user-controlled and could be exploited.

**Recommendation**:
- Whitelist allowed query patterns (e.g., only UPDATE/INSERT with specific table names)
- Use a query builder or prepared statement templates
- Add validation to ensure queries match expected patterns
- Consider removing this handler type if not essential, or restrict to specific operations

### 2. **Missing TLS Implementation**
**Location**: `cmd/api/main.go:136-169`, `cmd/api/tls.go`

**Issue**: TLS configuration is commented out with TODOs, leaving the service running in insecure mode by default.

**Recommendation**:
- Complete the TLS implementation in `tls.go`
- Enable TLS by default in production environments
- Add configuration flag to explicitly allow insecure mode for development only

### 3. **Insecure Database Connection String Construction**
**Location**: `internal/db/db.go:24-32`

**Issue**: Password is directly interpolated into connection string without URL encoding.

**Recommendation**:
```go
import "net/url"

// Use url.Values or pgxpool.ParseConfig directly with struct
connString := fmt.Sprintf(
    "postgresql://%s:%s@%s:%s/%s?sslmode=%s&application_name=%s",
    url.QueryEscape(cfg.User),
    url.QueryEscape(cfg.Password), // URL encode password
    cfg.Host,
    cfg.Port,
    cfg.Database,
    cfg.SSLMode,
    "task-scheduler-api",
)
```

Or better yet, use `pgxpool.ParseConfig` with individual fields to avoid string interpolation issues.

## 🟠 High Priority Issues

### 4. **Hardcoded Database Connection Pool Settings**
**Location**: `internal/db/db.go:41-44`

**Issue**: Connection pool settings are hardcoded and not configurable.

**Recommendation**:
```go
type Config struct {
    // ... existing fields
    MaxConns        int           // Add with default
    MinConns        int           // Add with default
    MaxConnLifetime time.Duration // Add with default
    MaxConnIdleTime time.Duration // Add with default
}

func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
    // ... existing code
    poolConfig.MaxConns = cfg.MaxConns
    poolConfig.MinConns = cfg.MinConns
    // ... etc
}
```

### 5. **Missing Context Timeout in Database Operations**
**Location**: Multiple locations in `internal/db/`

**Issue**: Some database operations don't use context with timeout, which could lead to hanging operations.

**Recommendation**: Always use context with timeout for database operations:
```go
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()
// Use ctx in database operations
```

### 6. **Race Condition in Worker Shutdown**
**Location**: `internal/worker/worker.go:85-87`

**Issue**: Reading `w.ctx` without proper synchronization could lead to race conditions.

**Recommendation**: The mutex lock/unlock is good, but ensure all reads of `w.ctx` are protected.

### 7. **Missing Input Validation for HTTP Handler**
**Location**: `internal/worker/worker.go:816-878`

**Issue**: HTTP call handler doesn't validate URL scheme, which could allow SSRF attacks.

**Recommendation**:
```go
// Validate URL scheme
parsedURL, err := url.Parse(payload.URL)
if err != nil {
    return fmt.Errorf("invalid URL: %w", err)
}
if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
    return fmt.Errorf("unsupported URL scheme: %s", parsedURL.Scheme)
}
// Optionally whitelist allowed domains
```

### 8. **No Rate Limiting**
**Location**: `cmd/api/server.go`

**Issue**: No rate limiting on gRPC endpoints, making the service vulnerable to DoS attacks.

**Recommendation**:
- Add rate limiting middleware/interceptor
- Use `golang.org/x/time/rate` or similar
- Configure limits per client/IP

## 🟡 Medium Priority Issues

### 9. **Inconsistent Error Handling**
**Location**: Throughout codebase

**Issue**: Some functions return errors, others log and continue. Inconsistent patterns.

**Recommendation**:
- Establish error handling guidelines
- Use structured errors with error wrapping
- Consider using `pkg/errors` or Go 1.13+ error wrapping consistently

### 10. **Missing Structured Logging**
**Location**: Throughout codebase

**Issue**: Using standard `log` package instead of structured logging (zap, zerolog, etc.)

**Recommendation**:
- Migrate to structured logging (zap or zerolog)
- Add log levels (DEBUG, INFO, WARN, ERROR)
- Include request IDs/trace IDs for correlation

### 11. **No Connection Retry Logic**
**Location**: `internal/db/db.go`, `internal/redis/redis.go`

**Issue**: Initial connection failures cause immediate fatal errors without retry.

**Recommendation**:
```go
func NewPoolWithRetry(ctx context.Context, cfg Config, maxRetries int) (*pgxpool.Pool, error) {
    var pool *pgxpool.Pool
    var err error
    for i := 0; i < maxRetries; i++ {
        pool, err = NewPool(ctx, cfg)
        if err == nil {
            return pool, nil
        }
        if i < maxRetries-1 {
            time.Sleep(time.Duration(i+1) * time.Second)
        }
    }
    return nil, err
}
```

### 12. **Missing Metrics for Database Operations**
**Location**: `internal/db/`

**Issue**: No metrics for database query duration, connection pool usage, etc.

**Recommendation**:
- Add histogram for query duration
- Add gauge for connection pool size/usage
- Add counter for query errors

### 13. **Inefficient ListJobs Query**
**Location**: `cmd/api/server.go:660-695`

**Issue**: Using OFFSET for pagination is inefficient for large datasets.

**Recommendation**:
- Use cursor-based pagination with `task_id > last_seen_id`
- Add index on `(created_at, task_id)` for efficient ordering

### 14. **No Job Timeout**
**Location**: `internal/worker/worker.go:executeHandler`

**Issue**: Jobs can run indefinitely if they don't respect context cancellation.

**Recommendation**:
```go
// Add per-job timeout
jobCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
defer cancel()
// Use jobCtx in handler execution
```

### 15. **Missing Validation for Max Attempts**
**Location**: `cmd/api/server.go:91-96`

**Issue**: Max attempts validation allows 0, which could cause issues.

**Recommendation**: Ensure max_attempts >= 1:
```go
if job.MaxAttempts > 0 && job.MaxAttempts < 1 {
    return status.Error(codes.InvalidArgument, "max_attempts must be at least 1")
}
```

### 16. **No Graceful Degradation for Redis Failures**
**Location**: `cmd/api/server.go:303-316`

**Issue**: Redis push failures are logged but job is still considered successful.

**Recommendation**:
- Consider making Redis push failures fail the request (or have a configurable policy)
- Add circuit breaker for Redis operations
- Implement fallback mechanism

## 🔵 Low Priority / Nice to Have

### 17. **Code Duplication**
**Location**: `cmd/api/main.go` and `cmd/worker/main.go`

**Issue**: Similar initialization code duplicated between API and worker.

**Recommendation**: Extract common initialization into shared package.

### 18. **Magic Numbers**
**Location**: Throughout codebase

**Issue**: Hardcoded values like `5 * time.Minute`, `30 * time.Second`, etc.

**Recommendation**: Extract to constants or configuration:
```go
const (
    DefaultStuckJobTimeout = 5 * time.Minute
    DefaultRetryPumpInterval = 500 * time.Millisecond
    DefaultQueueDepthUpdateInterval = 5 * time.Second
)
```

### 19. **Missing Health Check for Redis in Worker**
**Location**: `cmd/worker/main.go:47-69`

**Issue**: Health check exists but could be more comprehensive.

**Recommendation**: Add Redis-specific health checks (e.g., test queue operations).

### 20. **No Request ID/Tracing**
**Location**: `cmd/api/server.go`

**Issue**: No request ID propagation for distributed tracing.

**Recommendation**:
- Add gRPC interceptor to extract/inject request IDs
- Include request ID in all logs
- Support OpenTelemetry or similar

### 21. **Missing Metrics Labels**
**Location**: `internal/metrics/metrics.go`

**Issue**: Some metrics could benefit from additional labels (e.g., job_type in processing metrics).

**Recommendation**: Review metrics and add useful labels where appropriate.

### 22. **No Configuration Validation**
**Location**: `cmd/api/main.go`, `cmd/worker/main.go`

**Issue**: Configuration values are not validated on startup.

**Recommendation**:
```go
func ValidateConfig(cfg Config) error {
    if cfg.MaxConns < cfg.MinConns {
        return fmt.Errorf("MaxConns must be >= MinConns")
    }
    // ... more validations
}
```

### 23. **Incomplete TLS Functions**
**Location**: `cmd/api/tls.go:104-125, 147-184, 189-213`

**Issue**: `loadClientTLSConfig`, `loadMutualTLSConfig`, and `validateTLSFiles` are not implemented.

**Recommendation**: Complete these functions or remove if not needed.

### 24. **No Database Migration System**
**Location**: `scripts/setup.sh`

**Issue**: Schema is created via script, not a proper migration system.

**Recommendation**: Use a migration tool like `golang-migrate` or `atlas` for versioned schema changes.

### 25. **Missing Indexes Documentation**
**Location**: `scripts/setup.sh`

**Issue**: Indexes are created but not documented why they exist.

**Recommendation**: Add comments explaining index purpose and query patterns they optimize.

### 26. **No Dead Letter Queue Processing**
**Location**: Worker code

**Issue**: Jobs are moved to DLQ but there's no mechanism to process/review them.

**Recommendation**: Add DLQ monitoring endpoint or separate DLQ processor.

### 27. **WatchJob Polling Could Be More Efficient**
**Location**: `cmd/api/server.go:396-531`

**Issue**: Polling database every second is inefficient.

**Recommendation**:
- Use database triggers/notifications (PostgreSQL LISTEN/NOTIFY)
- Or use Redis pub/sub for status updates
- Or increase polling interval with exponential backoff

### 28. **Missing Unit Tests for Error Cases**
**Location**: Test files

**Issue**: Some error paths may not be fully tested.

**Recommendation**: Add tests for:
- Database connection failures
- Redis connection failures
- Invalid input validation
- Timeout scenarios

### 29. **No Load Testing Documentation**
**Location**: `scripts/load-test.sh`

**Issue**: Load testing script exists but usage/interpretation not well documented.

**Recommendation**: Add documentation on:
- How to interpret results
- What metrics to watch
- Expected performance characteristics

### 30. **CI/CD Could Be More Comprehensive**
**Location**: `.github/workflows/ci.yml`

**Issue**: Some checks are optional (`continue-on-error: true`).

**Recommendation**:
- Make golangci-lint required (install it in CI)
- Add security scanning (gosec, etc.)
- Add dependency vulnerability scanning

## 📋 Summary by Category

### Security
- [ ] Fix SQL injection risk in db_tx handler
- [ ] Complete TLS implementation
- [ ] Add URL validation for HTTP handler (SSRF protection)
- [ ] Add rate limiting
- [ ] Fix connection string construction

### Performance
- [ ] Optimize ListJobs pagination (cursor-based)
- [ ] Add database query metrics
- [ ] Optimize WatchJob (use notifications instead of polling)
- [ ] Add connection retry logic

### Reliability
- [ ] Add job timeouts
- [ ] Improve error handling consistency
- [ ] Add circuit breaker for Redis
- [ ] Complete graceful shutdown testing

### Observability
- [ ] Migrate to structured logging
- [ ] Add request ID/tracing
- [ ] Add more comprehensive metrics
- [ ] Add database operation metrics

### Code Quality
- [ ] Remove code duplication
- [ ] Extract magic numbers to constants
- [ ] Complete TODO items
- [ ] Add configuration validation
- [ ] Improve test coverage

### Architecture
- [ ] Add database migration system
- [ ] Extract common initialization code
- [ ] Add DLQ processing mechanism
- [ ] Improve configuration management

## 🎯 Priority Recommendations

**Immediate (This Sprint)**:
1. Fix SQL injection risk (#1)
2. Complete TLS implementation (#2)
3. Add URL validation for HTTP handler (#7)
4. Add rate limiting (#8)

**Short Term (Next Sprint)**:
5. Make connection pool configurable (#4)
6. Add structured logging (#10)
7. Add job timeouts (#14)
8. Optimize ListJobs pagination (#13)

**Medium Term**:
9. Add request tracing (#20)
10. Improve error handling (#9)
11. Add comprehensive metrics (#12)
12. Add database migration system (#24)

**Long Term**:
13. Optimize WatchJob (#27)
14. Add DLQ processing (#26)
15. Improve test coverage (#28)

---

**Generated**: $(date)
**Reviewer**: AI Code Review Assistant

