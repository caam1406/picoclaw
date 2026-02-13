package bus

type InboundMessage struct {
	Channel    string            `json:"channel"`
	SenderID   string            `json:"sender_id"`
	ChatID     string            `json:"chat_id"`
	Content    string            `json:"content"`
	Media      []string          `json:"media,omitempty"`
	SessionKey string            `json:"session_key"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type OutboundMessage struct {
	Channel string `json:"channel"`
	ChatID  string `json:"chat_id"`
	Content string `json:"content"`
}

// QRCodeEvent represents a QR code authentication event from a channel.
type QRCodeEvent struct {
	Channel string `json:"channel"`        // e.g. "whatsapp"
	Event   string `json:"event"`          // "code", "success", "timeout", "error"
	Code    string `json:"code,omitempty"` // raw QR data string (only for "code" event)
	SVG     string `json:"svg,omitempty"`  // server-rendered SVG of the QR code
}

type MessageHandler func(InboundMessage) error
