package redisstore

import (
	"time"
)

// Config holds the Redis configuration with hardcoded values
type Config struct {
	Enabled         bool
	Network         string // "unix"
	Address         string // Unix socket path
	Username        string
	Password        string
	DB              int
	PoolSize        int
	MinIdle         int
	MaxRetries      int
	RetryDelay      time.Duration
	CompressEnabled bool // Enable/disable compression
}

func DefaultConfig() *Config {
	return &Config{
		Enabled:         true,
		Network:         "unix",
		Address:         "/media/redis/local.sock",
		Username:        "root",
		Password:        "root",
		DB:              0,
		PoolSize:        100,
		MinIdle:         10,
		MaxRetries:      3,
		RetryDelay:      time.Second * 2,
		CompressEnabled: false,
	}
}

// IsEnabled returns whether Redis storage is enabled
func (c *Config) IsEnabled() bool {
	return c.Enabled
}
