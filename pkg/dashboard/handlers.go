package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/contacts"
	"github.com/sipeed/picoclaw/pkg/storage"
)

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	status := map[string]interface{}{
		"version":  "0.1.0",
		"uptime":   time.Since(s.startTime).String(),
		"channels": s.channelManager.GetStatus(),
	}

	writeJSON(w, status)
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, s.channelManager.GetStatus())
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	sessions := s.sessions.ListSessions()
	writeJSON(w, sessions)
}

func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Extract session key from path: /api/v1/sessions/{key}
	key := strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/")
	if key == "" {
		http.Error(w, `{"error":"session key required"}`, http.StatusBadRequest)
		return
	}

	sess := s.sessions.GetSession(key)
	if sess == nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}

	writeJSON(w, sess)
}

func (s *Server) handleContacts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		contacts := s.contactsStore.List()
		writeJSON(w, contacts)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleContactDetail(w http.ResponseWriter, r *http.Request) {
	// Extract channel and id from path: /api/v1/contacts/{channel}/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/contacts/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, `{"error":"path must be /api/v1/contacts/{channel}/{id}"}`, http.StatusBadRequest)
		return
	}
	channel := parts[0]
	contactID := parts[1]

	switch r.Method {
	case http.MethodGet:
		ci := s.contactsStore.Get(channel, contactID)
		if ci == nil {
			http.Error(w, `{"error":"contact not found"}`, http.StatusNotFound)
			return
		}
		writeJSON(w, ci)

	case http.MethodPut:
		var body struct {
			DisplayName  string `json:"display_name"`
			Instructions string `json:"instructions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
			return
		}

		ci := contacts.ContactInstruction{
			ContactID:    contactID,
			Channel:      channel,
			DisplayName:  body.DisplayName,
			Instructions: body.Instructions,
		}

		if err := s.contactsStore.Set(ci); err != nil {
			http.Error(w, `{"error":"failed to save contact instruction"}`, http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]string{"status": "ok"})

	case http.MethodDelete:
		if err := s.contactsStore.Delete(channel, contactID); err != nil {
			http.Error(w, `{"error":"contact not found"}`, http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]string{"status": "deleted"})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDefaults(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		defaults := s.contactsStore.ListDefaults()
		writeJSON(w, defaults)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDefaultDetail(w http.ResponseWriter, r *http.Request) {
	// Extract channel from path: /api/v1/defaults/{channel}
	channel := strings.TrimPrefix(r.URL.Path, "/api/v1/defaults/")
	if channel == "" {
		http.Error(w, `{"error":"channel is required (use * for global)"}`, http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		defaults := s.contactsStore.ListDefaults()
		inst, ok := defaults[channel]
		if !ok {
			http.Error(w, `{"error":"default instruction not found"}`, http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]string{"channel": channel, "instructions": inst})

	case http.MethodPut:
		var body struct {
			Instructions string `json:"instructions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
			return
		}

		if err := s.contactsStore.SetDefault(channel, body.Instructions); err != nil {
			http.Error(w, `{"error":"failed to save default instruction"}`, http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]string{"status": "ok"})

	case http.MethodDelete:
		if err := s.contactsStore.DeleteDefault(channel); err != nil {
			http.Error(w, `{"error":"default instruction not found"}`, http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]string{"status": "deleted"})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Channel string `json:"channel"`
		ChatID  string `json:"chat_id"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	if body.Channel == "" || body.ChatID == "" || body.Content == "" {
		http.Error(w, `{"error":"channel, chat_id and content are required"}`, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := s.channelManager.SendToChannel(ctx, body.Channel, body.ChatID, body.Content); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "sent"})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Auth via query param for WebSocket
	token := r.URL.Query().Get("token")
	if token != s.config.Token {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	s.hub.handleWebSocket(w, r)
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// handleGetStorageConfig returns the current storage configuration (with password masked)
func (s *Server) handleGetStorageConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	cfg := s.cfg.Storage
	// Mask password in URL for security
	maskedURL := maskDatabaseURL(cfg.DatabaseURL)

	writeJSON(w, map[string]interface{}{
		"type":         cfg.Type,
		"database_url": maskedURL,
		"file_path":    cfg.FilePath,
		"ssl_enabled":  cfg.SSLEnabled,
	})
}

// handleUpdateStorageConfig updates the storage configuration
func (s *Server) handleUpdateStorageConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Type        string `json:"type"`
		DatabaseURL string `json:"database_url"`
		FilePath    string `json:"file_path"`
		SSLEnabled  bool   `json:"ssl_enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Validate storage type
	if body.Type != "file" && body.Type != "postgres" && body.Type != "sqlite" {
		http.Error(w, `{"error":"invalid storage type (must be: file, postgres, or sqlite)"}`, http.StatusBadRequest)
		return
	}

	// Update config
	s.cfg.Storage.Type = body.Type
	s.cfg.Storage.DatabaseURL = body.DatabaseURL
	s.cfg.Storage.FilePath = body.FilePath
	s.cfg.Storage.SSLEnabled = body.SSLEnabled

	// Save config to disk
	if err := s.cfg.Save(); err != nil {
		http.Error(w, `{"error":"failed to save config: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{
		"status":  "updated",
		"message": "Storage configuration updated. Restart required for changes to take effect.",
	})
}

// handleTestStorageConnection tests the database connection
func (s *Server) handleTestStorageConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Type        string `json:"type"`
		DatabaseURL string `json:"database_url"`
		FilePath    string `json:"file_path"`
		SSLEnabled  bool   `json:"ssl_enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Create temporary storage config for test
	testConfig := storage.Config{
		Type:         body.Type,
		DatabaseURL:  body.DatabaseURL,
		FilePath:     body.FilePath,
		SSLEnabled:   body.SSLEnabled,
		MaxIdleConns: 5,
		MaxOpenConns: 10,
		MaxLifetime:  30 * time.Minute,
	}

	// Create temporary storage for test
	testStore, err := storage.NewStorage(testConfig)
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := testStore.Connect(ctx); err != nil {
		writeJSON(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		testStore.Close()
		return
	}

	testStore.Close()

	writeJSON(w, map[string]interface{}{
		"success": true,
		"message": "Connection successful",
	})
}

// maskDatabaseURL masks the password in a database URL
func maskDatabaseURL(url string) string {
	if url == "" {
		return ""
	}

	// postgres://user:PASSWORD@host:port/db -> postgres://user:***@host:port/db
	if strings.HasPrefix(url, "postgres://") {
		parts := strings.SplitN(url, "@", 2)
		if len(parts) == 2 {
			userPass := strings.SplitN(parts[0], ":", 3)
			if len(userPass) == 3 {
				return userPass[0] + ":" + userPass[1] + ":***@" + parts[1]
			}
		}
	}

	return url
}
