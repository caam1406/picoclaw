package storage

import (
	"fmt"

	"github.com/sipeed/picoclaw/pkg/storage/file"
	"github.com/sipeed/picoclaw/pkg/storage/postgres"
)

// NewStorage creates a Storage implementation based on the provided configuration.
// Supported types: "file", "postgres", "sqlite"
func NewStorage(cfg Config) (Storage, error) {
	switch cfg.Type {
	case "file":
		return file.NewFileStorage(cfg.FilePath)
	case "postgres":
		return postgres.NewPostgresStorage(cfg.DatabaseURL, cfg.SSLEnabled, cfg.MaxIdleConns, cfg.MaxOpenConns, cfg.MaxLifetime)
	case "sqlite":
		// SQLite storage implementation (future)
		return nil, fmt.Errorf("sqlite storage not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported storage type: %s (supported: file, postgres, sqlite)", cfg.Type)
	}
}
