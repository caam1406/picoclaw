package file

import (
	"context"

	"github.com/sipeed/picoclaw/pkg/contacts"
	"github.com/sipeed/picoclaw/pkg/storage/repository"
)

type contactsRepository struct {
	store *contacts.Store
}

// NewContactsRepository creates a new file-based contacts repository adapter.
func NewContactsRepository(store *contacts.Store) repository.ContactsRepository {
	return &contactsRepository{store: store}
}

func (r *contactsRepository) Get(ctx context.Context, channel, contactID string) (*repository.ContactInstruction, error) {
	ci := r.store.Get(channel, contactID)
	if ci == nil {
		return nil, nil // Return nil instead of error when not found
	}
	return convertToRepoContact(ci), nil
}

func (r *contactsRepository) Set(ctx context.Context, ci repository.ContactInstruction) error {
	fileCI := convertToFileContact(&ci)
	return r.store.Set(*fileCI)
}

func (r *contactsRepository) Delete(ctx context.Context, channel, contactID string) error {
	return r.store.Delete(channel, contactID)
}

func (r *contactsRepository) List(ctx context.Context) ([]repository.ContactInstruction, error) {
	fileContacts := r.store.List()

	repoContacts := make([]repository.ContactInstruction, len(fileContacts))
	for i, fc := range fileContacts {
		repoContacts[i] = *convertToRepoContact(&fc)
	}

	return repoContacts, nil
}

func (r *contactsRepository) GetForSession(ctx context.Context, sessionKey string) (string, error) {
	return r.store.GetForSession(sessionKey), nil
}

func (r *contactsRepository) IsRegistered(ctx context.Context, sessionKey string) (bool, error) {
	return r.store.IsRegistered(sessionKey), nil
}

func (r *contactsRepository) Count(ctx context.Context) (int, error) {
	contacts := r.store.List()
	return len(contacts), nil
}

// Helper functions to convert between contact types

func convertToRepoContact(fileCI *contacts.ContactInstruction) *repository.ContactInstruction {
	if fileCI == nil {
		return nil
	}

	return &repository.ContactInstruction{
		ContactID:    fileCI.ContactID,
		Channel:      fileCI.Channel,
		DisplayName:  fileCI.DisplayName,
		Instructions: fileCI.Instructions,
		CreatedAt:    fileCI.CreatedAt,
		UpdatedAt:    fileCI.UpdatedAt,
	}
}

func convertToFileContact(repoCI *repository.ContactInstruction) *contacts.ContactInstruction {
	if repoCI == nil {
		return nil
	}

	return &contacts.ContactInstruction{
		ContactID:    repoCI.ContactID,
		Channel:      repoCI.Channel,
		DisplayName:  repoCI.DisplayName,
		Instructions: repoCI.Instructions,
		CreatedAt:    repoCI.CreatedAt,
		UpdatedAt:    repoCI.UpdatedAt,
	}
}
