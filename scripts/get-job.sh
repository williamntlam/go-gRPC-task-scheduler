#!/bin/bash

# Script to get job status
# Usage: ./scripts/get-job.sh [job_id]

set -e

GRPC_PORT=${GRPC_PORT:-8081}
JOB_ID=${1}

if [ -z "$JOB_ID" ]; then
    echo "Error: job_id is required"
    echo "Usage: ./scripts/get-job.sh <job_id>"
    exit 1
fi

# Check if grpcurl is installed
if ! command -v grpcurl &> /dev/null; then
    echo "Error: grpcurl is not installed"
    echo "Install it with: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest"
    exit 1
fi

echo "Getting job status for: $JOB_ID"
echo ""

grpcurl -plaintext -d "{
  \"job_id\": \"$JOB_ID\"
}" localhost:$GRPC_PORT scheduler.v1.SchedulerService/GetJob | jq '.'
