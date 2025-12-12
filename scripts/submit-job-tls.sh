#!/bin/bash

# Script to submit a single job to the scheduler via TLS (Envoy)
# Usage: ./scripts/submit-job-tls.sh [type] [priority] [max_attempts] [payload_json]
#
# Examples:
#   ./scripts/submit-job-tls.sh noop high 3
#   ./scripts/submit-job-tls.sh http_call critical 5 '{"url":"https://example.com","method":"GET"}'
#   ./scripts/submit-job-tls.sh db_tx default 3 '{"query":"SELECT 1","params":[]}'

set -e

ENVOY_PORT=${ENVOY_PORT:-8080}
CERT_DIR="./certs"
CA_CERT="$CERT_DIR/ca.crt"
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

# Check if CA certificate exists
if [ ! -f "$CA_CERT" ]; then
    echo "Error: CA certificate not found: $CA_CERT"
    echo "Run: ./scripts/generate-certs.sh"
    exit 1
fi

echo "Submitting job via TLS (Envoy port $ENVOY_PORT)..."
echo "  Type: $TYPE"
echo "  Priority: $PRIORITY ($PRIORITY_ENUM)"
echo "  Max Attempts: $MAX_ATTEMPTS"
echo "  Payload: $PAYLOAD_JSON"
echo ""

# Submit the job via TLS (using -insecure for self-signed certs in development)
RESPONSE=$(grpcurl -insecure -d "{
  \"job\": {
    \"type\": \"$TYPE\",
    \"priority\": \"$PRIORITY_ENUM\",
    \"max_attempts\": $MAX_ATTEMPTS,
    \"payload_json\": \"$PAYLOAD_JSON\"
  }
}" localhost:$ENVOY_PORT scheduler.v1.SchedulerService/SubmitJob)

echo "Response: $RESPONSE"

# Extract job_id from response (basic parsing)
JOB_ID=$(echo "$RESPONSE" | grep -o '"job_id":"[^"]*"' | cut -d'"' -f4)

if [ -n "$JOB_ID" ]; then
    echo ""
    echo "✅ Job submitted successfully via TLS!"
    echo "   Job ID: $JOB_ID"
    echo ""
    echo "View job status:"
    echo "   grpcurl -insecure -d '{\"job_id\":\"$JOB_ID\"}' localhost:$ENVOY_PORT scheduler.v1.SchedulerService/GetJob"
else
    echo "⚠️  Could not extract job_id from response"
fi
