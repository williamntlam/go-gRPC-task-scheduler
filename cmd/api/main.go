package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	// "google.golang.org/grpc/credentials" // Uncomment when implementing TLS
	"google.golang.org/grpc/reflection"

	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	redisc "github.com/redis/go-redis/v9"

	"github.com/williamntlam/go-grpc-task-scheduler/internal/db"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/redis"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/utils"
	schedulerv1 "github.com/williamntlam/go-grpc-task-scheduler/proto/scheduler/v1"
)

const (
	// Default gRPC server port (can be overridden via GRPC_PORT env var)
	defaultGRPCPort = "8081"
	// Default metrics server port (can be overridden via METRICS_PORT env var)
	defaultMetricsPort = "2112"
)

func startMetricsServer(port string, dbPool *pgxpool.Pool, redisClient *redisc.Client) {
	mux := http.NewServeMux()

	// Metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Health check endpoint - checks if service is alive
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Readiness endpoint - checks if dependencies are available
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		// Check database connection
		if err := dbPool.Ping(ctx); err != nil {
			log.Printf("[Health] Database check failed: %v", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Database unavailable"))
			return
		}

		// Check Redis connection
		if err := redisClient.Ping(ctx).Err(); err != nil {
			log.Printf("[Health] Redis check failed: %v", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Redis unavailable"))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ready"))
	})

	log.Printf("Metrics and health server listening on :%s", port)
	log.Printf("  - Metrics: http://localhost:%s/metrics", port)
	log.Printf("  - Health:  http://localhost:%s/health", port)
	log.Printf("  - Ready:   http://localhost:%s/ready", port)

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Failed to start metrics server: %v", err)
	}
}

func main() {
	// Load .env file if it exists (ignore errors - .env is optional)
	// Environment variables set in the shell will override .env file values
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables and defaults")
	}

	// Get configuration from environment variables
	grpcPort := utils.GetEnv("GRPC_PORT", defaultGRPCPort)
	metricsPort := utils.GetEnv("METRICS_PORT", defaultMetricsPort)

	// Initialize CockroachDB connection pool
	dbConfig := db.Config{
		Host:     utils.GetEnv("COCKROACHDB_HOST", "localhost"),
		Port:     utils.GetEnv("COCKROACHDB_PORT", "26257"),
		User:     utils.GetEnv("COCKROACHDB_USER", "root"),
		Password: utils.GetEnv("COCKROACHDB_PASSWORD", ""),
		Database: utils.GetEnv("COCKROACHDB_DATABASE", "scheduler"),
		SSLMode:  utils.GetEnv("COCKROACHDB_SSLMODE", "disable"),
	}

	dbPool, err := db.NewPool(context.Background(), dbConfig)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.ClosePool(dbPool)
	log.Println("Connected to CockroachDB")

	// TODO: Initialize other dependencies
	// - Redis client
	// - Logger (zap/zerolog)
	// - Prometheus metrics registry

	redisConfig := redis.Config{
		Host:     utils.GetEnv("REDIS_HOST", "localhost"),
		Port:     utils.GetEnv("REDIS_PORT", "6379"),
		Password: utils.GetEnv("REDIS_PASSWORD", ""),
		DB:       0,
	}

	redisClient, err := redis.NewClient(context.Background(), redisConfig)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	defer redis.CloseClient(redisClient)
	log.Println("Connected to Redis")

	// ============================================================================
	// TLS CONFIGURATION
	// ============================================================================
	// Load TLS credentials for the gRPC server
	// If TLS certificates are provided, use them; otherwise run in insecure mode
	// TODO: Implement TLS loading
	// Step 1: Call loadServerTLSConfig() to get TLS credentials
	// Step 2: If credentials are returned, create server with grpc.Creds(creds)
	// Step 3: If nil (no TLS), create server without credentials (insecure mode)
	// Step 4: Log whether TLS is enabled or not
	//
	// Example:
	//   serverCreds, err := loadServerTLSConfig()
	//   if err != nil {
	//       log.Fatalf("Failed to load TLS credentials: %v", err)
	//   }
	//   var grpcServer *grpc.Server
	//   if serverCreds != nil {
	//       log.Println("TLS enabled for gRPC server")
	//       grpcServer = grpc.NewServer(grpc.Creds(serverCreds))
	//   } else {
	//       log.Println("WARNING: Running gRPC server in insecure mode (no TLS)")
	//       grpcServer = grpc.NewServer()
	//   }

	// TODO: Uncomment and implement the TLS loading above
	// serverCreds, err := loadServerTLSConfig()
	// if err != nil {
	//     log.Fatalf("Failed to load TLS credentials: %v", err)
	// }
	//
	// var grpcServer *grpc.Server
	// if serverCreds != nil {
	//     log.Println("TLS enabled for gRPC server")
	//     grpcServer = grpc.NewServer(grpc.Creds(serverCreds))
	// } else {
	//     log.Println("WARNING: Running gRPC server in insecure mode (no TLS)")
	//     grpcServer = grpc.NewServer()
	// }

	// Create gRPC server with interceptors (metrics, tracing, etc.)
	// TODO: Add interceptors for metrics, tracing, logging
	// TODO: Implement TLS loading above and use it here
	grpcServer := grpc.NewServer(
	// grpc.UnaryInterceptor(...), // Add metrics interceptor
	// grpc.StreamInterceptor(...), // Add streaming interceptor
	)

	// Create and register the scheduler service
	// NewServer is defined in cmd/api/server.go (same package)
	schedulerServer := NewServer(dbPool, redisClient)
	schedulerv1.RegisterSchedulerServiceServer(grpcServer, schedulerServer)

	// Enable gRPC reflection for development/testing (disable in production)
	// This allows tools like grpcurl to discover services
	reflection.Register(grpcServer)

	// Start gRPC server
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", grpcPort, err)
	}

	log.Printf("Starting gRPC server on :%s", grpcPort)
	log.Printf("Metrics server will be on :%s/metrics", metricsPort)

	// Start server in a goroutine
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC server: %v", err)
		}
	}()

	// Start metrics server (Prometheus HTTP endpoint)
	go startMetricsServer(metricsPort, dbPool, redisClient)

	// Wait for interrupt signal for graceful shutdown
	waitForShutdown(grpcServer)
}

// waitForShutdown handles graceful shutdown
func waitForShutdown(grpcServer *grpc.Server) {
	// Create a channel to receive OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Block until signal is received
	sig := <-sigChan
	log.Printf("Received signal: %v. Initiating graceful shutdown...", sig)

	// Create a context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Gracefully stop the gRPC server
	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	// Wait for graceful stop or timeout
	select {
	case <-stopped:
		log.Println("gRPC server stopped gracefully")
	case <-ctx.Done():
		log.Println("Graceful shutdown timeout exceeded, forcing stop")
		grpcServer.Stop()
	}

	log.Println("Shutdown complete")
}
