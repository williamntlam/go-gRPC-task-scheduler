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

# Step 1: Generate CA private key
echo "Step 1: Generating CA private key..."
openssl genrsa -out "$CA_KEY" 2048
chmod 600 "$CA_KEY"

# Step 2: Generate CA certificate (self-signed) with proper extensions
echo "Step 2: Generating CA certificate..."
openssl req -new -x509 -key "$CA_KEY" -out "$CA_CRT" -days 365 \
  -subj "/CN=Local Development CA/O=Development/C=US" \
  -extensions v3_ca -config <(echo "[req]"; echo "distinguished_name=req"; echo "[v3_ca]"; echo "basicConstraints=critical,CA:true"; echo "keyUsage=keyCertSign,cRLSign")

# Step 3: Generate server private key
echo "Step 3: Generating server private key..."
openssl genrsa -out "$SERVER_KEY" 2048
# Set permissions: readable by all (required for Docker bind mount access)
# Note: For development only. In production, use proper user/group mapping
# or copy certificates into container image with correct ownership
chmod 644 "$SERVER_KEY"

# Step 4: Generate server certificate signing request (CSR) with SAN
echo "Step 4: Generating server certificate signing request..."
openssl req -new -key "$SERVER_KEY" -out "$SERVER_CSR" \
  -subj "/CN=localhost/O=Development/C=US" \
  -addext "subjectAltName=DNS:localhost,DNS:*.localhost,IP:127.0.0.1,IP:0.0.0.0"

# Step 5: Sign server certificate with CA
echo "Step 5: Signing server certificate with CA..."
openssl x509 -req -in "$SERVER_CSR" -CA "$CA_CRT" -CAkey "$CA_KEY" \
  -CAcreateserial -out "$SERVER_CRT" -days 365 \
  -extensions v3_req -extfile <(echo "[v3_req]"; echo "subjectAltName=DNS:localhost,DNS:*.localhost,IP:127.0.0.1,IP:0.0.0.0")

# Step 6: Clean up CSR files (not needed after signing)
echo "Step 6: Cleaning up temporary files..."
rm "$SERVER_CSR" 2>/dev/null || true
rm "$CERT_DIR/ca.srl" 2>/dev/null || true

echo ""
echo "=========================================="
echo "Certificate generation complete!"
echo "=========================================="
echo ""
echo "Generated files:"
echo "  - $CA_KEY (CA private key)"
echo "  - $CA_CRT (CA certificate)"
echo "  - $SERVER_KEY (Server private key)"
echo "  - $SERVER_CRT (Server certificate)"
echo ""
echo "For Envoy TLS termination, these files are needed:"
echo "  - $SERVER_CRT -> /etc/envoy/certs/server.crt"
echo "  - $SERVER_KEY -> /etc/envoy/certs/server.key"
echo ""
echo "For client verification (optional):"
echo "  - $CA_CRT (use this to verify the server certificate)"
echo ""

