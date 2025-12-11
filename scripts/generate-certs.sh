#!/bin/bash

# ============================================================================
# TLS Certificate Generation Script for Development
# ============================================================================
# This script generates self-signed certificates for local development
# 
# Usage:
#   ./scripts/generate-certs.sh
#
# This will create:
#   - ca.key, ca.crt (Certificate Authority)
#   - server.key, server.crt (Server certificate)
#   - client.key, client.crt (Client certificate, optional for mTLS)
#
# Output directory: ./certs/ (gitignored)
# ============================================================================

set -e  # Exit on error

CERT_DIR="./certs"
CA_KEY="$CERT_DIR/ca.key"
CA_CRT="$CERT_DIR/ca.crt"
SERVER_KEY="$CERT_DIR/server.key"
SERVER_CSR="$CERT_DIR/server.csr"
SERVER_CRT="$CERT_DIR/server.crt"
CLIENT_KEY="$CERT_DIR/client.key"
CLIENT_CSR="$CERT_DIR/client.csr"
CLIENT_CRT="$CERT_DIR/client.crt"

# Create certs directory if it doesn't exist
mkdir -p "$CERT_DIR"

echo "=========================================="
echo "Generating TLS Certificates for Development"
echo "=========================================="
echo ""

# TODO: Implement certificate generation
# Step 1: Generate CA private key
#   openssl genrsa -out "$CA_KEY" 2048
# Step 2: Generate CA certificate (self-signed)
#   openssl req -new -x509 -key "$CA_KEY" -out "$CA_CRT" -days 365 \
#     -subj "/CN=Local Development CA/O=Development/C=US"
# Step 3: Generate server private key
#   openssl genrsa -out "$SERVER_KEY" 2048
# Step 4: Generate server certificate signing request (CSR)
#   openssl req -new -key "$SERVER_KEY" -out "$SERVER_CSR" \
#     -subj "/CN=localhost/O=Development/C=US"
# Step 5: Sign server certificate with CA
#   openssl x509 -req -in "$SERVER_CSR" -CA "$CA_CRT" -CAkey "$CA_KEY" \
#     -CAcreateserial -out "$SERVER_CRT" -days 365
# Step 6: (Optional) Generate client certificate for mTLS
#   Similar steps for client.key, client.csr, client.crt
# Step 7: Clean up CSR files (not needed after signing)
#   rm "$SERVER_CSR" "$CLIENT_CSR" 2>/dev/null || true

echo "TODO: Implement certificate generation"
echo ""
echo "Expected output files:"
echo "  - $CA_KEY (CA private key)"
echo "  - $CA_CRT (CA certificate)"
echo "  - $SERVER_KEY (Server private key)"
echo "  - $SERVER_CRT (Server certificate)"
echo ""
echo "After generation, set these environment variables:"
echo "  export TLS_CERT_PATH=$SERVER_CRT"
echo "  export TLS_KEY_PATH=$SERVER_KEY"
echo "  export TLS_CA_PATH=$CA_CRT"
echo ""

