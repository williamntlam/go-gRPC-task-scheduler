.PHONY: dev up down api worker migrate clean setup teardown

# Complete setup: start infrastructure and initialize database
setup:
	./scripts/setup.sh

# Teardown: stop all services (use 'make teardown CLEAN=true' to remove volumes)
teardown:
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

