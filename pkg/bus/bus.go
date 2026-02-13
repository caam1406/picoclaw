package bus

import (
	"context"
	"sync"
	"time"
)

// BusEvent represents an observed message event for dashboard streaming.
type BusEvent struct {
	Type     string           `json:"type"` // "inbound", "outbound", or "qr_code"
	Inbound  *InboundMessage  `json:"inbound,omitempty"`
	Outbound *OutboundMessage `json:"outbound,omitempty"`
	QRCode   *QRCodeEvent     `json:"qr_code,omitempty"`
	Time     time.Time        `json:"time"`
}

type MessageBus struct {
	inbound   chan InboundMessage
	outbound  chan OutboundMessage
	handlers  map[string]MessageHandler
	mu        sync.RWMutex
	observers []chan BusEvent
	obsMu     sync.RWMutex
}

func NewMessageBus() *MessageBus {
	return &MessageBus{
		inbound:   make(chan InboundMessage, 100),
		outbound:  make(chan OutboundMessage, 100),
		handlers:  make(map[string]MessageHandler),
		observers: make([]chan BusEvent, 0),
	}
}

// Subscribe returns a channel that receives copies of all bus events.
func (mb *MessageBus) Subscribe() chan BusEvent {
	ch := make(chan BusEvent, 50)
	mb.obsMu.Lock()
	mb.observers = append(mb.observers, ch)
	mb.obsMu.Unlock()
	return ch
}

// Unsubscribe removes an observer channel.
func (mb *MessageBus) Unsubscribe(ch chan BusEvent) {
	mb.obsMu.Lock()
	defer mb.obsMu.Unlock()
	for i, obs := range mb.observers {
		if obs == ch {
			mb.observers = append(mb.observers[:i], mb.observers[i+1:]...)
			close(ch)
			return
		}
	}
}

func (mb *MessageBus) notifyObservers(event BusEvent) {
	mb.obsMu.RLock()
	defer mb.obsMu.RUnlock()
	for _, obs := range mb.observers {
		select {
		case obs <- event:
		default:
			// Non-blocking: skip slow observers
		}
	}
}

func (mb *MessageBus) PublishInbound(msg InboundMessage) {
	mb.inbound <- msg
	mb.notifyObservers(BusEvent{
		Type:    "inbound",
		Inbound: &msg,
		Time:    time.Now(),
	})
}

func (mb *MessageBus) ConsumeInbound(ctx context.Context) (InboundMessage, bool) {
	select {
	case msg := <-mb.inbound:
		return msg, true
	case <-ctx.Done():
		return InboundMessage{}, false
	}
}

func (mb *MessageBus) PublishOutbound(msg OutboundMessage) {
	mb.outbound <- msg
	mb.notifyObservers(BusEvent{
		Type:     "outbound",
		Outbound: &msg,
		Time:     time.Now(),
	})
}

func (mb *MessageBus) PublishQRCode(event QRCodeEvent) {
	mb.notifyObservers(BusEvent{
		Type:   "qr_code",
		QRCode: &event,
		Time:   time.Now(),
	})
}

func (mb *MessageBus) SubscribeOutbound(ctx context.Context) (OutboundMessage, bool) {
	select {
	case msg := <-mb.outbound:
		return msg, true
	case <-ctx.Done():
		return OutboundMessage{}, false
	}
}

func (mb *MessageBus) RegisterHandler(channel string, handler MessageHandler) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.handlers[channel] = handler
}

func (mb *MessageBus) GetHandler(channel string) (MessageHandler, bool) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	handler, ok := mb.handlers[channel]
	return handler, ok
}

func (mb *MessageBus) Close() {
	close(mb.inbound)
	close(mb.outbound)
}
