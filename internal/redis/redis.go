package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Config holds Redis configuration
type Config struct {
	Host     string
	Port     string
	Password string
	DB       int
}

// NewClient creates a new Redis client and tests the connection
func NewClient(ctx context.Context, cfg Config) (*redis.Client, error) {
	// Validate required configuration
	if cfg.Host == "" {
		return nil, fmt.Errorf("redis host is required")
	}
	if cfg.Port == "" {
		return nil, fmt.Errorf("redis port is required")
	}

	// Build Redis options
	options := &redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	}

	// Create Redis client
	client := redis.NewClient(options)

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return client, nil
}

func CloseClient(client *redis.Client) {
	if client == nil {
		return
	}
	client.Close()
}