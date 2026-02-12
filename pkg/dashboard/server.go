package dashboard

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/contacts"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/session"
)

type Server struct {
	config         config.DashboardConfig
	channelManager *channels.Manager
	sessions       *session.SessionManager
	contactsStore  *contacts.Store
	msgBus         *bus.MessageBus
	hub            *Hub
	httpServer     *http.Server
	startTime      time.Time
}

func NewServer(
	cfg config.DashboardConfig,
	channelManager *channels.Manager,
	sessions *session.SessionManager,
	contactsStore *contacts.Store,
	msgBus *bus.MessageBus,
) *Server {
	return &Server{
		config:         cfg,
		channelManager: channelManager,
		sessions:       sessions,
		contactsStore:  contactsStore,
		msgBus:         msgBus,
	}
}

func (s *Server) Start(ctx context.Context) error {
	s.startTime = time.Now()

	// Create WebSocket hub
	s.hub = NewHub(s.msgBus)
	go s.hub.Run(ctx)

	// Create HTTP mux
	mux := http.NewServeMux()

	// API routes (require auth)
	mux.HandleFunc("/api/v1/status", s.authMiddleware(s.handleStatus))
	mux.HandleFunc("/api/v1/channels", s.authMiddleware(s.handleChannels))
	mux.HandleFunc("/api/v1/sessions", s.authMiddleware(s.handleSessions))
	mux.HandleFunc("/api/v1/sessions/", s.authMiddleware(s.handleSessionDetail))
	mux.HandleFunc("/api/v1/contacts", s.authMiddleware(s.handleContacts))
	mux.HandleFunc("/api/v1/contacts/", s.authMiddleware(s.handleContactDetail))
	mux.HandleFunc("/api/v1/send", s.authMiddleware(s.handleSend))

	// WebSocket (auth via query param)
	mux.HandleFunc("/ws", s.handleWebSocket)

	// Static frontend files (no auth required for login page)
	frontendSub, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		return fmt.Errorf("failed to create frontend sub-filesystem: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(frontendSub)))

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.corsMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	go func() {
		logger.InfoCF("dashboard", "Dashboard server started", map[string]interface{}{
			"address": addr,
		})
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.ErrorCF("dashboard", "Dashboard server error", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}()

	return nil
}

func (s *Server) Stop() {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpServer.Shutdown(ctx)
		logger.InfoC("dashboard", "Dashboard server stopped")
	}
}

// authMiddleware wraps a handler with bearer token authentication.
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := s.extractToken(r)
		if token != s.config.Token {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// extractToken gets the bearer token from Authorization header.
func (s *Server) extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	// Fallback: query parameter (for WebSocket)
	return r.URL.Query().Get("token")
}

// corsMiddleware adds CORS headers for same-origin requests.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
