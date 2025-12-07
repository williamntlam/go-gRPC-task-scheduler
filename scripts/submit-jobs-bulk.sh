#!/bin/bash

# Script to submit multiple jobs in bulk for testing
# Usage: ./scripts/submit-jobs-bulk.sh [count] [type] [priority] [delay_ms]
#
# Examples:
#   ./scripts/submit-jobs-bulk.sh 100 noop high 100    # 100 high-priority noop jobs, 100ms delay
#   ./scripts/submit-jobs-bulk.sh 50 http_call default 50
#   ./scripts/submit-jobs-bulk.sh 20 db_tx critical 0  # 20 critical jobs, no delay (burst)

set -e

GRPC_PORT=${GRPC_PORT:-8081}
COUNT=${1:-10}
TYPE=${2:-noop}
PRIORITY=${3:-default}
DELAY_MS=${4:-100}

# Convert priority to protobuf enum format
case "$PRIORITY" in
  critical|CRITICAL)
    PRIORITY_ENUM="PRIORITY_CRITICAL"
    ;;
  high|HIGH)
    PRIORITY_ENUM="PRIORITY_HIGH"
    ;;
  default|DEFAULT)
    PRIORITY_ENUM="PRIORITY_DEFAULT"
    ;;
  low|LOW)
    PRIORITY_ENUM="PRIORITY_LOW"
    ;;
  *)
    echo "Error: Invalid priority. Must be: critical, high, default, or low"
    exit 1
    ;;
esac

# Check if grpcurl is installed
if ! command -v grpcurl &> /dev/null; then
    echo "Error: grpcurl is not installed"
    echo "Install it with: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest"
    exit 1
fi

echo "Submitting $COUNT jobs..."
echo "  Type: $TYPE"
echo "  Priority: $PRIORITY ($PRIORITY_ENUM)"
echo "  Delay: ${DELAY_MS}ms between submissions"
echo ""

SUCCESS=0
FAILED=0
START_TIME=$(date +%s)

for i in $(seq 1 $COUNT); do
    if grpcurl -plaintext -d "{
      \"job\": {
        \"type\": \"$TYPE\",
        \"priority\": \"$PRIORITY_ENUM\",
        \"max_attempts\": 3,
        \"payload_json\": \"{}\"
      }
    }" localhost:$GRPC_PORT scheduler.v1.SchedulerService/SubmitJob > /dev/null 2>&1; then
        SUCCESS=$((SUCCESS + 1))
        if [ $((i % 10)) -eq 0 ]; then
            echo "  Submitted $i/$COUNT jobs..."
        fi
    else
        FAILED=$((FAILED + 1))
        echo "  ⚠️  Failed to submit job $i"
    fi
    
    # Delay between submissions (except for last one)
    if [ $i -lt $COUNT ] && [ $DELAY_MS -gt 0 ]; then
        sleep $(echo "scale=3; $DELAY_MS / 1000" | bc)
    fi
done

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

echo ""
echo "✅ Submission complete!"
echo "   Success: $SUCCESS"
echo "   Failed: $FAILED"
echo "   Duration: ${DURATION}s"
echo "   Rate: $(echo "scale=2; $SUCCESS / $DURATION" | bc) jobs/sec"
echo ""
echo "Check Grafana dashboard at http://localhost:3000 to see metrics!"
