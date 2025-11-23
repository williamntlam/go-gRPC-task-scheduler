package utils

import "os"

// GetEnv retrieves an environment variable or returns a default value
// If the environment variable is set (non-empty), it returns that value.
// Otherwise, it returns the defaultValue.
func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

