.PHONY: dev up down api worker migrate clean setup teardown test test-setup test-clean test-down
.PHONY: docker-build-api docker-build-worker docker-build docker-run-api docker-run-worker docker-stop-api docker-stop-worker

# Complete setup: start infrastructure and initialize database
setup:
	@chmod +x scripts/setup.sh
	./scripts/setup.sh

# Teardown: stop all services (use 'make teardown CLEAN=true' to remove volumes)
teardown:
	@chmod +x scripts/teardown.sh scripts/shutdown.sh
	./scripts/teardown.sh $(if $(CLEAN),--clean,)

# Start all infrastructure services (Redis, CockroachDB, Prometheus, Grafana, Envoy Load Balancer)
dev:
	cd deploy && docker compose up -d
	@echo "Infrastructure services started!"
	@echo "CockroachDB UI: http://localhost:8082"
	@echo "Grafana: http://localhost:3000 (admin/admin)"
	@echo "Prometheus: http://localhost:9090"
	@echo "Envoy Load Balancer: localhost:8080 (gRPC endpoint)"
	@echo "Envoy Admin: http://localhost:9901"

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

# ============================================================================
# Docker Commands (for consistent development environment)
# ============================================================================

# Build Docker images
docker-build-api:
	@echo "🐳 Building API server Docker image..."
	docker build -t scheduler-api:latest -f cmd/api/Dockerfile .

docker-build-worker:
	@echo "🐳 Building worker Docker image..."
	docker build -t scheduler-worker:latest -f cmd/worker/Dockerfile .

docker-build: docker-build-api docker-build-worker
	@echo "✅ All Docker images built successfully!"

# Run containers (connects to infrastructure network)
# Note: Network name is auto-detected from docker-compose network
docker-run-api: docker-build-api
	@echo "🚀 Starting API server in Docker..."
	@echo "ℹ️  Make sure infrastructure is running: make dev"
	@NETWORK=$$(docker inspect cockroachdb --format='{{range $$k, $$v := .NetworkSettings.Networks}}{{$$k}}{{end}}' 2>/dev/null | head -1); \
	if [ -z "$$NETWORK" ]; then \
		echo "❌ Error: Infrastructure not running. Run 'make dev' first."; \
		exit 1; \
	fi; \
	echo "📡 Connecting to network: $$NETWORK"; \
	docker run -d --rm \
		--name scheduler-api \
		--network $$NETWORK \
		-p 8081:8081 \
		-p 2112:2112 \
		-e GRPC_PORT=8081 \
		-e METRICS_PORT=2112 \
		-e COCKROACHDB_HOST=cockroachdb \
		-e COCKROACHDB_PORT=26257 \
		-e COCKROACHDB_USER=root \
		-e COCKROACHDB_PASSWORD= \
		-e COCKROACHDB_DATABASE=scheduler \
		-e COCKROACHDB_SSLMODE=disable \
		-e REDIS_HOST=redis \
		-e REDIS_PORT=6379 \
		-e REDIS_PASSWORD= \
		scheduler-api:latest
	@echo "✅ API server started! Logs: docker logs -f scheduler-api"

docker-run-worker: docker-build-worker
	@echo "🚀 Starting worker in Docker..."
	@echo "ℹ️  Make sure infrastructure is running: make dev"
	@NETWORK=$$(docker inspect cockroachdb --format='{{range $$k, $$v := .NetworkSettings.Networks}}{{$$k}}{{end}}' 2>/dev/null | head -1); \
	if [ -z "$$NETWORK" ]; then \
		echo "❌ Error: Infrastructure not running. Run 'make dev' first."; \
		exit 1; \
	fi; \
	echo "📡 Connecting to network: $$NETWORK"; \
	docker run -d --rm \
		--name scheduler-worker \
		--network $$NETWORK \
		-p 2113:2113 \
		-e WORKER_POOL_SIZE=10 \
		-e METRICS_PORT=2113 \
		-e COCKROACHDB_HOST=cockroachdb \
		-e COCKROACHDB_PORT=26257 \
		-e COCKROACHDB_USER=root \
		-e COCKROACHDB_PASSWORD= \
		-e COCKROACHDB_DATABASE=scheduler \
		-e COCKROACHDB_SSLMODE=disable \
		-e REDIS_HOST=redis \
		-e REDIS_PORT=6379 \
		-e REDIS_PASSWORD= \
		scheduler-worker:latest
	@echo "✅ Worker started! Logs: docker logs -f scheduler-worker"

# Stop containers
docker-stop-api:
	@echo "🛑 Stopping API server container..."
	@docker stop scheduler-api 2>/dev/null || echo "API server container not running"

docker-stop-worker:
	@echo "🛑 Stopping worker container..."
	@docker stop scheduler-worker 2>/dev/null || echo "Worker container not running"

docker-stop: docker-stop-api docker-stop-worker
	@echo "✅ All containers stopped"

# View container logs
docker-logs-api:
	docker logs -f scheduler-api

docker-logs-worker:
	docker logs -f scheduler-worker

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

