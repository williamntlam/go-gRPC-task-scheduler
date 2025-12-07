.PHONY: dev up down api worker migrate clean setup teardown test test-setup test-clean test-down

# Complete setup: start infrastructure and initialize database
setup:
	@chmod +x scripts/setup.sh
	./scripts/setup.sh

# Teardown: stop all services (use 'make teardown CLEAN=true' to remove volumes)
teardown:
	@chmod +x scripts/teardown.sh scripts/shutdown.sh
	./scripts/teardown.sh $(if $(CLEAN),--clean,)

# Start all infrastructure services (Redis, CockroachDB, Prometheus, Grafana)
dev:
	cd deploy && docker compose up -d
	@echo "Infrastructure services started!"
	@echo "CockroachDB UI: http://localhost:8080"
	@echo "Grafana: http://localhost:3000 (admin/admin)"
	@echo "Prometheus: http://localhost:9090"

# Alias for dev
up: dev

# Stop all infrastructure services
down:
	cd deploy && docker compose down

# Stop and remove all volumes (clean slate)
clean:
	cd deploy && docker compose down -v

# Start the API server
api:
	go run ./cmd/api

# Start the worker
worker:
	go run ./cmd/worker

# Run database migrations (runs setup.sh to create/update schema)
migrate:
	./scripts/setup.sh

# Check if services are running
status:
	cd deploy && docker compose ps

# View logs
logs:
	cd deploy && docker compose logs -f

# Test infrastructure setup: start only CockroachDB and Redis for testing
test-setup:
	@chmod +x scripts/setup-test.sh
	./scripts/setup-test.sh

# Stop test infrastructure (only CockroachDB and Redis)
test-down:
	cd deploy && docker compose stop cockroachdb redis

# Clean test infrastructure (stop and remove volumes)
test-clean:
	cd deploy && docker compose stop cockroachdb redis
	cd deploy && docker compose rm -f cockroachdb redis
	@echo "ℹ️  Note: Volumes are preserved. Use 'make clean' to remove all volumes."

# Run tests: setup test infrastructure, run tests, and optionally clean up
# Usage: make test              (keeps infrastructure running)
#        make test CLEAN=true    (cleans up after tests)
test: test-setup
	@echo ""
	@echo "🧪 Running tests..."
	@chmod +x scripts/test-summary.sh
	@./scripts/test-summary.sh
	@echo ""
	@if [ "$(CLEAN)" = "true" ]; then \
		echo "🧹 Cleaning up test infrastructure..."; \
		$(MAKE) test-clean; \
	else \
		echo "ℹ️  Test infrastructure still running. Use 'make test-clean' to stop it."; \
	fi

# ============================================================================
# Job Submission Commands (for testing and development)
# ============================================================================

# Submit a single job
# Usage: make submit-job TYPE=noop PRIORITY=high
#        make submit-job TYPE=http_call PRIORITY=critical PAYLOAD='{"url":"https://example.com","method":"GET"}'
submit-job:
	@chmod +x scripts/submit-job.sh
	@./scripts/submit-job.sh $(TYPE) $(PRIORITY) $(MAX_ATTEMPTS) $(PAYLOAD)

# Submit multiple jobs in bulk
# Usage: make submit-bulk COUNT=100 TYPE=noop PRIORITY=high
#        make submit-bulk COUNT=50 TYPE=http_call PRIORITY=default DELAY=50
submit-bulk:
	@chmod +x scripts/submit-jobs-bulk.sh
	@./scripts/submit-jobs-bulk.sh $(COUNT) $(TYPE) $(PRIORITY) $(DELAY)

# Submit a mixed workload (realistic distribution)
# Usage: make submit-mixed COUNT=200
submit-mixed:
	@chmod +x scripts/submit-jobs-mixed.sh
	@./scripts/submit-jobs-mixed.sh $(COUNT)

# Run a load test (sustained rate)
# Usage: make load-test RATE=10 DURATION=60 PRIORITY=default
#        make load-test RATE=50 DURATION=120 PRIORITY=high
load-test:
	@chmod +x scripts/load-test.sh
	@./scripts/load-test.sh $(RATE) $(DURATION) $(PRIORITY)

# Get job status
# Usage: make get-job JOB_ID=550e8400-e29b-41d4-a716-446655440000
get-job:
	@chmod +x scripts/get-job.sh
	@./scripts/get-job.sh $(JOB_ID)

# Quick test: submit a few jobs for basic testing
# Usage: make test-jobs
test-jobs:
	@echo "🧪 Submitting test jobs..."
	@chmod +x scripts/submit-job.sh
	@./scripts/submit-job.sh noop critical 3
	@./scripts/submit-job.sh noop high 3
	@./scripts/submit-job.sh noop default 3
	@./scripts/submit-job.sh noop low 3
	@echo "✅ Test jobs submitted! Check Grafana: http://localhost:3000"

# Quick load test: submit 100 jobs at high priority
# Usage: make quick-load
quick-load:
	@echo "⚡ Running quick load test..."
	@chmod +x scripts/submit-jobs-bulk.sh
	@./scripts/submit-jobs-bulk.sh 100 noop high 50
	@echo "✅ Load test complete! Check Grafana: http://localhost:3000"

