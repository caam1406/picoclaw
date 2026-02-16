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

type Store struct {
	mu               sync.RWMutex
	instructions     map[string]*ContactInstruction
	filePath         string
	defaults         map[string]string // key: channel name or "*" for global
	defaultsFilePath string
}

func NewStore(workspace string) *Store {
	dir := filepath.Join(workspace, "contacts")
	os.MkdirAll(dir, 0755)

	s := &Store{
		instructions:     make(map[string]*ContactInstruction),
		filePath:         filepath.Join(dir, "instructions.json"),
		defaults:         make(map[string]string),
		defaultsFilePath: filepath.Join(dir, "defaults.json"),
	}
	s.load()
	s.loadDefaults()
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
		existing.AgentID = ci.AgentID
		existing.AllowedMCPs = append([]string{}, ci.AllowedMCPs...)
		existing.Instructions = ci.Instructions
		existing.ResponseDelaySeconds = ci.ResponseDelaySeconds
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

	if ci := s.getContactForSessionLocked(sessionKey); ci != nil {
		return ci.Instructions
	}
	return ""
}

// GetContactForSession returns full contact settings by session key.
func (s *Store) GetContactForSession(sessionKey string) *ContactInstruction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getContactForSessionLocked(sessionKey)
}

// IsRegistered checks if a contact exists in the store by session key.
// Uses the same lookup logic as GetForSession (exact match + JID strip).
func (s *Store) IsRegistered(sessionKey string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Try exact match
	if s.getContactForSessionLocked(sessionKey) != nil {
		return true
	}
	return false
}

// Count returns the number of registered contacts.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.instructions)
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

// GetDefault returns the default instruction for a channel.
// It tries the specific channel first, then falls back to the global ("*") default.
func (s *Store) GetDefault(channel string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if inst, ok := s.defaults[channel]; ok {
		return inst
	}
	if inst, ok := s.defaults["*"]; ok {
		return inst
	}
	return ""
}

// SetDefault sets a default instruction for a channel (use "*" for global).
func (s *Store) SetDefault(channel, instructions string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.defaults[channel] = instructions
	return s.saveDefaultsLocked()
}

// DeleteDefault removes a default instruction for a channel.
func (s *Store) DeleteDefault(channel string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.defaults[channel]; !ok {
		return fmt.Errorf("default instruction not found for channel: %s", channel)
	}
	delete(s.defaults, channel)
	return s.saveDefaultsLocked()
}

// ListDefaults returns all default instructions.
func (s *Store) ListDefaults() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]string, len(s.defaults))
	for k, v := range s.defaults {
		result[k] = v
	}
	return result
}

func (s *Store) loadDefaults() {
	data, err := os.ReadFile(s.defaultsFilePath)
	if err != nil {
		return
	}

	json.Unmarshal(data, &s.defaults)
}

func (s *Store) saveDefaultsLocked() error {
	data, err := json.MarshalIndent(s.defaults, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.defaultsFilePath, data, 0644)
}

func (s *Store) getContactForSessionLocked(sessionKey string) *ContactInstruction {
	// Try exact match first
	if ci, ok := s.instructions[sessionKey]; ok {
		return ci
	}

	// Parse channel and chatID from session key
	idx := strings.Index(sessionKey, ":")
	if idx <= 0 {
		return nil
	}
	channel := sessionKey[:idx]
	chatID := sessionKey[idx+1:]

	// Try without JID suffix (e.g., strip @s.whatsapp.net)
	if atIdx := strings.Index(chatID, "@"); atIdx > 0 {
		stripped := chatID[:atIdx]
		key := makeKey(channel, stripped)
		if ci, ok := s.instructions[key]; ok {
			return ci
		}
	}

	return nil
}
