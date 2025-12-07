#!/bin/bash

# Script to submit a mix of different job types and priorities
# This is useful for testing queue behavior and seeing different metrics
# Usage: ./scripts/submit-jobs-mixed.sh [total_count]
#
# Example: ./scripts/submit-jobs-mixed.sh 100

set -e

GRPC_PORT=${GRPC_PORT:-8081}
TOTAL_COUNT=${1:-50}

# Distribution: 20% critical, 30% high, 40% default, 10% low
CRITICAL_COUNT=$((TOTAL_COUNT * 20 / 100))
HIGH_COUNT=$((TOTAL_COUNT * 30 / 100))
DEFAULT_COUNT=$((TOTAL_COUNT * 40 / 100))
LOW_COUNT=$((TOTAL_COUNT - CRITICAL_COUNT - HIGH_COUNT - DEFAULT_COUNT))

# Job types distribution: 50% noop, 30% http_call, 20% db_tx
NOOP_RATIO=50
HTTP_RATIO=30
DB_RATIO=20

echo "Submitting mixed job load..."
echo "  Total: $TOTAL_COUNT jobs"
echo "  Critical: $CRITICAL_COUNT"
echo "  High: $HIGH_COUNT"
echo "  Default: $DEFAULT_COUNT"
echo "  Low: $LOW_COUNT"
echo ""

# Check if grpcurl is installed
if ! command -v grpcurl &> /dev/null; then
    echo "Error: grpcurl is not installed"
    echo "Install it with: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest"
    exit 1
fi

SUBMIT_JOB() {
    local type=$1
    local priority=$2
    local payload=$3
    
    case "$priority" in
      critical) PRIORITY_ENUM="PRIORITY_CRITICAL" ;;
      high) PRIORITY_ENUM="PRIORITY_HIGH" ;;
      default) PRIORITY_ENUM="PRIORITY_DEFAULT" ;;
      low) PRIORITY_ENUM="PRIORITY_LOW" ;;
    esac
    
    grpcurl -plaintext -d "{
      \"job\": {
        \"type\": \"$type\",
        \"priority\": \"$PRIORITY_ENUM\",
        \"max_attempts\": 3,
        \"payload_json\": \"$payload\"
      }
    }" localhost:$GRPC_PORT scheduler.v1.SchedulerService/SubmitJob > /dev/null 2>&1
}

SUBMITTED=0

# Submit critical jobs
echo "Submitting critical jobs..."
for i in $(seq 1 $CRITICAL_COUNT); do
    RAND=$((RANDOM % 100))
    if [ $RAND -lt $NOOP_RATIO ]; then
        TYPE="noop"
        PAYLOAD="{}"
    elif [ $RAND -lt $((NOOP_RATIO + HTTP_RATIO)) ]; then
        TYPE="http_call"
        PAYLOAD='{"url":"https://httpbin.org/get","method":"GET"}'
    else
        TYPE="db_tx"
        PAYLOAD='{"query":"SELECT 1","params":[]}'
    fi
    
    if SUBMIT_JOB "$TYPE" "critical" "$PAYLOAD"; then
        SUBMITTED=$((SUBMITTED + 1))
    fi
    sleep 0.05
done

# Submit high priority jobs
echo "Submitting high priority jobs..."
for i in $(seq 1 $HIGH_COUNT); do
    RAND=$((RANDOM % 100))
    if [ $RAND -lt $NOOP_RATIO ]; then
        TYPE="noop"
        PAYLOAD="{}"
    elif [ $RAND -lt $((NOOP_RATIO + HTTP_RATIO)) ]; then
        TYPE="http_call"
        PAYLOAD='{"url":"https://httpbin.org/get","method":"GET"}'
    else
        TYPE="db_tx"
        PAYLOAD='{"query":"SELECT 1","params":[]}'
    fi
    
    if SUBMIT_JOB "$TYPE" "high" "$PAYLOAD"; then
        SUBMITTED=$((SUBMITTED + 1))
    fi
    sleep 0.05
done

# Submit default priority jobs
echo "Submitting default priority jobs..."
for i in $(seq 1 $DEFAULT_COUNT); do
    RAND=$((RANDOM % 100))
    if [ $RAND -lt $NOOP_RATIO ]; then
        TYPE="noop"
        PAYLOAD="{}"
    elif [ $RAND -lt $((NOOP_RATIO + HTTP_RATIO)) ]; then
        TYPE="http_call"
        PAYLOAD='{"url":"https://httpbin.org/get","method":"GET"}'
    else
        TYPE="db_tx"
        PAYLOAD='{"query":"SELECT 1","params":[]}'
    fi
    
    if SUBMIT_JOB "$TYPE" "default" "$PAYLOAD"; then
        SUBMITTED=$((SUBMITTED + 1))
    fi
    sleep 0.05
done

# Submit low priority jobs
echo "Submitting low priority jobs..."
for i in $(seq 1 $LOW_COUNT); do
    RAND=$((RANDOM % 100))
    if [ $RAND -lt $NOOP_RATIO ]; then
        TYPE="noop"
        PAYLOAD="{}"
    elif [ $RAND -lt $((NOOP_RATIO + HTTP_RATIO)) ]; then
        TYPE="http_call"
        PAYLOAD='{"url":"https://httpbin.org/get","method":"GET"}'
    else
        TYPE="db_tx"
        PAYLOAD='{"query":"SELECT 1","params":[]}'
    fi
    
    if SUBMIT_JOB "$TYPE" "low" "$PAYLOAD"; then
        SUBMITTED=$((SUBMITTED + 1))
    fi
    sleep 0.05
done

echo ""
echo "✅ Submitted $SUBMITTED jobs!"
echo ""
echo "Check your dashboards:"
echo "  Grafana: http://localhost:3000"
echo "  Prometheus: http://localhost:9090"
