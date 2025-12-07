# Test Suite

This directory contains comprehensive tests for the gRPC Task Scheduler.

## Test Structure

- `db_jobs_test.go` - Tests for database job operations (CreateJob, GetJobByID, CancelJob, JobExists)
- `db_attempts_test.go` - Tests for task attempt tracking (InsertAttempt, UpdateAttemptOnSuccess, UpdateAttemptOnFailure)
- `redis_jobs_test.go` - Tests for Redis queue operations (PushJob, GetQueueName)
- `server_test.go` - Tests for gRPC API server methods (SubmitJob, GetJob, CancelJob, ListJobs, WatchJob)
- `worker_test.go` - Tests for worker functionality (job processing, retries, DLQ, graceful shutdown)
- `integration_test.go` - End-to-end integration tests

## Test Utilities

The `internal/testutil` package provides:
- `SetupTestDB()` - Creates test database connection
- `SetupTestRedis()` - Creates test Redis client
- `CleanupTestDB()` - Cleans up test database
- `CleanupTestRedis()` - Cleans up test Redis
- `GetTestConfig()` - Gets test configuration from environment

## Running Tests

### Prerequisites

1. Start test infrastructure:
   ```bash
   make dev
   ```

2. Set test environment variables (optional):
   ```bash
   export TEST_COCKROACHDB_HOST=localhost
   export TEST_COCKROACHDB_PORT=26257
   export TEST_COCKROACHDB_DATABASE=scheduler_test
   export TEST_REDIS_HOST=localhost
   export TEST_REDIS_PORT=6379
   ```

### Run All Tests

```bash
go test ./tests/... -v
```

### Run Specific Test Suite

```bash
# Database tests
go test ./tests/... -run TestGetJobByID -v

# Redis tests
go test ./tests/... -run TestPushJob -v

# Server tests
go test ./tests/... -run TestSubmitJob -v

# Integration tests
go test ./tests/... -run TestEndToEndJobFlow -v
```

### Run with Coverage

```bash
go test ./tests/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Test Coverage

The test suite covers:

### Unit Tests
- ✅ Database operations (CRUD)
- ✅ Redis queue operations
- ✅ Job validation
- ✅ Error handling
- ✅ Edge cases (nil values, invalid inputs)
- ✅ Concurrency scenarios

### Integration Tests
- ✅ End-to-end job flow (submit → process → complete)
- ✅ Job retry flow
- ✅ Job cancellation flow
- ✅ Priority ordering
- ✅ Idempotency
- ✅ Concurrent operations

## Adding New Tests

When adding new tests:

1. Use the `setup*Test()` and `teardown*Test()` helpers
2. Clean up test data after each test
3. Use descriptive test names following the pattern: `TestFunctionName_Scenario`
4. Test both success and error cases
5. Test edge cases (nil, empty, invalid inputs)
6. Test concurrency when relevant

## Test Dependencies

Tests require:
- `github.com/stretchr/testify` - Assertions and test helpers
- Test database (CockroachDB)
- Test Redis instance

Install testify:
```bash
go get github.com/stretchr/testify/assert
go get github.com/stretchr/testify/require
```
