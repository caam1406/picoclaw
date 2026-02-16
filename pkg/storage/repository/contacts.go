package repository

import (
	"context"
	"time"
)

// ContactInstruction represents per-contact custom instructions and metadata.
type ContactInstruction struct {
	ContactID            string    `json:"contact_id"`
	Channel              string    `json:"channel"`
	DisplayName          string    `json:"display_name"`
	AgentID              string    `json:"agent_id,omitempty"`
	AllowedMCPs          []string  `json:"allowed_mcps,omitempty"`
	Instructions         string    `json:"instructions"`
	ResponseDelaySeconds int       `json:"response_delay_seconds,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// ContactsRepository defines the interface for contact instruction persistence.
type ContactsRepository interface {
	// Get retrieves contact instructions by channel and contact ID.
	// Returns nil if contact is not found.
	Get(ctx context.Context, channel, contactID string) (*ContactInstruction, error)

	// Set creates or updates contact instructions.
	// Handles timestamp management (CreatedAt, UpdatedAt).
	Set(ctx context.Context, ci ContactInstruction) error

	// Delete removes contact instructions.
	// Returns error if contact is not found.
	Delete(ctx context.Context, channel, contactID string) error

	// List returns all contact instructions.
	List(ctx context.Context) ([]ContactInstruction, error)

	// GetForSession looks up contact by session key (format: "channel:contactID").
	// Supports special cases like WhatsApp JID formats.
	GetForSession(ctx context.Context, sessionKey string) (string, error)

	// IsRegistered checks if a contact exists for the given session key.
	IsRegistered(ctx context.Context, sessionKey string) (bool, error)

	// Count returns the total number of registered contacts.
	Count(ctx context.Context) (int, error)
}
