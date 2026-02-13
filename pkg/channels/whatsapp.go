package channels

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/voice"
)

// WhatsAppChannel implements native WhatsApp Web communication via whatsmeow.
type WhatsAppChannel struct {
	*BaseChannel
	client      *whatsmeow.Client
	config      config.WhatsAppConfig
	container   *sqlstore.Container
	transcriber *voice.GroqTranscriber
	mu          sync.Mutex
	cancelFunc  context.CancelFunc
}

// NewWhatsAppChannel creates a new WhatsApp channel instance.
func NewWhatsAppChannel(cfg config.WhatsAppConfig, msgBus *bus.MessageBus) (*WhatsAppChannel, error) {
	base := NewBaseChannel("whatsapp", cfg, msgBus, cfg.AllowFrom)

	return &WhatsAppChannel{
		BaseChannel: base,
		config:      cfg,
	}, nil
}

// SetTranscriber injects an optional Groq voice transcriber.
func (c *WhatsAppChannel) SetTranscriber(transcriber *voice.GroqTranscriber) {
	c.transcriber = transcriber
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Start initializes the SQLite store, creates the whatsmeow client, and
// connects to WhatsApp. If no existing session is found it triggers QR code
// login.
func (c *WhatsAppChannel) Start(ctx context.Context) error {
	logger.InfoC("whatsapp", "Starting WhatsApp channel (native whatsmeow)")

	// 1. Resolve and ensure store directory
	storePath := c.resolveStorePath()
	if err := os.MkdirAll(filepath.Dir(storePath), 0755); err != nil {
		return fmt.Errorf("failed to create store directory: %w", err)
	}

	// 2. Initialise SQLite container with serialized access
	dbLog := waLog.Stdout("WhatsApp-DB", "WARN", true)
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)", storePath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("failed to open whatsmeow database: %w", err)
	}
	// Serialize all database access through a single connection to prevent SQLITE_BUSY
	db.SetMaxOpenConns(1)

	container := sqlstore.NewWithDB(db, "sqlite", dbLog)
	if err := container.Upgrade(ctx); err != nil {
		return fmt.Errorf("failed to upgrade whatsmeow database: %w", err)
	}
	c.container = container

	// 3. Get or create device
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("failed to get device from store: %w", err)
	}

	// 4. Create client
	clientLog := waLog.Stdout("WhatsApp", "WARN", true)
	c.client = whatsmeow.NewClient(deviceStore, clientLog)
	c.client.AddEventHandler(c.eventHandler)

	// 5. Connect – QR login if new device
	if c.client.Store.ID == nil {
		logger.InfoC("whatsapp", "No existing session found – starting QR code login")
		if err := c.loginWithQR(ctx); err != nil {
			return fmt.Errorf("QR login failed: %w", err)
		}
	} else {
		logger.InfoCF("whatsapp", "Resuming existing session", map[string]interface{}{
			"device_id": c.client.Store.ID.String(),
		})
		if err := c.client.Connect(); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
	}

	c.setRunning(true)

	// 6. Start reconnection monitor
	reconnCtx, cancel := context.WithCancel(ctx)
	c.cancelFunc = cancel
	go c.reconnectLoop(reconnCtx)

	logger.InfoC("whatsapp", "WhatsApp channel started successfully")
	return nil
}

// Stop disconnects from WhatsApp and releases resources.
func (c *WhatsAppChannel) Stop(ctx context.Context) error {
	logger.InfoC("whatsapp", "Stopping WhatsApp channel")

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancelFunc != nil {
		c.cancelFunc()
		c.cancelFunc = nil
	}

	if c.client != nil {
		c.client.Disconnect()
		c.client = nil
	}

	c.container = nil
	c.setRunning(false)

	logger.InfoC("whatsapp", "WhatsApp channel stopped")
	return nil
}

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

// loginWithQR performs the interactive QR-code pairing flow.
func (c *WhatsAppChannel) loginWithQR(ctx context.Context) error {
	qrChan, err := c.client.GetQRChannel(ctx)
	if err != nil {
		return fmt.Errorf("failed to get QR channel: %w", err)
	}

	if err := c.client.Connect(); err != nil {
		return fmt.Errorf("failed to connect for QR: %w", err)
	}

	for evt := range qrChan {
		switch evt.Event {
		case "code":
			fmt.Println("\n--- Scan this QR code with WhatsApp (Linked Devices) ---")
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			fmt.Println("--- Waiting for scan... ---")
			logger.InfoC("whatsapp", "QR code displayed – waiting for scan")

		case "login", "success":
			devID := "unknown"
			if c.client.Store.ID != nil {
				devID = c.client.Store.ID.String()
			}
			logger.InfoCF("whatsapp", "WhatsApp login successful", map[string]interface{}{
				"device_id": devID,
				"event":     evt.Event,
			})
			return nil

		case "timeout":
			logger.WarnC("whatsapp", "QR code timed out")
			return fmt.Errorf("QR code login timed out – restart to try again")

		case "error":
			logger.ErrorC("whatsapp", "QR login error")
			return fmt.Errorf("QR login error")
		}
	}

	// Channel closed – check if we're actually connected (race with event handler)
	if c.client.IsConnected() || c.client.Store.ID != nil {
		logger.InfoC("whatsapp", "QR channel closed but client is connected – login OK")
		return nil
	}

	return fmt.Errorf("QR channel closed unexpectedly")
}

// resolveStorePath expands ~ in the configured store path.
func (c *WhatsAppChannel) resolveStorePath() string {
	path := c.config.StorePath
	if path == "" {
		path = "~/.picoclaw/whatsapp.db"
	}
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		if len(path) > 1 && (path[1] == '/' || path[1] == '\\') {
			path = home + path[1:]
		} else {
			path = home
		}
	}
	return path
}

// ---------------------------------------------------------------------------
// Event handling
// ---------------------------------------------------------------------------

// eventHandler is the central whatsmeow event dispatcher.
func (c *WhatsAppChannel) eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		c.handleIncomingMessage(v)
	case *events.Connected:
		logger.InfoC("whatsapp", "WhatsApp connected")
	case *events.Disconnected:
		logger.WarnC("whatsapp", "WhatsApp disconnected")
	case *events.LoggedOut:
		logger.ErrorCF("whatsapp", "WhatsApp logged out", map[string]interface{}{
			"reason": fmt.Sprintf("%v", v.Reason),
		})
		c.setRunning(false)
	case *events.HistorySync:
		// Ignore history syncs – we only process real-time messages
	}
}

// ---------------------------------------------------------------------------
// Inbound messages
// ---------------------------------------------------------------------------

// handleIncomingMessage processes a single incoming WhatsApp message.
func (c *WhatsAppChannel) handleIncomingMessage(evt *events.Message) {
	// Skip own messages and broadcasts
	if evt.Info.IsFromMe {
		return
	}
	if evt.Info.Chat.Server == "broadcast" {
		return
	}

	senderID := evt.Info.Sender.User
	chatID := evt.Info.Chat.String()

	// Extract text
	content := c.extractTextContent(evt.Message)

	// Extract and download media
	mediaPaths := c.extractMedia(evt)

	// Voice transcription
	content = c.handleVoiceTranscription(evt, content, mediaPaths)

	if content == "" && len(mediaPaths) == 0 {
		return
	}
	if content == "" {
		content = "[media]"
	}

	logger.InfoCF("whatsapp", "Message received", map[string]interface{}{
		"sender":  senderID,
		"chat":    chatID,
		"preview": truncateString(content, 50),
	})

	metadata := map[string]string{
		"message_id":   evt.Info.ID,
		"sender_jid":   evt.Info.Sender.String(),
		"chat_jid":     evt.Info.Chat.String(),
		"sender_phone": senderID,
		"is_group":     fmt.Sprintf("%t", evt.Info.Chat.Server == types.GroupServer),
		"timestamp":    evt.Info.Timestamp.Format(time.RFC3339),
	}
	if evt.Info.PushName != "" {
		metadata["push_name"] = evt.Info.PushName
	}

	c.HandleMessage(senderID, chatID, content, mediaPaths, metadata)
}

// extractTextContent returns the plain-text body from a WhatsApp message.
func (c *WhatsAppChannel) extractTextContent(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if t := msg.GetConversation(); t != "" {
		return t
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	if img := msg.GetImageMessage(); img != nil {
		return img.GetCaption()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid.GetCaption()
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		return doc.GetCaption()
	}
	return ""
}

// ---------------------------------------------------------------------------
// Media handling
// ---------------------------------------------------------------------------

// extractMedia downloads any media attached to the message.
func (c *WhatsAppChannel) extractMedia(evt *events.Message) []string {
	msg := evt.Message
	if msg == nil {
		return nil
	}

	var paths []string

	if img := msg.GetImageMessage(); img != nil {
		if p := c.downloadMedia(img, ".jpg"); p != "" {
			paths = append(paths, p)
		}
	}
	if audio := msg.GetAudioMessage(); audio != nil {
		ext := ".ogg"
		if audio.GetMimetype() == "audio/mp4" {
			ext = ".m4a"
		}
		if p := c.downloadMedia(audio, ext); p != "" {
			paths = append(paths, p)
		}
	}
	if video := msg.GetVideoMessage(); video != nil {
		if p := c.downloadMedia(video, ".mp4"); p != "" {
			paths = append(paths, p)
		}
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		ext := filepath.Ext(doc.GetFileName())
		if ext == "" {
			ext = ".bin"
		}
		if p := c.downloadMedia(doc, ext); p != "" {
			paths = append(paths, p)
		}
	}
	if sticker := msg.GetStickerMessage(); sticker != nil {
		if p := c.downloadMedia(sticker, ".webp"); p != "" {
			paths = append(paths, p)
		}
	}

	return paths
}

// downloadMedia fetches encrypted media via whatsmeow and stores it locally.
func (c *WhatsAppChannel) downloadMedia(msg whatsmeow.DownloadableMessage, ext string) string {
	data, err := c.client.Download(context.Background(), msg)
	if err != nil {
		logger.ErrorCF("whatsapp", "Failed to download media", map[string]interface{}{
			"error": err.Error(),
		})
		return ""
	}

	mediaDir := filepath.Join(os.TempDir(), "picoclaw_media")
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		logger.ErrorCF("whatsapp", "Failed to create media directory", map[string]interface{}{
			"error": err.Error(),
		})
		return ""
	}

	filename := fmt.Sprintf("wa_%d%s", time.Now().UnixNano(), ext)
	localPath := filepath.Join(mediaDir, filename)

	if err := os.WriteFile(localPath, data, 0644); err != nil {
		logger.ErrorCF("whatsapp", "Failed to write media file", map[string]interface{}{
			"error": err.Error(),
			"path":  localPath,
		})
		return ""
	}

	logger.DebugCF("whatsapp", "Media downloaded", map[string]interface{}{
		"path": localPath,
		"size": len(data),
	})
	return localPath
}

// ---------------------------------------------------------------------------
// Voice transcription
// ---------------------------------------------------------------------------

// handleVoiceTranscription transcribes voice notes (PTT) via the Groq
// transcriber when available.
func (c *WhatsAppChannel) handleVoiceTranscription(evt *events.Message, content string, mediaPaths []string) string {
	msg := evt.Message
	if msg == nil {
		return content
	}

	audio := msg.GetAudioMessage()
	if audio == nil {
		return content
	}

	voicePath := ""
	if len(mediaPaths) > 0 {
		voicePath = mediaPaths[len(mediaPaths)-1]
	}
	if voicePath == "" {
		return content
	}

	// Only transcribe push-to-talk voice notes
	if !audio.GetPTT() {
		return appendLine(content, fmt.Sprintf("[audio: %s]", voicePath))
	}

	transcribed := fmt.Sprintf("[voice: %s]", voicePath)
	if c.transcriber != nil && c.transcriber.IsAvailable() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := c.transcriber.Transcribe(ctx, voicePath)
		if err != nil {
			logger.ErrorCF("whatsapp", "Voice transcription failed", map[string]interface{}{
				"error": err.Error(),
			})
			transcribed = fmt.Sprintf("[voice: %s (transcription failed)]", voicePath)
		} else {
			transcribed = fmt.Sprintf("[voice transcription: %s]", result.Text)
			logger.InfoCF("whatsapp", "Voice transcribed", map[string]interface{}{
				"text": truncateString(result.Text, 50),
			})
		}
	}

	return appendLine(content, transcribed)
}

// appendLine joins two strings with a newline, skipping empty parts.
func appendLine(base, extra string) string {
	if base == "" {
		return extra
	}
	return base + "\n" + extra
}

// ---------------------------------------------------------------------------
// Outbound messages
// ---------------------------------------------------------------------------

// Send delivers a text message to the specified WhatsApp chat.
func (c *WhatsAppChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil || !c.client.IsConnected() {
		return fmt.Errorf("whatsapp client not connected")
	}

	targetJID, err := types.ParseJID(msg.ChatID)
	if err != nil {
		return fmt.Errorf("invalid chat ID '%s': %w", msg.ChatID, err)
	}

	// Typing indicator
	_ = c.client.SendChatPresence(ctx, targetJID, types.ChatPresenceComposing, "")

	waMsg := &waE2E.Message{
		Conversation: proto.String(msg.Content),
	}

	resp, err := c.client.SendMessage(ctx, targetJID, waMsg)
	if err != nil {
		return fmt.Errorf("failed to send whatsapp message: %w", err)
	}

	// Clear typing indicator
	_ = c.client.SendChatPresence(ctx, targetJID, types.ChatPresencePaused, "")

	logger.DebugCF("whatsapp", "Message sent", map[string]interface{}{
		"to":         targetJID.String(),
		"message_id": resp.ID,
	})

	return nil
}

// ---------------------------------------------------------------------------
// Reconnection
// ---------------------------------------------------------------------------

// reconnectLoop monitors the connection and retries with exponential backoff.
func (c *WhatsAppChannel) reconnectLoop(ctx context.Context) {
	backoff := 5 * time.Second
	maxBackoff := 5 * time.Minute

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
			if c.client == nil {
				return
			}
			if !c.client.IsConnected() && c.client.IsLoggedIn() {
				logger.WarnCF("whatsapp", "Connection lost – attempting reconnect", map[string]interface{}{
					"backoff_seconds": backoff.Seconds(),
				})

				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}

				if err := c.client.Connect(); err != nil {
					logger.ErrorCF("whatsapp", "Reconnection failed", map[string]interface{}{
						"error":   err.Error(),
						"backoff": backoff.String(),
					})
					backoff = backoff * 2
					if backoff > maxBackoff {
						backoff = maxBackoff
					}
				} else {
					logger.InfoC("whatsapp", "Reconnected successfully")
					backoff = 5 * time.Second
				}
			}
		}
	}
}
