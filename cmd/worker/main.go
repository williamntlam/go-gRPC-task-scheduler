package main

import (
	"log"

	"github.com/joho/godotenv"
)

// This is the entry point for the worker process.
// It will:
// 1. Initialize database and Redis connections
// 2. Create a Worker instance from internal/worker package
// 3. Start the worker to begin processing jobs

const (
	defaultWorkerPoolSize = 10
	defaultMetricsPort = 2113
)

func main() {
	// Load .env file if it exists (ignore errors - .env is optional)
	// Environment variables set in the shell will override .env file values
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables and defaults")
	}
}