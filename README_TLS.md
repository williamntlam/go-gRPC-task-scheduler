# TLS Setup Guide

This guide explains how to set up TLS for your gRPC application.

## Overview

TLS (Transport Layer Security) encrypts communication between gRPC clients and servers. This guide covers:

- Server-side TLS (one-way authentication)
- Client-side TLS configuration
- Certificate generation for development

## Quick Start

### 1. Generate Development Certificates

```bash
# Make script executable
chmod +x scripts/generate-certs.sh

# Generate certificates
./scripts/generate-certs.sh
```

This creates certificates in `./certs/` directory (gitignored).

### 2. Set Environment Variables

Add to your `.env` file or export:

```bash
# Server TLS (for gRPC server)
export TLS_CERT_PATH=./certs/server.crt
export TLS_KEY_PATH=./certs/server.key

# Client TLS (for gRPC clients)
export TLS_CA_PATH=./certs/ca.crt
```

### 3. Implement TLS Functions

Complete the implementation in `cmd/api/tls.go`:

- `loadServerTLSConfig()` - Load server certificates
- `loadClientTLSConfig()` - Load CA certificate for client verification
- `loadMutualTLSConfig()` - Optional, for mutual TLS (mTLS)
- `validateTLSFiles()` - Validate certificate files exist

### 4. Update Server Code

The server code in `cmd/api/main.go` already has TODOs showing where to integrate TLS.

## Implementation Steps

### Step 1: Implement `loadServerTLSConfig()`

```go
func loadServerTLSConfig() (credentials.TransportCredentials, error) {
    certPath := os.Getenv("TLS_CERT_PATH")
    keyPath := os.Getenv("TLS_KEY_PATH")

    if certPath == "" || keyPath == "" {
        log.Println("TLS certificates not provided, running in insecure mode")
        return nil, nil
    }

    creds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
    if err != nil {
        return nil, fmt.Errorf("failed to load server TLS: %w", err)
    }

    return creds, nil
}
```

### Step 2: Implement `loadClientTLSConfig()`

```go
func loadClientTLSConfig() (credentials.TransportCredentials, error) {
    caPath := os.Getenv("TLS_CA_PATH")

    if caPath == "" {
        log.Println("TLS CA certificate not provided, using insecure connection")
        return nil, nil
    }

    creds, err := credentials.NewClientTLSFromFile(caPath, "")
    if err != nil {
        return nil, fmt.Errorf("failed to load client TLS: %w", err)
    }

    return creds, nil
}
```

### Step 3: Use in Server

```go
serverCreds, err := loadServerTLSConfig()
if err != nil {
    log.Fatalf("Failed to load TLS credentials: %v", err)
}

var grpcServer *grpc.Server
if serverCreds != nil {
    grpcServer = grpc.NewServer(grpc.Creds(serverCreds))
} else {
    grpcServer = grpc.NewServer() // Insecure mode
}
```

### Step 4: Use in Client

```go
clientCreds, err := loadClientTLSConfig()
if err != nil {
    log.Fatalf("Failed to load TLS credentials: %v", err)
}

var opts []grpc.DialOption
if clientCreds != nil {
    opts = append(opts, grpc.WithTransportCredentials(clientCreds))
} else {
    opts = append(opts, grpc.WithInsecure())
}

conn, err := grpc.Dial("server:8081", opts...)
```

## Certificate Generation

### Manual Generation

If you prefer to generate certificates manually:

```bash
# 1. Create CA
openssl genrsa -out ca.key 2048
openssl req -new -x509 -key ca.key -out ca.crt -days 365 \
  -subj "/CN=Local CA/O=Development/C=US"

# 2. Create server certificate
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr \
  -subj "/CN=localhost/O=Development/C=US"
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out server.crt -days 365

# 3. Clean up
rm server.csr
```

### Production Certificates

For production, use:

- **Let's Encrypt** (free, automated)
- **Your organization's CA** (for internal services)
- **Commercial CA** (DigiCert, GlobalSign, etc.)

## Environment Variables

| Variable               | Description                  | Required    |
| ---------------------- | ---------------------------- | ----------- |
| `TLS_CERT_PATH`        | Server certificate file path | Server only |
| `TLS_KEY_PATH`         | Server private key file path | Server only |
| `TLS_CA_PATH`          | CA certificate file path     | Client only |
| `TLS_CLIENT_CERT_PATH` | Client certificate (mTLS)    | Optional    |
| `TLS_CLIENT_KEY_PATH`  | Client private key (mTLS)    | Optional    |

## Security Best Practices

1. **Never commit certificates** - They're in `.gitignore`
2. **Use strong keys** - 2048-bit RSA minimum (4096-bit recommended)
3. **Rotate certificates** - Set expiration dates and renew regularly
4. **Protect private keys** - Use proper file permissions (600)
5. **Use production CAs** - Self-signed certs are for development only
6. **Enable TLS in production** - Never run production services without TLS

## Troubleshooting

### "certificate signed by unknown authority"

- Ensure `TLS_CA_PATH` points to the correct CA certificate
- Verify the server certificate was signed by the CA

### "connection refused" or "no such file or directory"

- Check certificate file paths are correct
- Verify files exist and are readable
- Check file permissions

### "tls: bad certificate"

- Verify certificate hasn't expired
- Check certificate matches the server hostname
- Ensure CA certificate is correct

## Testing

### Test with grpcurl

```bash
# Insecure (development)
grpcurl -plaintext localhost:8081 list

# With TLS
grpcurl -cacert certs/ca.crt localhost:8081 list
```

### Test with Go client

See `cmd/api/tls.go` for client TLS loading examples.

## Next Steps

1. Complete the TODOs in `cmd/api/tls.go`
2. Implement certificate generation in `scripts/generate-certs.sh`
3. Test TLS connection locally
4. Configure production certificates
5. Update deployment scripts to mount certificates
