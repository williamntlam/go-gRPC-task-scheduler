#!/bin/bash

set -e  # Exit on error

# Check if --clean flag is provided to remove volumes
CLEAN_VOLUMES=false
if [[ "$1" == "--clean" ]] || [[ "$1" == "-c" ]]; then
    CLEAN_VOLUMES=true
fi

echo "🛑 Shutting down infrastructure services..."

if [ "$CLEAN_VOLUMES" = true ]; then
    echo "   (This will also remove all volumes - clean slate)"
    cd deploy && docker compose down -v
else
    cd deploy && docker compose down
fi

echo ""
echo "✅ All services stopped!"

if [ "$CLEAN_VOLUMES" = true ]; then
    echo ""
    echo "🧹 Volumes removed - next setup will start fresh"
else
    echo ""
    echo "💡 Tip: Use './scripts/teardown.sh --clean' to also remove volumes"
    echo "   (This will delete all data: CockroachDB, Redis, Prometheus, Grafana)"
fi

