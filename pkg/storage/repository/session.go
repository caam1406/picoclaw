package repository

import (
	"context"
	"time"

	"github.com/sipeed/picoclaw/pkg/providers"
)

// Session represents a conversation session with message history.
type Session struct {
	Key      string              `json:"key"`
	Messages []providers.Message `json:"messages"`
	Summary  string              `json:"summary,omitempty"`
	Created  time.Time           `json:"created"`
	Updated  time.Time           `json:"updated"`
}

// SessionInfo provides summary information about a session.
type SessionInfo struct {
	Key          string    `json:"key"`
	MessageCount int       `json:"message_count"`
	HasSummary   bool      `json:"has_summary"`
	Created      time.Time `json:"created"`
	Updated      time.Time `json:"updated"`
}

// SessionRepository defines the interface for session persistence operations.
type SessionRepository interface {
	// GetOrCreate retrieves an existing session or creates a new one if it doesn't exist.
	GetOrCreate(ctx context.Context, key string) (*Session, error)

	// Get retrieves a session by its key.
	// Returns error if session is not found.
	Get(ctx context.Context, key string) (*Session, error)

	// Save persists a complete session (replaces existing).
	Save(ctx context.Context, session *Session) error

	// AddMessage appends a new message to the session.
	// Updates the session's Updated timestamp.
	AddMessage(ctx context.Context, sessionKey string, msg providers.Message) error

	// GetMessages retrieves all messages for a session.
	GetMessages(ctx context.Context, sessionKey string) ([]providers.Message, error)

	// GetSummary retrieves the session summary.
	GetSummary(ctx context.Context, sessionKey string) (string, error)

	// SetSummary updates the session summary.
	SetSummary(ctx context.Context, sessionKey, summary string) error

	// TruncateMessages keeps only the last N messages in the session.
	// Used for managing session history size.
	TruncateMessages(ctx context.Context, sessionKey string, keepLast int) error

	// List returns summary information for all sessions.
	List(ctx context.Context) ([]SessionInfo, error)

	// Delete removes a session permanently.
	Delete(ctx context.Context, sessionKey string) error
}
