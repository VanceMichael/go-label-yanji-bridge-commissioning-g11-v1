package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Address           string
	DatabasePath      string
	SessionTTL        time.Duration
	WorkerInterval    time.Duration
	ShutdownTimeout   time.Duration
	MaxRequestBytes   int64
	BootstrapPassword string
}

func Load() (Config, error) {
	cfg := Config{
		Address:           env("BRIDGEWATCH_ADDR", ":8080"),
		DatabasePath:      env("BRIDGEWATCH_DATABASE", "bridgewatch.db"),
		SessionTTL:        8 * time.Hour,
		WorkerInterval:    500 * time.Millisecond,
		ShutdownTimeout:   10 * time.Second,
		MaxRequestBytes:   1 << 20,
		BootstrapPassword: os.Getenv("BRIDGEWATCH_BOOTSTRAP_PASSWORD"),
	}
	var err error
	if cfg.SessionTTL, err = duration("BRIDGEWATCH_SESSION_TTL", cfg.SessionTTL); err != nil {
		return Config{}, err
	}
	if cfg.WorkerInterval, err = duration("BRIDGEWATCH_WORKER_INTERVAL", cfg.WorkerInterval); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = duration("BRIDGEWATCH_SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	if raw := os.Getenv("BRIDGEWATCH_MAX_REQUEST_BYTES"); raw != "" {
		cfg.MaxRequestBytes, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || cfg.MaxRequestBytes < 1024 {
			return Config{}, fmt.Errorf("BRIDGEWATCH_MAX_REQUEST_BYTES: invalid positive size")
		}
	}
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	if c.Address == "" || c.DatabasePath == "" {
		return errors.New("address and database path are required")
	}
	if len(c.BootstrapPassword) < 12 {
		return errors.New("BRIDGEWATCH_BOOTSTRAP_PASSWORD must contain at least 12 characters")
	}
	if c.SessionTTL <= 0 || c.WorkerInterval <= 0 || c.ShutdownTimeout <= 0 {
		return errors.New("durations must be positive")
	}
	return nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func duration(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s: invalid positive duration", key)
	}
	return value, nil
}
