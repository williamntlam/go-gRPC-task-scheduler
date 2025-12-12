package testutil

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	redisc "github.com/redis/go-redis/v9"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/db"
	"github.com/williamntlam/go-grpc-task-scheduler/internal/redis"
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

	// Test connection and verify we're connected to the right database
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping test database: %w", err)
	}

	// Verify database exists by checking current database
	var currentDB string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&currentDB); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to verify database: %w", err)
	}

	if currentDB != cfg.DBDatabase {
		pool.Close()
		return nil, fmt.Errorf("connected to wrong database: expected %s, got %s", cfg.DBDatabase, currentDB)
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
// If tables don't exist, this is a no-op (useful for first run)
func CleanupTestDB(ctx context.Context, pool *pgxpool.Pool) error {
	// Check if database connection is valid
	if err := pool.Ping(ctx); err != nil {
		// If connection fails, database might not exist yet - that's okay for first cleanup
		return nil
	}

	tables := []string{"task_attempts", "tasks"}
	for _, table := range tables {
		// Check if table exists before truncating
		var exists bool
		checkQuery := `
			SELECT EXISTS (
				SELECT FROM information_schema.tables 
				WHERE table_schema = 'public' AND table_name = $1
			)
		`
		if err := pool.QueryRow(ctx, checkQuery, table).Scan(&exists); err != nil {
			// If we can't check, skip this table (might be permissions issue or table doesn't exist)
			continue
		}

		if exists {
			query := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)
			if _, err := pool.Exec(ctx, query); err != nil {
				// Ignore errors about tables not existing (race condition)
				if strings.Contains(err.Error(), "does not exist") {
					continue
				}
				return fmt.Errorf("failed to truncate table %s: %w", table, err)
			}
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
