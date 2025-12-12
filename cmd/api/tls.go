package main

import (
	// "crypto/tls"  // Uncomment when implementing loadMutualTLSConfig
	// "log"         // Uncomment when implementing functions
	// "os"          // Uncomment when implementing functions
	"fmt"
	// "crypto/x509" // Uncomment when implementing loadMutualTLSConfig

	"log"

	"github.com/williamntlam/go-grpc-task-scheduler/internal/utils"
	"google.golang.org/grpc/credentials"
)

// NOTE: TLS Architecture Decision
//
// For separation of concerns, TLS termination is handled at the load balancer (Envoy) level.
// This means:
//   - Clients connect to Envoy with TLS (secure)
//   - Envoy communicates with backend API servers over plain gRPC (internal network)
//   - The Go application doesn't need to handle TLS certificates
//
// Benefits:
//   - Centralized certificate management (update certs in one place)
//   - Better performance (TLS offloading from app servers)
//   - Simpler application code (no TLS configuration needed)
//   - Easier certificate rotation
//
// The functions below are kept for advanced use cases where you might want:
//   - End-to-end encryption (TLS from client all the way to app)
//   - Direct client connections (bypassing load balancer)
//   - Additional security layers
//
// For most production deployments, use TLS at Envoy and keep the app servers without TLS.

// loadServerTLSConfig loads TLS credentials for the gRPC server
// This function should:
// 1. Read TLS_CERT_PATH and TLS_KEY_PATH from environment variables
// 2. If both are set, load the server certificate and key files
// 3. Use credentials.NewServerTLSFromFile(certPath, keyPath) to create credentials
// 4. Return the credentials and nil error if successful
// 5. If certificates are not provided, return nil, nil (for development/insecure mode)
// 6. Handle errors appropriately (log and return)
//
// Example usage:
//   creds, err := loadServerTLSConfig()
//   if err != nil {
//       log.Fatalf("Failed to load TLS credentials: %v", err)
//   }
//   if creds != nil {
//       grpcServer := grpc.NewServer(grpc.Creds(creds))
//   } else {
//       grpcServer := grpc.NewServer() // Insecure mode
//   }
func loadServerTLSConfig() (credentials.TransportCredentials, error) {
	// Step 1: Get TLS_CERT_PATH from environment (use utils.GetEnv or os.Getenv)
	// Step 2: Get TLS_KEY_PATH from environment
	// Step 3: Check if both are set (if not, return nil, nil for insecure mode)
	// Step 4: Use credentials.NewServerTLSFromFile(certPath, keyPath)
	// Step 5: Return the credentials and any error
	

	certPath := utils.GetEnv("TLS_CERT_PATH", "")
	keyPath := utils.GetEnv("TLS_KEY_PATH", "")

	if certPath == "" || keyPath == "" {
		log.Println("TLS certificates not provided. Running in insecure mode.")
		return nil, nil
	}

	creds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load server TLS: %w", err)
	}

	return creds, nil
	
}

// loadClientTLSConfig loads TLS credentials for a gRPC client
// This function should:
// 1. Read TLS_CA_PATH from environment variable (CA certificate path)
// 2. If TLS_CA_PATH is set, load the CA certificate
// 3. Use credentials.NewClientTLSFromFile(caPath, "") to create credentials
//    (empty server name means use the server name from the connection)
// 4. Return the credentials and nil error if successful
// 5. If CA is not provided, return nil, nil (for development/insecure mode)
// 6. Handle errors appropriately
//
// Example usage:
//   creds, err := loadClientTLSConfig()
//   if err != nil {
//       log.Fatalf("Failed to load TLS credentials: %v", err)
//   }
//   var opts []grpc.DialOption
//   if creds != nil {
//       opts = append(opts, grpc.WithTransportCredentials(creds))
//   } else {
//       opts = append(opts, grpc.WithInsecure())
//   }
//   conn, err := grpc.Dial("server:8081", opts...)
func loadClientTLSConfig() (credentials.TransportCredentials, error) {
	// TODO: Implement this function
	// Step 1: Get TLS_CA_PATH from environment
	// Step 2: Check if it's set (if not, return nil, nil for insecure mode)
	// Step 3: Use credentials.NewClientTLSFromFile(caPath, "")
	//         (empty server name means use server name from connection)
	// Step 4: Return the credentials and any error
	
	// Example structure:
	// caPath := os.Getenv("TLS_CA_PATH")
	// if caPath == "" {
	//     log.Println("TLS CA certificate not provided, using insecure connection")
	//     return nil, nil
	// }
	// creds, err := credentials.NewClientTLSFromFile(caPath, "")
	// if err != nil {
	//     return nil, err
	// }
	// return creds, nil

	return nil, nil
}

// loadMutualTLSConfig loads TLS credentials for mutual TLS (mTLS)
// This is for advanced use cases where both client and server verify each other
// This function should:
// 1. Read TLS_CA_PATH, TLS_CLIENT_CERT_PATH, TLS_CLIENT_KEY_PATH from environment
// 2. Load CA certificate, client certificate, and client key
// 3. Create a tls.Config with:
//    - RootCAs: CA certificate pool
//    - Certificates: Client certificate and key
// 4. Use credentials.NewTLS(config) to create credentials
// 5. Return the credentials and any error
//
// Example usage (for client):
//   creds, err := loadMutualTLSConfig()
//   if err != nil {
//       log.Fatalf("Failed to load mTLS credentials: %v", err)
//   }
//   conn, err := grpc.Dial("server:8081", grpc.WithTransportCredentials(creds))
func loadMutualTLSConfig() (credentials.TransportCredentials, error) {
	// TODO: Implement this function (optional, for mTLS)
	// This is more complex and requires:
	// 1. Loading CA certificate into a certificate pool
	// 2. Loading client certificate and key
	// 3. Creating a tls.Config with both
	// 4. Using credentials.NewTLS(config)
	
	// Example structure:
	// caPath := os.Getenv("TLS_CA_PATH")
	// clientCertPath := os.Getenv("TLS_CLIENT_CERT_PATH")
	// clientKeyPath := os.Getenv("TLS_CLIENT_KEY_PATH")
	// 
	// // Load CA certificate
	// caCert, err := os.ReadFile(caPath)
	// if err != nil {
	//     return nil, err
	// }
	// caCertPool := x509.NewCertPool()
	// if !caCertPool.AppendCertsFromPEM(caCert) {
	//     return nil, fmt.Errorf("failed to parse CA certificate")
	// }
	// 
	// // Load client certificate and key
	// clientCert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	// if err != nil {
	//     return nil, err
	// }
	// 
	// // Create TLS config
	// config := &tls.Config{
	//     RootCAs:      caCertPool,
	//     Certificates: []tls.Certificate{clientCert},
	// }
	// 
	// return credentials.NewTLS(config), nil

	return nil, nil
}

// validateTLSFiles checks if TLS certificate files exist and are readable
// This is a helper function for better error messages
// Returns error if files don't exist or can't be read
func validateTLSFiles(certPath, keyPath string) error {
	// TODO: Implement this function
	// Step 1: Check if certPath file exists using os.Stat
	// Step 2: Check if keyPath file exists
	// Step 3: Try to read both files to ensure they're readable
	// Step 4: Return appropriate errors if files are missing or unreadable
	
	// Example structure:
	// if _, err := os.Stat(certPath); os.IsNotExist(err) {
	//     return fmt.Errorf("TLS certificate file not found: %s", certPath)
	// }
	// if _, err := os.Stat(keyPath); os.IsNotExist(err) {
	//     return fmt.Errorf("TLS key file not found: %s", keyPath)
	// }
	// // Try to read files to ensure they're readable
	// if _, err := os.ReadFile(certPath); err != nil {
	//     return fmt.Errorf("cannot read TLS certificate file: %v", err)
	// }
	// if _, err := os.ReadFile(keyPath); err != nil {
	//     return fmt.Errorf("cannot read TLS key file: %v", err)
	// }
	// return nil

	return nil
}

