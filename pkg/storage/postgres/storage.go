package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"github.com/sipeed/picoclaw/pkg/storage/repository"
)

// PostgresStorage implements the storage.Storage interface for PostgreSQL.
type PostgresStorage struct {
	db       *sql.DB
	sessions repository.SessionRepository
	contacts repository.ContactsRepository
	cron     repository.CronRepository
}

// Config holds PostgreSQL-specific configuration
type Config struct {
	DatabaseURL  string
	SSLEnabled   bool
	MaxIdleConns int
	MaxOpenConns int
	MaxLifetime  time.Duration
}

// NewPostgresStorage creates a new PostgreSQL storage instance.
func NewPostgresStorage(databaseURL string, sslEnabled bool, maxIdleConns, maxOpenConns int, maxLifetime time.Duration) (*PostgresStorage, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("database URL is required for PostgreSQL storage")
	}

	// Add or modify sslmode in the connection string
	if !strings.Contains(databaseURL, "sslmode=") {
		// If sslmode is not present, add it based on sslEnabled
		sep := "?"
		if strings.Contains(databaseURL, "?") {
			sep = "&"
		}

		if sslEnabled {
			databaseURL = databaseURL + sep + "sslmode=require"
		} else {
			databaseURL = databaseURL + sep + "sslmode=disable"
		}
	}
	// If sslmode is already in the URL, respect the existing value

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL connection: %w", err)
	}

	// Configure connection pool
	if maxIdleConns > 0 {
		db.SetMaxIdleConns(maxIdleConns)
	}
	if maxOpenConns > 0 {
		db.SetMaxOpenConns(maxOpenConns)
	}
	if maxLifetime > 0 {
		db.SetConnMaxLifetime(maxLifetime)
	}

	s := &PostgresStorage{
		db: db,
	}

	// Initialize repositories
	s.sessions = NewSessionRepository(db)
	s.contacts = NewContactsRepository(db)
	s.cron = NewCronRepository(db)

	return s, nil
}

// Connect establishes connection and runs migrations.
func (s *PostgresStorage) Connect(ctx context.Context) error {
	// Test connection
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Run migrations
	if err := RunMigrations(s.db); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (s *PostgresStorage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Sessions returns the session repository.
func (s *PostgresStorage) Sessions() repository.SessionRepository {
	return s.sessions
}

// Contacts returns the contacts repository.
func (s *PostgresStorage) Contacts() repository.ContactsRepository {
	return s.contacts
}

// Cron returns the cron repository.
func (s *PostgresStorage) Cron() repository.CronRepository {
	return s.cron
}

// Ping checks if the database connection is alive.
func (s *PostgresStorage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}
