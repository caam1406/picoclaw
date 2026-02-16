package agent

import (
	"context"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/contacts"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// Manager routes inbound messages to agent instances based on contact configuration.
// Each agent instance has its own workspace/session/tools runtime.
type Manager struct {
	bus           *bus.MessageBus
	contactsStore *contacts.Store
	defaultAgent  string
	agents        map[string]*AgentLoop
}

func NewManager(cfg *config.Config, msgBus *bus.MessageBus, provider providers.LLMProvider, contactsStore *contacts.Store, contactsOnly bool) *Manager {
	manager := &Manager{
		bus:           msgBus,
		contactsStore: contactsStore,
		defaultAgent:  cfg.GetDefaultAgentID(),
		agents:        make(map[string]*AgentLoop),
	}

	for _, agentID := range cfg.ListAgentIDs() {
		loop := NewAgentLoopForAgent(cfg, msgBus, provider, agentID)
		loop.SetContactsStore(contactsStore, contactsOnly)
		manager.agents[agentID] = loop
	}

	if _, ok := manager.agents[manager.defaultAgent]; !ok {
		loop := NewAgentLoopForAgent(cfg, msgBus, provider, manager.defaultAgent)
		loop.SetContactsStore(contactsStore, contactsOnly)
		manager.agents[manager.defaultAgent] = loop
	}

	logger.InfoCF("agent", "Agent manager initialized", map[string]interface{}{
		"default_agent": manager.defaultAgent,
		"agent_count":   len(manager.agents),
		"agent_ids":     cfg.ListAgentIDs(),
	})

	return manager
}

func (m *Manager) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			msg, ok := m.bus.ConsumeInbound(ctx)
			if !ok {
				continue
			}

			loop := m.resolveLoop(msg)
			response, err := loop.ProcessInbound(ctx, msg)
			if err != nil {
				response = fmt.Sprintf("Error processing message: %v", err)
			}

			if response != "" {
				m.bus.PublishOutbound(bus.OutboundMessage{
					Channel: msg.Channel,
					ChatID:  msg.ChatID,
					Content: response,
				})
			}
		}
	}
}

func (m *Manager) Stop() {}

func (m *Manager) DefaultLoop() *AgentLoop {
	return m.agents[m.defaultAgent]
}

func (m *Manager) ProcessDirectWithChannel(ctx context.Context, content, sessionKey, channel, chatID string) (string, error) {
	return m.DefaultLoop().ProcessDirectWithChannel(ctx, content, sessionKey, channel, chatID)
}

func (m *Manager) resolveLoop(msg bus.InboundMessage) *AgentLoop {
	agentID := m.defaultAgent
	if m.contactsStore != nil {
		if ci := m.contactsStore.GetContactForSession(msg.SessionKey); ci != nil && ci.AgentID != "" {
			agentID = ci.AgentID
		}
	}

	if loop, ok := m.agents[agentID]; ok {
		return loop
	}

	logger.WarnCF("agent", "Unknown contact agent_id, falling back to default agent", map[string]interface{}{
		"contact_agent_id": agentID,
		"default_agent":    m.defaultAgent,
		"session_key":      msg.SessionKey,
	})
	return m.agents[m.defaultAgent]
}
