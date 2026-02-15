package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/contacts"
	"github.com/sipeed/picoclaw/pkg/logger"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins (auth is via token)
	},
}

// Client represents a single WebSocket client connection.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// Hub manages all WebSocket clients and broadcasts bus events.
type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	msgBus     *bus.MessageBus
	contacts   *contacts.Store
	mu         sync.RWMutex
	latestQR   *bus.QRCodeEvent
	qrMu       sync.RWMutex
}

func NewHub(msgBus *bus.MessageBus, contactsStore *contacts.Store) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte, 256),
		msgBus:     msgBus,
		contacts:   contactsStore,
	}
}

func (h *Hub) Run(ctx context.Context) {
	// Subscribe to bus events
	events := h.msgBus.Subscribe()
	defer h.msgBus.Unsubscribe(events)

	for {
		select {
		case <-ctx.Done():
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			logger.DebugC("dashboard", "WebSocket client connected")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			logger.DebugC("dashboard", "WebSocket client disconnected")

		case event := <-events:
			if event.Type == "inbound" && event.Inbound != nil {
				enriched := *event.Inbound
				sender := h.resolveSenderLabel(&enriched)
				if sender != "" {
					if enriched.Metadata == nil {
						enriched.Metadata = map[string]string{}
					}
					enriched.Metadata["sender_display"] = sender

					// Backward compatibility: old frontend versions only render content.
					prefix := fmt.Sprintf("[%s] ", sender)
					if !strings.HasPrefix(enriched.Content, prefix) {
						enriched.Content = prefix + enriched.Content
					}
				}
				event.Inbound = &enriched
			}

			// Cache latest QR state for late-joining clients
			if event.Type == "qr_code" && event.QRCode != nil {
				h.qrMu.Lock()
				switch event.QRCode.Event {
				case "success", "timeout", "error":
					h.latestQR = nil
				default:
					h.latestQR = event.QRCode
				}
				h.qrMu.Unlock()
			}

			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- data:
				default:
					// Client buffer full, will be cleaned up
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) resolveSenderLabel(in *bus.InboundMessage) string {
	channel := strings.ToLower(strings.TrimSpace(in.Channel))
	metadata := in.Metadata

	idCandidates := []string{
		strings.TrimSpace(in.SenderID),
		strings.TrimSpace(metadata["sender_phone"]),
		strings.TrimSpace(metadata["sender_id"]),
		strings.TrimSpace(metadata["sender_jid"]),
		strings.TrimSpace(in.ChatID),
		strings.TrimSpace(metadata["chat_id"]),
		strings.TrimSpace(metadata["chat_jid"]),
	}

	if h.contacts != nil {
		for _, id := range idCandidates {
			if name := h.lookupContactName(channel, id); name != "" {
				return name
			}
		}
	}

	nameCandidates := []string{
		strings.TrimSpace(metadata["display_name"]),
		strings.TrimSpace(metadata["sender_name"]),
		strings.TrimSpace(metadata["push_name"]),
		strings.TrimSpace(metadata["first_name"]),
		strings.TrimSpace(metadata["username"]),
	}
	for _, n := range nameCandidates {
		if n != "" {
			return n
		}
	}

	for _, id := range idCandidates {
		if id == "" {
			continue
		}
		if channel == "whatsapp" {
			return normalizeWhatsAppID(id)
		}
		return id
	}
	return ""
}

func (h *Hub) lookupContactName(channel, id string) string {
	id = strings.TrimSpace(id)
	if id == "" || h.contacts == nil {
		return ""
	}

	tryIDs := []string{id}
	if channel == "whatsapp" {
		norm := normalizeWhatsAppID(id)
		if norm != "" && norm != id {
			tryIDs = append(tryIDs, norm, norm+"@s.whatsapp.net")
		}
	}

	for _, candidate := range tryIDs {
		ci := h.contacts.Get(channel, candidate)
		if ci != nil && strings.TrimSpace(ci.DisplayName) != "" {
			return strings.TrimSpace(ci.DisplayName)
		}
	}
	return ""
}

func normalizeWhatsAppID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if idx := strings.Index(id, "@"); idx >= 0 {
		return id[:idx]
	}
	return id
}

// GetLatestQR returns the latest pending QR code event, or nil if none.
func (h *Hub) GetLatestQR() *bus.QRCodeEvent {
	h.qrMu.RLock()
	defer h.qrMu.RUnlock()
	return h.latestQR
}

func (h *Hub) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.ErrorCF("dashboard", "WebSocket upgrade failed", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	client := &Client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 256),
	}

	h.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
