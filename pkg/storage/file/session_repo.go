package file

import (
	"context"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/session"
	"github.com/sipeed/picoclaw/pkg/storage/repository"
)

type sessionRepository struct {
	mgr *session.SessionManager
}

// NewSessionRepository creates a new file-based session repository adapter.
func NewSessionRepository(mgr *session.SessionManager) repository.SessionRepository {
	return &sessionRepository{mgr: mgr}
}

func (r *sessionRepository) GetOrCreate(ctx context.Context, key string) (*repository.Session, error) {
	sess := r.mgr.GetOrCreate(key)
	return convertToRepoSession(sess), nil
}

func (r *sessionRepository) Get(ctx context.Context, key string) (*repository.Session, error) {
	sess := r.mgr.GetSession(key)
	if sess == nil {
		return nil, fmt.Errorf("session not found: %s", key)
	}
	return convertToRepoSession(sess), nil
}

func (r *sessionRepository) Save(ctx context.Context, sess *repository.Session) error {
	// Convert repository.Session to session.Session
	fileSess := convertToFileSession(sess)

	// Save to file
	if err := r.mgr.Save(fileSess); err != nil {
		return err
	}

	return nil
}

func (r *sessionRepository) AddMessage(ctx context.Context, sessionKey string, msg providers.Message) error {
	r.mgr.AddFullMessage(sessionKey, msg)

	// Persist to file
	sess := r.mgr.GetSession(sessionKey)
	if sess != nil {
		return r.mgr.Save(sess)
	}

	return nil
}

func (r *sessionRepository) GetMessages(ctx context.Context, sessionKey string) ([]providers.Message, error) {
	return r.mgr.GetHistory(sessionKey), nil
}

func (r *sessionRepository) GetSummary(ctx context.Context, sessionKey string) (string, error) {
	return r.mgr.GetSummary(sessionKey), nil
}

func (r *sessionRepository) SetSummary(ctx context.Context, sessionKey, summary string) error {
	r.mgr.SetSummary(sessionKey, summary)

	// Persist to file
	sess := r.mgr.GetSession(sessionKey)
	if sess != nil {
		return r.mgr.Save(sess)
	}

	return nil
}

func (r *sessionRepository) TruncateMessages(ctx context.Context, sessionKey string, keepLast int) error {
	r.mgr.TruncateHistory(sessionKey, keepLast)

	// Persist to file
	sess := r.mgr.GetSession(sessionKey)
	if sess != nil {
		return r.mgr.Save(sess)
	}

	return nil
}

func (r *sessionRepository) List(ctx context.Context) ([]repository.SessionInfo, error) {
	fileInfos := r.mgr.ListSessions()

	repoInfos := make([]repository.SessionInfo, len(fileInfos))
	for i, fi := range fileInfos {
		repoInfos[i] = repository.SessionInfo{
			Key:          fi.Key,
			MessageCount: fi.MessageCount,
			HasSummary:   fi.HasSummary,
			Created:      fi.Created,
			Updated:      fi.Updated,
		}
	}

	return repoInfos, nil
}

func (r *sessionRepository) Delete(ctx context.Context, sessionKey string) error {
	// File-based implementation doesn't have a Delete method
	// We would need to add this to SessionManager or implement file deletion here
	return fmt.Errorf("delete not implemented for file-based storage")
}

// Helper functions to convert between session types

func convertToRepoSession(fileSess *session.Session) *repository.Session {
	if fileSess == nil {
		return nil
	}

	return &repository.Session{
		Key:      fileSess.Key,
		Messages: fileSess.Messages,
		Summary:  fileSess.Summary,
		Created:  fileSess.Created,
		Updated:  fileSess.Updated,
	}
}

func convertToFileSession(repoSess *repository.Session) *session.Session {
	if repoSess == nil {
		return nil
	}

	return &session.Session{
		Key:      repoSess.Key,
		Messages: repoSess.Messages,
		Summary:  repoSess.Summary,
		Created:  repoSess.Created,
		Updated:  repoSess.Updated,
	}
}
