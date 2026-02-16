package channels

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/tencent-connect/botgo"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/event"
	"github.com/tencent-connect/botgo/openapi"
	"github.com/tencent-connect/botgo/token"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

type QQChannel struct {
	*BaseChannel
	config         config.QQConfig
	api            openapi.OpenAPI
	token          *token.Token
	ctx            context.Context
	cancel         context.CancelFunc
	sessionManager botgo.SessionManager
	processedIDs   map[string]bool
	mu             sync.RWMutex
}

func NewQQChannel(cfg config.QQConfig, messageBus *bus.MessageBus) (*QQChannel, error) {
	base := NewBaseChannel("qq", cfg, messageBus, cfg.AllowFrom)

	return &QQChannel{
		BaseChannel:  base,
		config:       cfg,
		processedIDs: make(map[string]bool),
	}, nil
}

func (c *QQChannel) Start(ctx context.Context) error {
	if c.config.AppID == "" || c.config.AppSecret == "" {
		return fmt.Errorf("QQ app_id and app_secret not configured")
	}

	logger.InfoC("qq", "Starting QQ bot (WebSocket mode)")

	// Parse AppID as uint64
	appID, err := strconv.ParseUint(c.config.AppID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid app_id %q: %w", c.config.AppID, err)
	}

	// Create bot token (v0.2.0 API)
	c.token = token.BotToken(appID, c.config.AppSecret)

	// Create child context
	c.ctx, c.cancel = context.WithCancel(ctx)

	// Initialize OpenAPI client
	c.api = botgo.NewOpenAPI(c.token).WithTimeout(5 * time.Second)

	// Register event handlers (v0.2.0: ATMessage for guild @bot, DirectMessage for DMs)
	intent := event.RegisterHandlers(
		c.handleATMessage(),
		c.handleDirectMessage(),
	)

	// Get WebSocket access point
	wsInfo, err := c.api.WS(c.ctx, nil, "")
	if err != nil {
		return fmt.Errorf("failed to get websocket info: %w", err)
	}

	logger.InfoCF("qq", "Got WebSocket info", map[string]interface{}{
		"shards": wsInfo.Shards,
	})

	// Create and save sessionManager
	c.sessionManager = botgo.NewSessionManager()

	// Start WebSocket connection in goroutine to avoid blocking
	go func() {
		if err := c.sessionManager.Start(wsInfo, c.token, &intent); err != nil {
			logger.ErrorCF("qq", "WebSocket session error", map[string]interface{}{
				"error": err.Error(),
			})
			c.setRunning(false)
		}
	}()

	c.setRunning(true)
	logger.InfoC("qq", "QQ bot started successfully")

	return nil
}

func (c *QQChannel) Stop(ctx context.Context) error {
	logger.InfoC("qq", "Stopping QQ bot")
	c.setRunning(false)

	if c.cancel != nil {
		c.cancel()
	}

	return nil
}

func (c *QQChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("QQ bot not running")
	}

	// Build message
	msgToCreate := &dto.MessageToCreate{
		Content: msg.Content,
	}

	// Send message to channel (v0.2.0: PostMessage uses channelID)
	_, err := c.api.PostMessage(ctx, msg.ChatID, msgToCreate)
	if err != nil {
		logger.ErrorCF("qq", "Failed to send message", map[string]interface{}{
			"error": err.Error(),
		})
		return err
	}

	return nil
}

// handleATMessage handles guild @bot messages (v0.2.0 API)
func (c *QQChannel) handleATMessage() event.ATMessageEventHandler {
	return func(event *dto.WSPayload, data *dto.WSATMessageData) error {
		msg := (*dto.Message)(data)

		// Dedup check
		if c.isDuplicate(msg.ID) {
			return nil
		}

		// Extract user info
		var senderID string
		if msg.Author != nil && msg.Author.ID != "" {
			senderID = msg.Author.ID
		} else {
			logger.WarnC("qq", "Received AT message with no sender ID")
			return nil
		}

		// Extract message content
		content := msg.Content
		if content == "" {
			logger.DebugC("qq", "Received empty AT message, ignoring")
			return nil
		}

		logger.InfoCF("qq", "Received AT message", map[string]interface{}{
			"sender":  senderID,
			"channel": msg.ChannelID,
			"guild":   msg.GuildID,
			"length":  len(content),
		})

		// Forward to message bus (use ChannelID as ChatID for replies)
		metadata := map[string]string{
			"message_id": msg.ID,
			"channel_id": msg.ChannelID,
			"guild_id":   msg.GuildID,
		}

		c.HandleMessage(senderID, msg.ChannelID, content, []string{}, metadata)

		return nil
	}
}

// handleDirectMessage handles direct (private) messages (v0.2.0 API)
func (c *QQChannel) handleDirectMessage() event.DirectMessageEventHandler {
	return func(event *dto.WSPayload, data *dto.WSDirectMessageData) error {
		msg := (*dto.Message)(data)

		// Dedup check
		if c.isDuplicate(msg.ID) {
			return nil
		}

		// Extract user info
		var senderID string
		if msg.Author != nil && msg.Author.ID != "" {
			senderID = msg.Author.ID
		} else {
			logger.WarnC("qq", "Received direct message with no sender ID")
			return nil
		}

		// Extract message content
		content := msg.Content
		if content == "" {
			logger.DebugC("qq", "Received empty direct message, ignoring")
			return nil
		}

		logger.InfoCF("qq", "Received direct message", map[string]interface{}{
			"sender": senderID,
			"guild":  msg.GuildID,
			"length": len(content),
		})

		// Forward to message bus (use GuildID as ChatID for DM replies)
		metadata := map[string]string{
			"message_id": msg.ID,
			"guild_id":   msg.GuildID,
		}

		c.HandleMessage(senderID, msg.GuildID, content, []string{}, metadata)

		return nil
	}
}

// isDuplicate checks if a message has already been processed
func (c *QQChannel) isDuplicate(messageID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.processedIDs[messageID] {
		return true
	}

	c.processedIDs[messageID] = true

	// Simple cleanup: limit map size
	if len(c.processedIDs) > 10000 {
		// Clear half
		count := 0
		for id := range c.processedIDs {
			if count >= 5000 {
				break
			}
			delete(c.processedIDs, id)
			count++
		}
	}

	return false
}
