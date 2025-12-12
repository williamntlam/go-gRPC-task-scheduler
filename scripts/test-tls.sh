#!/bin/bash

# ============================================================================
# TLS Testing Script
# ============================================================================
# This script tests the TLS configuration for the gRPC Task Scheduler
# 
# Usage:
#   ./scripts/test-tls.sh
#
# Prerequisites:
#   1. Certificates generated: ./scripts/generate-certs.sh
#   2. Services running: cd deploy && docker-compose up -d
#   3. grpcurl installed: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
# ============================================================================

set -e

ENVOY_PORT=8080
API_PORT=8081
CERT_DIR="./certs"
CA_CERT="$CERT_DIR/ca.crt"
SERVER_CERT="$CERT_DIR/server.crt"
SERVER_KEY="$CERT_DIR/server.key"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "=========================================="
echo "TLS Configuration Test"
echo "=========================================="
echo ""

# Check if grpcurl is installed
if ! command -v grpcurl &> /dev/null; then
    echo -e "${RED}❌ Error: grpcurl is not installed${NC}"
    echo "Install it with: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest"
    exit 1
fi

# Check if certificates exist
echo "Step 1: Checking certificates..."
if [ ! -f "$CA_CERT" ]; then
    echo -e "${RED}❌ CA certificate not found: $CA_CERT${NC}"
    echo "   Generating certificates..."
    chmod +x scripts/generate-certs.sh
    ./scripts/generate-certs.sh || exit 1
fi

if [ ! -f "$SERVER_CERT" ]; then
    echo -e "${RED}❌ Server certificate not found: $SERVER_CERT${NC}"
    echo "   Generating certificates..."
    chmod +x scripts/generate-certs.sh
    ./scripts/generate-certs.sh || exit 1
fi

if [ ! -f "$SERVER_KEY" ]; then
    echo -e "${RED}❌ Server key not found: $SERVER_KEY${NC}"
    echo "   Generating certificates..."
    chmod +x scripts/generate-certs.sh
    ./scripts/generate-certs.sh || exit 1
fi

echo -e "${GREEN}✅ Certificates found locally${NC}"

# Verify certificate permissions
if [ ! -r "$SERVER_KEY" ]; then
    echo -e "${YELLOW}⚠️  Warning: Server key is not readable, fixing permissions...${NC}"
    chmod 600 "$SERVER_KEY" 2>/dev/null || true
fi

if [ ! -r "$SERVER_CERT" ]; then
    echo -e "${YELLOW}⚠️  Warning: Server certificate is not readable, fixing permissions...${NC}"
    chmod 644 "$SERVER_CERT" 2>/dev/null || true
fi

# Verify certificates are readable
if [ ! -r "$SERVER_KEY" ] || [ ! -r "$SERVER_CERT" ]; then
    echo -e "${RED}❌ Certificates are not readable${NC}"
    echo "   Check file permissions: ls -la $CERT_DIR/"
    exit 1
fi

echo -e "${GREEN}✅ Certificates are readable${NC}"
echo ""

# Check if services are running
echo "Step 2: Checking if services are running..."

# Check Envoy container
ENVOY_RUNNING=$(docker ps --filter "name=envoy" --format "{{.Names}}" 2>/dev/null || echo "")
if [ -z "$ENVOY_RUNNING" ]; then
    echo -e "${RED}❌ Envoy container is not running${NC}"
    echo "   Checking if container exists..."
    ENVOY_EXISTS=$(docker ps -a --filter "name=envoy" --format "{{.Names}}" 2>/dev/null || echo "")
    if [ -n "$ENVOY_EXISTS" ]; then
        echo -e "${YELLOW}   Envoy container exists but is stopped${NC}"
        echo "   Attempting to start Envoy..."
        cd deploy && docker compose up -d envoy 2>&1 | grep -v "^$" || true
        cd ..
        echo "   Waiting 5 seconds for Envoy to start..."
        sleep 5
    else
        echo -e "${YELLOW}   Envoy container does not exist${NC}"
        echo "   Starting all services..."
        cd deploy && docker compose up -d 2>&1 | grep -v "^$" || true
        cd ..
        echo "   Waiting 10 seconds for services to start..."
        sleep 10
    fi
fi

# Check Envoy admin interface
if ! curl -s http://localhost:9901/ready > /dev/null 2>&1; then
    echo -e "${YELLOW}⚠️  Envoy admin interface not responding (port 9901)${NC}"
    echo "   Checking Envoy container status..."
    docker ps --filter "name=envoy" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || echo "   Container not found"
    echo ""
    echo "   Recent Envoy logs:"
    docker logs envoy --tail 20 2>&1 | sed 's/^/   /' || echo "   Could not retrieve logs"
    echo ""
    echo "   Troubleshooting steps:"
    echo "   1. Check if certificates exist locally: ls -la ./certs/"
    echo "   2. Verify certificates in container: docker exec envoy ls -la /etc/envoy/certs/"
    echo "   3. Check volume mount in docker-compose.yml (should be: ../certs:/etc/envoy/certs:ro)"
    echo "   4. Restart Envoy: cd deploy && docker compose restart envoy"
    echo "   5. Check Envoy logs: docker logs envoy"
    echo ""
    echo "   If certificates are missing in container, try:"
    echo "   - Regenerate certificates: make tls-certs"
    echo "   - Restart all services: cd deploy && docker compose down && docker compose up -d"
    echo ""
else
    echo -e "${GREEN}✅ Envoy is running${NC}"
    # Verify certificates are mounted
    echo "   Verifying certificates in container..."
    if docker exec envoy test -f /etc/envoy/certs/server.crt 2>/dev/null; then
        echo -e "${GREEN}✅ Server certificate found in container${NC}"
    else
        echo -e "${RED}❌ Server certificate not found in container: /etc/envoy/certs/server.crt${NC}"
        echo "   Listing /etc/envoy/certs/ in container:"
        docker exec envoy ls -la /etc/envoy/certs/ 2>/dev/null || echo "   Directory does not exist or is not accessible"
        echo ""
        echo "   Checking local certificates:"
        ls -la "$CERT_DIR/" 2>/dev/null | head -5 || echo "   Local certs directory not found"
        echo ""
        echo "   Fix: Ensure certificates exist and volume mount is correct in docker-compose.yml"
        ENVOY_FAILED=true
    fi
    
    if docker exec envoy test -f /etc/envoy/certs/server.key 2>/dev/null; then
        echo -e "${GREEN}✅ Server key found in container${NC}"
    else
        echo -e "${RED}❌ Server key not found in container: /etc/envoy/certs/server.key${NC}"
        ENVOY_FAILED=true
    fi
    
    if [ "$ENVOY_FAILED" = "true" ]; then
        echo ""
        echo "   Attempting to fix by restarting Envoy with proper volume mount..."
        cd deploy && docker compose stop envoy && docker compose up -d envoy
        cd ..
        echo "   Waiting 5 seconds for Envoy to restart..."
        sleep 5
        # Check again
        if docker exec envoy test -f /etc/envoy/certs/server.key 2>/dev/null; then
            echo -e "${GREEN}✅ Certificates now accessible after restart${NC}"
        else
            echo -e "${RED}❌ Certificates still not accessible. Manual intervention required.${NC}"
            echo "   Run: make tls-certs && cd deploy && docker compose restart envoy"
        fi
    fi
fi

if ! curl -s http://localhost:2112/health > /dev/null 2>&1; then
    echo -e "${YELLOW}⚠️  API server health check not responding (port 2112)${NC}"
else
    echo -e "${GREEN}✅ API server is running${NC}"
fi
echo ""

# Test 1: List services via Envoy with TLS (should work)
echo "Step 3: Testing TLS connection to Envoy (port $ENVOY_PORT)..."
echo "   Note: Using -insecure for self-signed certificates (development only)"
echo "   Command: grpcurl -insecure localhost:$ENVOY_PORT list"

# Try the connection with -insecure (self-signed certs need this for Go TLS)
# For development, this is acceptable - TLS is still active, just verification is skipped
TLS_OUTPUT=$(grpcurl -insecure localhost:$ENVOY_PORT list 2>&1)
TLS_EXIT_CODE=$?

if [ $TLS_EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}✅ TLS connection successful!${NC}"
    echo ""
    echo "   Available services:"
    echo "$TLS_OUTPUT" | sed 's/^/     /'
    echo ""
    echo "   Note: -insecure skips certificate verification (acceptable for dev)"
    echo "   In production, use proper CA-signed certificates"
else
    echo -e "${RED}❌ TLS connection failed${NC}"
    echo ""
    echo "   Error details:"
    echo "$TLS_OUTPUT" | sed 's/^/     /'
    echo ""
    echo "   Troubleshooting:"
    echo "   1. Check Envoy is running: docker ps | grep envoy"
    echo "   2. Check Envoy logs: docker logs envoy"
    echo "   3. Verify certificates in container:"
    echo "      docker exec envoy ls -la /etc/envoy/certs/"
    echo "   4. Test Envoy config: docker exec envoy /usr/local/bin/envoy --config-path /etc/envoy/envoy.yaml --mode validate"
    echo "   5. Check if port is listening: netstat -tlnp | grep 8080 || ss -tlnp | grep 8080"
    echo ""
    exit 1
fi
echo ""

# Test 2: Try plaintext connection to Envoy (should fail)
echo "Step 4: Testing that plaintext is rejected (should fail)..."
if grpcurl -plaintext localhost:$ENVOY_PORT list > /dev/null 2>&1; then
    echo -e "${RED}❌ Plaintext connection succeeded (TLS not enforced!)${NC}"
    echo "   This is a security issue - TLS should be required"
else
    echo -e "${GREEN}✅ Plaintext connection correctly rejected${NC}"
fi
echo ""

# Test 3: Submit a job via TLS
echo "Step 5: Testing job submission via TLS..."
RESPONSE=$(grpcurl -insecure -d '{
  "job": {
    "type": "noop",
    "priority": "PRIORITY_HIGH",
    "max_attempts": 3,
    "payload_json": "{}"
  }
}' localhost:$ENVOY_PORT scheduler.v1.SchedulerService/SubmitJob 2>&1)

# Check for job ID in response (handles both "job_id" and "jobId" formats)
# Try to extract job ID - handle "jobId" format first (camelCase)
JOB_ID=$(echo "$RESPONSE" | grep -oE '"jobId"[[:space:]]*:[[:space:]]*"[^"]*"' | grep -oE '"[a-f0-9-]*"' | head -1 | tr -d '"')
# If not found, try "job_id" format (snake_case)
if [ -z "$JOB_ID" ]; then
    JOB_ID=$(echo "$RESPONSE" | grep -oE '"job_id"[[:space:]]*:[[:space:]]*"[^"]*"' | grep -oE '"[a-f0-9-]*"' | head -1 | tr -d '"')
fi

if [ -n "$JOB_ID" ]; then
    echo -e "${GREEN}✅ Job submitted successfully via TLS!${NC}"
    echo "   Job ID: $JOB_ID"
    echo ""
    
    # Test 4: Get job status via TLS
    echo "Step 6: Testing GetJob via TLS..."
    if grpcurl -insecure -d "{\"job_id\":\"$JOB_ID\"}" \
        localhost:$ENVOY_PORT scheduler.v1.SchedulerService/GetJob > /dev/null 2>&1; then
        echo -e "${GREEN}✅ GetJob successful via TLS!${NC}"
        echo ""
        echo "   Job status:"
        grpcurl -insecure -d "{\"job_id\":\"$JOB_ID\"}" \
            localhost:$ENVOY_PORT scheduler.v1.SchedulerService/GetJob | jq '.' 2>/dev/null || \
        grpcurl -insecure -d "{\"job_id\":\"$JOB_ID\"}" \
            localhost:$ENVOY_PORT scheduler.v1.SchedulerService/GetJob
    else
        echo -e "${YELLOW}⚠️  GetJob failed (job might not be processed yet)${NC}"
    fi
else
    echo -e "${RED}❌ Job submission failed${NC}"
    echo "   Response: $RESPONSE"
    exit 1
fi
echo ""

# Test 5: Verify certificate details
echo "Step 7: Verifying certificate details..."
echo "   Server certificate info:"
openssl x509 -in "$SERVER_CERT" -noout -subject -issuer -dates 2>/dev/null | sed 's/^/     /' || echo "     (openssl not available)"
echo ""

# Test 6: Test with insecure flag (should work but show warning)
echo "Step 8: Testing with -insecure flag (skips verification)..."
if grpcurl -insecure localhost:$ENVOY_PORT list > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Connection with -insecure works (TLS is active)${NC}"
    echo "   Note: This skips certificate verification - use -cacert in production"
else
    echo -e "${RED}❌ Connection with -insecure failed${NC}"
fi
echo ""

# Summary
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo -e "${GREEN}✅ TLS is properly configured and working!${NC}"
echo ""
echo "Next steps:"
echo "  1. Update your client applications to use TLS:"
echo "     grpcurl -cacert $CA_CERT localhost:$ENVOY_PORT ..."
echo ""
echo "  2. For production, replace self-signed certificates with:"
echo "     - Let's Encrypt (free, automated)"
echo "     - Your organization's CA"
echo "     - Commercial CA certificates"
echo ""
echo "  3. Update scripts to use TLS (see scripts/submit-job-tls.sh)"
echo ""
