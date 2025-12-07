package testutil

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/db"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/redis"
	redisc "github.com/redis/go-redis/v9"
)

// TestConfig holds test configuration
type TestConfig struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBDatabase string
	RedisHost  string
	RedisPort  string
	RedisDB    int
}

// GetTestConfig returns test configuration from environment or defaults
func GetTestConfig() TestConfig {
	return TestConfig{
		DBHost:     getEnv("TEST_COCKROACHDB_HOST", "localhost"),
		DBPort:     getEnv("TEST_COCKROACHDB_PORT", "26257"),
		DBUser:     getEnv("TEST_COCKROACHDB_USER", "root"),
		DBPassword: getEnv("TEST_COCKROACHDB_PASSWORD", ""),
		DBDatabase: getEnv("TEST_COCKROACHDB_DATABASE", "scheduler_test"),
		RedisHost:  getEnv("TEST_REDIS_HOST", "localhost"),
		RedisPort:  getEnv("TEST_REDIS_PORT", "6379"),
		RedisDB:    1, // Use DB 1 for tests to avoid conflicts
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// SetupTestDB creates a test database connection pool
func SetupTestDB(ctx context.Context, cfg TestConfig) (*pgxpool.Pool, error) {
	dbConfig := db.Config{
		Host:     cfg.DBHost,
		Port:     cfg.DBPort,
		User:     cfg.DBUser,
		Password: cfg.DBPassword,
		Database: cfg.DBDatabase,
		SSLMode:  "disable",
	}

	pool, err := db.NewPool(ctx, dbConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to setup test database: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping test database: %w", err)
	}

	return pool, nil
}

// SetupTestRedis creates a test Redis client
func SetupTestRedis(ctx context.Context, cfg TestConfig) (*redisc.Client, error) {
	redisConfig := redis.Config{
		Host:     cfg.RedisHost,
		Port:     cfg.RedisPort,
		Password: "",
		DB:       cfg.RedisDB,
	}

	client, err := redis.NewClient(ctx, redisConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to setup test redis: %w", err)
	}

	return client, nil
}

// CleanupTestDB truncates all tables to clean up test data
func CleanupTestDB(ctx context.Context, pool *pgxpool.Pool) error {
	tables := []string{"task_attempts", "tasks"}
	for _, table := range tables {
		query := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to truncate table %s: %w", table, err)
		}
	}
	return nil
}

// CleanupTestRedis flushes the test Redis database
func CleanupTestRedis(ctx context.Context, client *redisc.Client) error {
	return client.FlushDB(ctx).Err()
}

// WaitForDB waits for database to be ready (useful for integration tests)
func WaitForDB(ctx context.Context, pool *pgxpool.Pool, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := pool.Ping(ctx); err == nil {
				return nil
			}
		}
	}
}

// WaitForRedis waits for Redis to be ready
func WaitForRedis(ctx context.Context, client *redisc.Client, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := client.Ping(ctx).Err(); err == nil {
				return nil
			}
		}
	}
}
