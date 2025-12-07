#!/bin/bash

# Script to submit a single job to the scheduler
# Usage: ./scripts/submit-job.sh [type] [priority] [max_attempts] [payload_json]
#
# Examples:
#   ./scripts/submit-job.sh noop high 3
#   ./scripts/submit-job.sh http_call critical 5 '{"url":"https://example.com","method":"GET"}'
#   ./scripts/submit-job.sh db_tx default 3 '{"query":"SELECT 1","params":[]}'

set -e

GRPC_PORT=${GRPC_PORT:-8081}
TYPE=${1:-noop}
PRIORITY=${2:-default}
MAX_ATTEMPTS=${3:-3}
PAYLOAD_JSON=${4:-"{}"}

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

echo "Submitting job..."
echo "  Type: $TYPE"
echo "  Priority: $PRIORITY ($PRIORITY_ENUM)"
echo "  Max Attempts: $MAX_ATTEMPTS"
echo "  Payload: $PAYLOAD_JSON"
echo ""

# Submit the job
RESPONSE=$(grpcurl -plaintext -d "{
  \"job\": {
    \"type\": \"$TYPE\",
    \"priority\": \"$PRIORITY_ENUM\",
    \"max_attempts\": $MAX_ATTEMPTS,
    \"payload_json\": \"$PAYLOAD_JSON\"
  }
}" localhost:$GRPC_PORT scheduler.v1.SchedulerService/SubmitJob)

echo "Response: $RESPONSE"

# Extract job_id from response (basic parsing)
JOB_ID=$(echo "$RESPONSE" | grep -o '"job_id":"[^"]*"' | cut -d'"' -f4)

if [ -n "$JOB_ID" ]; then
    echo ""
    echo "✅ Job submitted successfully!"
    echo "   Job ID: $JOB_ID"
    echo ""
    echo "View job status:"
    echo "   grpcurl -plaintext -d '{\"job_id\":\"$JOB_ID\"}' localhost:$GRPC_PORT scheduler.v1.SchedulerService/GetJob"
else
    echo "⚠️  Could not extract job_id from response"
fi
