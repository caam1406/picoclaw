package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/storage/repository"
)

type contactsRepository struct {
	db dbExecutor
}

// NewContactsRepository creates a new PostgreSQL contacts repository.
func NewContactsRepository(db dbExecutor) repository.ContactsRepository {
	return &contactsRepository{db: db}
}

func (r *contactsRepository) Get(ctx context.Context, channel, contactID string) (*repository.ContactInstruction, error) {
	query := `SELECT contact_id, channel, display_name, agent_id, allowed_mcps, instructions, response_delay_seconds, created_at, updated_at
	          FROM contact_instructions
	          WHERE channel = $1 AND contact_id = $2`

	var ci repository.ContactInstruction
	var allowedRaw []byte
	err := r.db.QueryRowContext(ctx, query, channel, contactID).Scan(
		&ci.ContactID,
		&ci.Channel,
		&ci.DisplayName,
		&ci.AgentID,
		&allowedRaw,
		&ci.Instructions,
		&ci.ResponseDelaySeconds,
		&ci.CreatedAt,
		&ci.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Return nil instead of error when not found
	}
	if err != nil {
		return nil, err
	}
	if len(allowedRaw) > 0 {
		_ = json.Unmarshal(allowedRaw, &ci.AllowedMCPs)
	}

	return &ci, nil
}

func (r *contactsRepository) Set(ctx context.Context, ci repository.ContactInstruction) error {
	now := time.Now()

	// If CreatedAt is zero, this is a new contact
	if ci.CreatedAt.IsZero() {
		ci.CreatedAt = now
	}
	ci.UpdatedAt = now

	allowedJSON, _ := json.Marshal(ci.AllowedMCPs)
	query := `INSERT INTO contact_instructions (channel, contact_id, display_name, agent_id, allowed_mcps, instructions, response_delay_seconds, created_at, updated_at)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	          ON CONFLICT (channel, contact_id) DO UPDATE SET
	              display_name = EXCLUDED.display_name,
	              agent_id = EXCLUDED.agent_id,
	              allowed_mcps = EXCLUDED.allowed_mcps,
	              instructions = EXCLUDED.instructions,
	              response_delay_seconds = EXCLUDED.response_delay_seconds,
	              updated_at = EXCLUDED.updated_at`

	_, err := r.db.ExecContext(ctx, query,
		ci.Channel,
		ci.ContactID,
		ci.DisplayName,
		ci.AgentID,
		allowedJSON,
		ci.Instructions,
		ci.ResponseDelaySeconds,
		ci.CreatedAt,
		ci.UpdatedAt,
	)

	return err
}

func (r *contactsRepository) Delete(ctx context.Context, channel, contactID string) error {
	query := `DELETE FROM contact_instructions WHERE channel = $1 AND contact_id = $2`

	result, err := r.db.ExecContext(ctx, query, channel, contactID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (r *contactsRepository) List(ctx context.Context) ([]repository.ContactInstruction, error) {
	query := `SELECT contact_id, channel, display_name, agent_id, allowed_mcps, instructions, response_delay_seconds, created_at, updated_at
	          FROM contact_instructions
	          ORDER BY updated_at DESC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []repository.ContactInstruction
	for rows.Next() {
		var ci repository.ContactInstruction
		var allowedRaw []byte
		if err := rows.Scan(&ci.ContactID, &ci.Channel, &ci.DisplayName, &ci.AgentID, &allowedRaw, &ci.Instructions, &ci.ResponseDelaySeconds, &ci.CreatedAt, &ci.UpdatedAt); err != nil {
			return nil, err
		}
		if len(allowedRaw) > 0 {
			_ = json.Unmarshal(allowedRaw, &ci.AllowedMCPs)
		}
		contacts = append(contacts, ci)
	}

	return contacts, rows.Err()
}

func (r *contactsRepository) GetForSession(ctx context.Context, sessionKey string) (string, error) {
	// Parse session key (format: "channel:contactID")
	parts := strings.SplitN(sessionKey, ":", 2)
	if len(parts) != 2 {
		return "", nil
	}

	channel := parts[0]
	contactID := parts[1]

	// Try exact match first
	ci, err := r.Get(ctx, channel, contactID)
	if err != nil {
		return "", err
	}
	if ci != nil {
		return ci.Instructions, nil
	}

	// For WhatsApp, try without @s.whatsapp.net suffix
	if channel == "whatsapp" && strings.HasSuffix(contactID, "@s.whatsapp.net") {
		bareID := strings.TrimSuffix(contactID, "@s.whatsapp.net")
		ci, err = r.Get(ctx, channel, bareID)
		if err != nil {
			return "", err
		}
		if ci != nil {
			return ci.Instructions, nil
		}
	}

	return "", nil
}

func (r *contactsRepository) IsRegistered(ctx context.Context, sessionKey string) (bool, error) {
	instructions, err := r.GetForSession(ctx, sessionKey)
	if err != nil {
		return false, err
	}
	return instructions != "", nil
}

func (r *contactsRepository) Count(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM contact_instructions`

	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}
