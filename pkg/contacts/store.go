package contacts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ContactInstruction struct {
	ContactID    string    `json:"contact_id"`
	Channel      string    `json:"channel"`
	DisplayName  string    `json:"display_name"`
	Instructions string    `json:"instructions"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Store struct {
	mu           sync.RWMutex
	instructions map[string]*ContactInstruction
	filePath     string
}

func NewStore(workspace string) *Store {
	dir := filepath.Join(workspace, "contacts")
	os.MkdirAll(dir, 0755)

	s := &Store{
		instructions: make(map[string]*ContactInstruction),
		filePath:     filepath.Join(dir, "instructions.json"),
	}
	s.load()
	return s
}

func makeKey(channel, contactID string) string {
	return fmt.Sprintf("%s:%s", channel, contactID)
}

func (s *Store) Get(channel, contactID string) *ContactInstruction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.instructions[makeKey(channel, contactID)]
}

func (s *Store) Set(ci ContactInstruction) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := makeKey(ci.Channel, ci.ContactID)
	existing, ok := s.instructions[key]
	if ok {
		existing.DisplayName = ci.DisplayName
		existing.Instructions = ci.Instructions
		existing.UpdatedAt = time.Now()
	} else {
		now := time.Now()
		ci.CreatedAt = now
		ci.UpdatedAt = now
		s.instructions[key] = &ci
	}
	return s.saveLocked()
}

func (s *Store) Delete(channel, contactID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := makeKey(channel, contactID)
	if _, ok := s.instructions[key]; !ok {
		return fmt.Errorf("contact instruction not found: %s", key)
	}
	delete(s.instructions, key)
	return s.saveLocked()
}

func (s *Store) List() []ContactInstruction {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ContactInstruction, 0, len(s.instructions))
	for _, ci := range s.instructions {
		result = append(result, *ci)
	}
	return result
}

// GetForSession looks up contact instructions by session key.
// Session keys are formatted as "channel:chatID" (e.g., "whatsapp:5511982650676@s.whatsapp.net").
// It tries the full key first, then strips JID suffixes for WhatsApp compatibility.
func (s *Store) GetForSession(sessionKey string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Try exact match first
	if ci, ok := s.instructions[sessionKey]; ok {
		return ci.Instructions
	}

	// Parse channel and chatID from session key
	idx := strings.Index(sessionKey, ":")
	if idx <= 0 {
		return ""
	}
	channel := sessionKey[:idx]
	chatID := sessionKey[idx+1:]

	// Try without JID suffix (e.g., strip @s.whatsapp.net)
	if atIdx := strings.Index(chatID, "@"); atIdx > 0 {
		stripped := chatID[:atIdx]
		key := makeKey(channel, stripped)
		if ci, ok := s.instructions[key]; ok {
			return ci.Instructions
		}
	}

	return ""
}

func (s *Store) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}

	var items []ContactInstruction
	if err := json.Unmarshal(data, &items); err != nil {
		return
	}

	for i := range items {
		key := makeKey(items[i].Channel, items[i].ContactID)
		s.instructions[key] = &items[i]
	}
}

func (s *Store) saveLocked() error {
	items := make([]ContactInstruction, 0, len(s.instructions))
	for _, ci := range s.instructions {
		items = append(items, *ci)
	}

	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}
