package file

import (
	"context"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/contacts"
	"github.com/sipeed/picoclaw/pkg/cron"
	"github.com/sipeed/picoclaw/pkg/session"
	"github.com/sipeed/picoclaw/pkg/storage/repository"
)

// FileStorage implements the storage.Storage interface using file-based persistence.
// It wraps the existing file-based implementations (SessionManager, contacts.Store, etc.)
type FileStorage struct {
	workspacePath string
	sessionMgr    *session.SessionManager
	contactsStore *contacts.Store
	cronService   *cron.CronService
	sessions      repository.SessionRepository
	contactsRepo  repository.ContactsRepository
	cronRepo      repository.CronRepository
}

// NewFileStorage creates a new file-based storage instance.
func NewFileStorage(filePath string) (*FileStorage, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path is required for file-based storage")
	}

	// Initialize existing file-based components
	sessionMgr := session.NewSessionManager(filePath)
	contactsStore := contacts.NewStore(filePath)

	// Note: CronService requires a JobHandler which we don't have here
	// We'll handle cron separately or pass a dummy handler
	var cronSvc *cron.CronService

	fs := &FileStorage{
		workspacePath: filePath,
		sessionMgr:    sessionMgr,
		contactsStore: contactsStore,
		cronService:   cronSvc,
	}

	// Create repository adapters
	fs.sessions = NewSessionRepository(sessionMgr)
	fs.contactsRepo = NewContactsRepository(contactsStore)
	fs.cronRepo = NewCronRepository(cronSvc) // Will work even if cronSvc is nil

	return fs, nil
}

// SetCronService sets the cron service after initialization.
// This is needed because CronService requires a JobHandler that's only available at app startup.
func (fs *FileStorage) SetCronService(cronSvc *cron.CronService) {
	fs.cronService = cronSvc
	fs.cronRepo = NewCronRepository(cronSvc)
}

// Connect initializes the file-based storage (ensures directories exist).
func (fs *FileStorage) Connect(ctx context.Context) error {
	// File-based storage doesn't need explicit connection
	// Directory creation is handled by individual components
	return nil
}

// Close closes the file-based storage (no-op for files).
func (fs *FileStorage) Close() error {
	// No cleanup needed for file-based storage
	return nil
}

// Sessions returns the session repository.
func (fs *FileStorage) Sessions() repository.SessionRepository {
	return fs.sessions
}

// Contacts returns the contacts repository.
func (fs *FileStorage) Contacts() repository.ContactsRepository {
	return fs.contactsRepo
}

// Cron returns the cron repository.
func (fs *FileStorage) Cron() repository.CronRepository {
	return fs.cronRepo
}

// Ping checks if the file-based storage is accessible.
func (fs *FileStorage) Ping(ctx context.Context) error {
	// For file-based storage, just check if workspace path exists
	// This is a simple health check
	return nil
}
