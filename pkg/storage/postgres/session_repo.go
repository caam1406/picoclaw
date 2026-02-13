package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/storage/repository"
)

type sessionRepository struct {
	db dbExecutor
}

// dbExecutor is an interface that works with both *sql.DB and *sql.Tx
type dbExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// NewSessionRepository creates a new PostgreSQL session repository.
func NewSessionRepository(db dbExecutor) repository.SessionRepository {
	return &sessionRepository{db: db}
}

func (r *sessionRepository) GetOrCreate(ctx context.Context, key string) (*repository.Session, error) {
	session, err := r.Get(ctx, key)
	if err == sql.ErrNoRows {
		// Create new session
		session = &repository.Session{
			Key:      key,
			Messages: []providers.Message{},
			Created:  time.Now(),
			Updated:  time.Now(),
		}
		if err := r.Save(ctx, session); err != nil {
			return nil, err
		}
		return session, nil
	}
	return session, err
}

func (r *sessionRepository) Get(ctx context.Context, key string) (*repository.Session, error) {
	query := `SELECT key, messages, summary, created_at, updated_at
	          FROM sessions WHERE key = $1`

	var session repository.Session
	var messagesJSON []byte
	var summary sql.NullString

	err := r.db.QueryRowContext(ctx, query, key).Scan(
		&session.Key,
		&messagesJSON,
		&summary,
		&session.Created,
		&session.Updated,
	)

	if err != nil {
		return nil, err
	}

	// Unmarshal messages from JSONB
	if err := json.Unmarshal(messagesJSON, &session.Messages); err != nil {
		return nil, fmt.Errorf("failed to unmarshal messages: %w", err)
	}

	if summary.Valid {
		session.Summary = summary.String
	}

	return &session, nil
}

func (r *sessionRepository) Save(ctx context.Context, session *repository.Session) error {
	messagesJSON, err := json.Marshal(session.Messages)
	if err != nil {
		return fmt.Errorf("failed to marshal messages: %w", err)
	}

	query := `INSERT INTO sessions (key, messages, summary, created_at, updated_at)
	          VALUES ($1, $2, $3, $4, $5)
	          ON CONFLICT (key) DO UPDATE SET
	              messages = EXCLUDED.messages,
	              summary = EXCLUDED.summary,
	              updated_at = EXCLUDED.updated_at`

	_, err = r.db.ExecContext(ctx, query,
		session.Key,
		messagesJSON,
		nullString(session.Summary),
		session.Created,
		session.Updated,
	)

	return err
}

func (r *sessionRepository) AddMessage(ctx context.Context, sessionKey string, msg providers.Message) error {
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	query := `UPDATE sessions
	          SET messages = messages || $1::jsonb,
	              updated_at = $2
	          WHERE key = $3`

	result, err := r.db.ExecContext(ctx, query, msgJSON, time.Now(), sessionKey)
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

func (r *sessionRepository) GetMessages(ctx context.Context, sessionKey string) ([]providers.Message, error) {
	session, err := r.Get(ctx, sessionKey)
	if err != nil {
		return nil, err
	}
	return session.Messages, nil
}

func (r *sessionRepository) GetSummary(ctx context.Context, sessionKey string) (string, error) {
	query := `SELECT summary FROM sessions WHERE key = $1`

	var summary sql.NullString
	err := r.db.QueryRowContext(ctx, query, sessionKey).Scan(&summary)
	if err != nil {
		return "", err
	}

	if summary.Valid {
		return summary.String, nil
	}
	return "", nil
}

func (r *sessionRepository) SetSummary(ctx context.Context, sessionKey, summary string) error {
	query := `UPDATE sessions
	          SET summary = $1, updated_at = $2
	          WHERE key = $3`

	result, err := r.db.ExecContext(ctx, query, summary, time.Now(), sessionKey)
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

func (r *sessionRepository) TruncateMessages(ctx context.Context, sessionKey string, keepLast int) error {
	// PostgreSQL query to keep only the last N elements of JSONB array
	query := `UPDATE sessions
	          SET messages = (
	              SELECT jsonb_agg(elem ORDER BY ord DESC)
	              FROM (
	                  SELECT elem, ROW_NUMBER() OVER () as ord
	                  FROM jsonb_array_elements(messages) elem
	              ) subq
	              ORDER BY ord DESC
	              LIMIT $1
	          ),
	          updated_at = $2
	          WHERE key = $3`

	result, err := r.db.ExecContext(ctx, query, keepLast, time.Now(), sessionKey)
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

func (r *sessionRepository) List(ctx context.Context) ([]repository.SessionInfo, error) {
	query := `SELECT key,
	                 COALESCE(jsonb_array_length(messages), 0) as message_count,
	                 (summary IS NOT NULL AND summary != '') as has_summary,
	                 created_at,
	                 updated_at
	          FROM sessions
	          ORDER BY updated_at DESC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var infos []repository.SessionInfo
	for rows.Next() {
		var info repository.SessionInfo
		if err := rows.Scan(&info.Key, &info.MessageCount, &info.HasSummary, &info.Created, &info.Updated); err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}

	return infos, rows.Err()
}

func (r *sessionRepository) Delete(ctx context.Context, sessionKey string) error {
	query := `DELETE FROM sessions WHERE key = $1`

	result, err := r.db.ExecContext(ctx, query, sessionKey)
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

// Helper function to convert string to sql.NullString
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}
