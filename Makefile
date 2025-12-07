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

