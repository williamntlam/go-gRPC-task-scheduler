package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	redisc "github.com/redis/go-redis/v9"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/db"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/redis"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/utils"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/worker"
)

// This is the entry point for the worker process.
// It will:
// 1. Initialize database and Redis connections
// 2. Create a Worker instance from internal/worker package
// 3. Start the worker to begin processing jobs

const (
	defaultWorkerPoolSize = 10
	defaultMetricsPort    = 2113
)

func startMetricsServer(port int, dbPool *pgxpool.Pool, redisClient *redisc.Client) {
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
	
	log.Printf("Metrics and health server listening on :%d", port)
	log.Printf("  - Metrics: http://localhost:%d/metrics", port)
	log.Printf("  - Health:  http://localhost:%d/health", port)
	log.Printf("  - Ready:   http://localhost:%d/ready", port)
	
	if err := http.ListenAndServe(":"+strconv.Itoa(port), mux); err != nil {
		log.Fatalf("Failed to start metrics server: %v", err)
	}
}

func main() {
	// Step 1: Load .env file if it exists (ignore errors - .env is optional)
	// Environment variables set in the shell will override .env file values
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables and defaults")
	}

	// Step 2: Get configuration from environment variables
	// - Get WORKER_POOL_SIZE (use utils.GetEnv, convert to int with strconv.Atoi)
	// - Get METRICS_PORT (use utils.GetEnv, convert to int with strconv.Atoi)
	// - Handle conversion errors (use default if invalid)
	workerPoolSizeStr := utils.GetEnv("WORKER_POOL_SIZE", strconv.Itoa(defaultWorkerPoolSize))
	workerPoolSize, err := strconv.Atoi(workerPoolSizeStr)
	if err != nil {
		log.Printf("Invalid WORKER_POOL_SIZE, using default: %d", defaultWorkerPoolSize)
		workerPoolSize = defaultWorkerPoolSize
	}

	metricsPortStr := utils.GetEnv("METRICS_PORT", strconv.Itoa(defaultMetricsPort))
	metricsPort, err := strconv.Atoi(metricsPortStr)

	if err != nil {
		log.Printf("Invalid METRICS_PORT, using default: %d", defaultMetricsPort)
		metricsPort = defaultMetricsPort
	}

	// Step 3: Initialize CockroachDB connection pool
	// - Create db.Config struct with env vars:
	//   - Host: utils.GetEnv("COCKROACHDB_HOST", "localhost")
	//   - Port: utils.GetEnv("COCKROACHDB_PORT", "26257")
	//   - User: utils.GetEnv("COCKROACHDB_USER", "root")
	//   - Password: utils.GetEnv("COCKROACHDB_PASSWORD", "")
	//   - Database: utils.GetEnv("COCKROACHDB_DATABASE", "scheduler")
	//   - SSLMode: utils.GetEnv("COCKROACHDB_SSLMODE", "disable")
	// - Call db.NewPool(context.Background(), dbConfig)
	// - Handle errors with log.Fatalf()
	// - Add defer db.ClosePool(dbPool)
	// - Log success: log.Println("Connected to CockroachDB")

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

	// Step 4: Initialize Redis connection
	// - Create redis.Config struct with env vars:
	//   - Host: utils.GetEnv("REDIS_HOST", "localhost")
	//   - Port: utils.GetEnv("REDIS_PORT", "6379")
	//   - Password: utils.GetEnv("REDIS_PASSWORD", "")
	//   - DB: 0
	// - Call redis.NewClient(context.Background(), redisConfig)
	// - Handle errors with log.Fatalf()
	// - Add defer redis.CloseClient(redisClient)
	// - Log success: log.Println("Connected to Redis")

	redisConfig := redis.Config{
		Host: utils.GetEnv("REDIS_HOST", "localhost"),
		Port: utils.GetEnv("REDIS_PORT", "6379"),
		Password: utils.GetEnv("REDIS_PASSWORD", ""),
		DB: 0,
	}

	redisClient, err := redis.NewClient(context.Background(), redisConfig)
	if err != nil {
		log.Fatalf("Failed to connect to redis: %v", err)
	}
	defer redis.CloseClient(redisClient)
	log.Printf("Connected to Redis")

	// Step 5: Parse worker pool size
	// - Convert workerPoolSize string to int using strconv.Atoi()
	// - Handle errors (if invalid, use defaultWorkerPoolSize)
	// - Store in variable (e.g., poolSize)

	// Step 6: Create Worker instance
	w := worker.NewWorker(dbPool, redisClient, workerPoolSize)

	// Step 7: Start worker
	// - Start worker in goroutine: go w.Start()
	// - Log: log.Printf("Starting worker with pool size: %d", poolSize)
	log.Printf("Starting worker with pool size: %d", workerPoolSize)
	go w.Start()

	// Start metrics server (Prometheus HTTP endpoint)
	go startMetricsServer(metricsPort, dbPool, redisClient)

	// Step 8: Handle graceful shutdown
	waitForShutdown(w)
}

// waitForShutdown handles graceful shutdown of the worker
func waitForShutdown(w *worker.Worker) {
	// Create a channel to receive OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Block until signal is received
	sig := <-sigChan
	log.Printf("Received signal: %v. Initiating graceful shutdown...", sig)

	// Create a context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Call worker's Stop() method
	if err := w.Stop(ctx); err != nil {
		log.Printf("Error during worker shutdown: %v", err)
	}

	log.Println("Shutdown complete")
}