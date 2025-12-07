#!/bin/bash

set -e  # Exit on error

echo "🚀 Starting infrastructure services..."
make dev

echo ""
echo "⏳ Waiting for CockroachDB to be ready..."
sleep 5

# Wait for CockroachDB to be healthy
max_attempts=30
attempt=0
while [ $attempt -lt $max_attempts ]; do
    if docker exec cockroachdb curl -f http://localhost:8080/health > /dev/null 2>&1; then
        echo "✅ CockroachDB is ready!"
        break
    fi
    attempt=$((attempt + 1))
    echo "   Waiting for CockroachDB... (attempt $attempt/$max_attempts)"
    sleep 2
done

if [ $attempt -eq $max_attempts ]; then
    echo "❌ CockroachDB failed to start in time"
    exit 1
fi

echo ""
echo "📦 Initializing CockroachDB database and schema..."

# Create database if it doesn't exist
docker exec -i cockroachdb ./cockroach sql --insecure <<EOF
-- Create database
CREATE DATABASE IF NOT EXISTS scheduler;

-- Use the database
USE scheduler;

-- Create tasks table
CREATE TABLE IF NOT EXISTS tasks (
    task_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type STRING NOT NULL,
    priority STRING NOT NULL,
    payload JSONB,
    status STRING NOT NULL DEFAULT 'queued',
    attempts INT DEFAULT 0,
    max_attempts INT DEFAULT 3,
    next_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- Create task_attempts table
CREATE TABLE IF NOT EXISTS task_attempts (
    task_id UUID NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    ok BOOLEAN,
    error STRING,
    PRIMARY KEY (task_id, started_at),
    FOREIGN KEY (task_id) REFERENCES tasks(task_id) ON DELETE CASCADE
);

-- Note: idempotency_keys table removed
-- Idempotency is now handled by using idempotency_key as task_id directly
-- If task_id already exists, it means the job was already submitted (idempotent)

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status) WHERE status IN ('queued', 'running');
CREATE INDEX IF NOT EXISTS idx_tasks_next_run_at ON tasks(next_run_at) WHERE status = 'retry';
CREATE INDEX IF NOT EXISTS idx_tasks_priority_status ON tasks(priority, status);

-- Show tables
SHOW TABLES;
EOF

echo ""
echo "📦 Creating test database and schema..."

# Create test database with same schema
docker exec -i cockroachdb ./cockroach sql --insecure <<EOF
-- Create test database
CREATE DATABASE IF NOT EXISTS scheduler_test;

-- Use the test database
USE scheduler_test;

-- Create tasks table
CREATE TABLE IF NOT EXISTS tasks (
    task_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type STRING NOT NULL,
    priority STRING NOT NULL,
    payload JSONB,
    status STRING NOT NULL DEFAULT 'queued',
    attempts INT DEFAULT 0,
    max_attempts INT DEFAULT 3,
    next_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- Create task_attempts table
CREATE TABLE IF NOT EXISTS task_attempts (
    task_id UUID NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    ok BOOLEAN,
    error STRING,
    PRIMARY KEY (task_id, started_at),
    FOREIGN KEY (task_id) REFERENCES tasks(task_id) ON DELETE CASCADE
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status) WHERE status IN ('queued', 'running');
CREATE INDEX IF NOT EXISTS idx_tasks_next_run_at ON tasks(next_run_at) WHERE status = 'retry';
CREATE INDEX IF NOT EXISTS idx_tasks_priority_status ON tasks(priority, status);
EOF

echo ""
echo "✅ Setup complete!"
echo ""
echo "📊 Services:"
echo "   - CockroachDB UI: http://localhost:8080"
echo "   - Grafana: http://localhost:3000 (admin/admin)"
echo "   - Prometheus: http://localhost:9090"
echo "   - Redis: localhost:6379"
echo ""
echo "🔗 Connection strings:"
echo "   CockroachDB: postgresql://root@localhost:26257/scheduler?sslmode=disable"
echo "   Redis: localhost:6379"

