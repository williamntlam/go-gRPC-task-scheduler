#!/bin/bash

# Get the script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DEPLOY_DIR="$PROJECT_ROOT/deploy"

# Check if --clean flag is provided to remove volumes
CLEAN_VOLUMES=false
if [[ "$1" == "--clean" ]] || [[ "$1" == "-c" ]]; then
    CLEAN_VOLUMES=true
fi

echo "🛑 Shutting down infrastructure services..."

# Stop all infrastructure services (production + test)
# This handles both full infrastructure and test-only infrastructure
if [ "$CLEAN_VOLUMES" = true ]; then
    echo "   (This will also remove all volumes - clean slate)"
    cd "$DEPLOY_DIR" && docker compose down -v || true
else
    cd "$DEPLOY_DIR" && docker compose down || true
fi

# Also ensure test infrastructure containers are stopped (in case they were started separately)
# This is a safety measure in case test infrastructure was started independently
echo ""
echo "🛑 Ensuring test infrastructure is stopped..."
cd "$DEPLOY_DIR" && docker compose stop cockroachdb redis 2>/dev/null || true
cd "$DEPLOY_DIR" && docker compose rm -f cockroachdb redis 2>/dev/null || true

echo ""
echo "✅ All services stopped!"

if [ "$CLEAN_VOLUMES" = true ]; then
    echo ""
    echo "🧹 Volumes removed - next setup will start fresh"
else
    echo ""
    echo "💡 Tip: Use './scripts/shutdown.sh --clean' to also remove volumes"
    echo "   (This will delete all data: CockroachDB, Redis, Prometheus, Grafana)"
fi

