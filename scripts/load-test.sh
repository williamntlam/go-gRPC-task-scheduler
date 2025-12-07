#!/bin/bash

# Load testing script - submits jobs at a sustained rate
# Usage: ./scripts/load-test.sh [rate_per_sec] [duration_sec] [priority]
#
# Examples:
#   ./scripts/load-test.sh 10 60 high      # 10 jobs/sec for 60 seconds, high priority
#   ./scripts/load-test.sh 50 120 default  # 50 jobs/sec for 2 minutes, default priority

set -e

GRPC_PORT=${GRPC_PORT:-8081}
RATE=${1:-10}        # jobs per second
DURATION=${2:-60}    # seconds
PRIORITY=${3:-default}

# Convert priority
case "$PRIORITY" in
  critical|CRITICAL) PRIORITY_ENUM="PRIORITY_CRITICAL" ;;
  high|HIGH) PRIORITY_ENUM="PRIORITY_HIGH" ;;
  default|DEFAULT) PRIORITY_ENUM="PRIORITY_DEFAULT" ;;
  low|LOW) PRIORITY_ENUM="PRIORITY_LOW" ;;
  *)
    echo "Error: Invalid priority"
    exit 1
    ;;
esac

# Check if grpcurl is installed
if ! command -v grpcurl &> /dev/null; then
    echo "Error: grpcurl is not installed"
    exit 1
fi

INTERVAL=$(echo "scale=3; 1 / $RATE" | bc)
TOTAL_JOBS=$((RATE * DURATION))

echo "Starting load test..."
echo "  Rate: $RATE jobs/sec"
echo "  Duration: ${DURATION}s"
echo "  Priority: $PRIORITY"
echo "  Total jobs: ~$TOTAL_JOBS"
echo "  Interval: ${INTERVAL}s between jobs"
echo ""
echo "Press Ctrl+C to stop early"
echo ""

SUBMIT_JOB() {
    grpcurl -plaintext -d "{
      \"job\": {
        \"type\": \"noop\",
        \"priority\": \"$PRIORITY_ENUM\",
        \"max_attempts\": 3,
        \"payload_json\": \"{}\"
      }
    }" localhost:$GRPC_PORT scheduler.v1.SchedulerService/SubmitJob > /dev/null 2>&1
}

SUCCESS=0
FAILED=0
START_TIME=$(date +%s)
END_TIME=$((START_TIME + DURATION))

while [ $(date +%s) -lt $END_TIME ]; do
    if SUBMIT_JOB; then
        SUCCESS=$((SUCCESS + 1))
    else
        FAILED=$((FAILED + 1))
    fi
    
    sleep $INTERVAL
done

ELAPSED=$(($(date +%s) - START_TIME))
ACTUAL_RATE=$(echo "scale=2; $SUCCESS / $ELAPSED" | bc)

echo ""
echo "✅ Load test complete!"
echo "   Submitted: $SUCCESS"
echo "   Failed: $FAILED"
echo "   Duration: ${ELAPSED}s"
echo "   Actual rate: ${ACTUAL_RATE} jobs/sec"
echo ""
echo "Check Grafana for real-time metrics: http://localhost:3000"
