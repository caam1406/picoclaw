package storage

import (
	"context"
	"time"

	"github.com/sipeed/picoclaw/pkg/storage/repository"
)

// Storage is the main storage abstraction interface.
// It provides access to different repository interfaces for data persistence.
type Storage interface {
	// Repository accessors
	Sessions() repository.SessionRepository
	Contacts() repository.ContactsRepository
	Cron() repository.CronRepository

	// Lifecycle management
	Connect(ctx context.Context) error
	Close() error

	// Health check
	Ping(ctx context.Context) error
}

// Config holds storage configuration for different backends.
type Config struct {
	Type         string        // "file", "postgres", "sqlite"
	FilePath     string        // For file-based storage (workspace path)
	DatabaseURL  string        // For database storage (connection string)
	SSLEnabled   bool          // Enable SSL for database connections
	MaxIdleConns int           // Database connection pool - max idle connections
	MaxOpenConns int           // Database connection pool - max open connections
	MaxLifetime  time.Duration // Database connection pool - max lifetime
}

// DefaultConfig returns a default storage configuration.
func DefaultConfig(storageType string) Config {
	return Config{
		Type:         storageType,
		MaxIdleConns: 5,
		MaxOpenConns: 25,
		MaxLifetime:  5 * time.Minute,
	}
}
