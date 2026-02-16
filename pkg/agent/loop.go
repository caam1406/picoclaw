// PicoClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/contacts"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/mcp"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/session"
	"github.com/sipeed/picoclaw/pkg/tools"
	"github.com/sipeed/picoclaw/pkg/utils"
)

type AgentLoop struct {
	agentID        string
	bus            *bus.MessageBus
	provider       providers.LLMProvider
	workspace      string
	model          string
	contextWindow  int // Maximum context window size in tokens
	maxIterations  int
	sessions       *session.SessionManager
	contextBuilder *ContextBuilder
	tools          *tools.ToolRegistry
	contactsStore  *contacts.Store // Per-contact gate: only registered contacts get responses
	contactsOnly   bool            // When true, only registered contacts get responses
	running        bool
	summarizing    sync.Map // Tracks which sessions are currently being summarized
	mcpServers     []config.MCPServerConfig
	mcpRuntime     *mcp.Runtime
	mcpToolsCount  int
}

// processOptions configures how a message is processed
type processOptions struct {
	SessionKey      string // Session identifier for history/context
	Channel         string // Target channel for tool execution
	ChatID          string // Target chat ID for tool execution
	UserMessage     string // User message content (may include prefix)
	DefaultResponse string // Response when LLM returns empty
	EnableSummary   bool   // Whether to trigger summarization
	SendResponse    bool   // Whether to send response via bus
}

type sessionMCPPolicy struct {
	Allowed map[string]bool
}

func NewAgentLoop(cfg *config.Config, msgBus *bus.MessageBus, provider providers.LLMProvider) *AgentLoop {
	defaultAgentID := cfg.GetDefaultAgentID()
	return NewAgentLoopForAgent(cfg, msgBus, provider, defaultAgentID)
}

func NewAgentLoopForAgent(cfg *config.Config, msgBus *bus.MessageBus, provider providers.LLMProvider, agentID string) *AgentLoop {
	resolved := cfg.ResolveAgentConfig(agentID)
	workspace := configExpandHome(resolved.Settings.Workspace)
	os.MkdirAll(workspace, 0755)

	toolsRegistry := tools.NewToolRegistry()
	toolsRegistry.Register(&tools.ReadFileTool{})
	toolsRegistry.Register(&tools.WriteFileTool{})
	toolsRegistry.Register(&tools.ListDirTool{})
	toolsRegistry.Register(tools.NewExecTool(workspace))

	braveAPIKey := cfg.Tools.Web.Search.APIKey
	toolsRegistry.Register(tools.NewWebSearchTool(braveAPIKey, cfg.Tools.Web.Search.MaxResults))
	toolsRegistry.Register(tools.NewWebFetchTool(50000))

	// Register message tool
	messageTool := tools.NewMessageTool()
	messageTool.SetSendCallback(func(channel, chatID, content string) error {
		msgBus.PublishOutbound(bus.OutboundMessage{
			Channel: channel,
			ChatID:  chatID,
			Content: content,
		})
		return nil
	})
	toolsRegistry.Register(messageTool)

	// Register spawn tool
	subagentManager := tools.NewSubagentManager(provider, workspace, msgBus)
	spawnTool := tools.NewSpawnTool(subagentManager)
	toolsRegistry.Register(spawnTool)

	// Register edit file tool
	editFileTool := tools.NewEditFileTool(workspace)
	toolsRegistry.Register(editFileTool)

	mcpRuntime := mcp.NewRuntime(resolved.AgentID, resolved.MCPServers)
	mcpRuntime.Start(context.Background())
	mcpToolsCount := 0
	for _, remote := range mcpRuntime.Tools() {
		client, ok := mcpRuntime.Client(remote.ServerName)
		if !ok || client == nil {
			continue
		}
		tool := tools.NewMCPTool(remote.ServerName, remote, client)
		if _, exists := toolsRegistry.Get(tool.Name()); exists {
			logger.WarnCF("mcp", "Skipping duplicated MCP tool name", map[string]interface{}{
				"agent_id": resolved.AgentID,
				"tool":     tool.Name(),
			})
			continue
		}
		toolsRegistry.Register(tool)
		mcpToolsCount++
	}

	sessionPath := filepath.Join(workspace, "sessions")
	if resolved.AgentID != cfg.GetDefaultAgentID() {
		sessionPath = filepath.Join(workspace, "sessions", resolved.AgentID)
	}
	sessionsManager := session.NewSessionManager(sessionPath)

	// Create context builder and set tools registry
	contextBuilder := NewContextBuilder(workspace)
	contextBuilder.SetToolsRegistry(toolsRegistry)

	return &AgentLoop{
		agentID:        resolved.AgentID,
		bus:            msgBus,
		provider:       provider,
		workspace:      workspace,
		model:          resolved.Settings.Model,
		contextWindow:  resolved.Settings.MaxTokens, // Restore context window for summarization
		maxIterations:  resolved.Settings.MaxToolIterations,
		sessions:       sessionsManager,
		contextBuilder: contextBuilder,
		tools:          toolsRegistry,
		running:        false,
		summarizing:    sync.Map{},
		mcpServers:     resolved.MCPServers,
		mcpRuntime:     mcpRuntime,
		mcpToolsCount:  mcpToolsCount,
	}
}

func (al *AgentLoop) AgentID() string {
	if al.agentID == "" {
		return "default"
	}
	return al.agentID
}

func (al *AgentLoop) Run(ctx context.Context) error {
	al.running = true

	for al.running {
		select {
		case <-ctx.Done():
			return nil
		default:
			msg, ok := al.bus.ConsumeInbound(ctx)
			if !ok {
				continue
			}

			response, err := al.processMessage(ctx, msg)
			if err != nil {
				response = fmt.Sprintf("Error processing message: %v", err)
			}

			if response != "" {
				al.publishOutboundWithContactDelay(ctx, bus.OutboundMessage{
					Channel: msg.Channel,
					ChatID:  msg.ChatID,
					Content: response,
				}, msg.SessionKey)
			}
		}
	}

	return nil
}

func (al *AgentLoop) Stop() {
	al.running = false
	if al.mcpRuntime != nil {
		al.mcpRuntime.Close()
	}
}

func (al *AgentLoop) RegisterTool(tool tools.Tool) {
	al.tools.Register(tool)
}

// SetContactsStore connects the per-contact instructions store to the context builder.
// When contactsOnly is true, only registered contacts will receive responses.
func (al *AgentLoop) SetContactsStore(store *contacts.Store, contactsOnly bool) {
	al.contactsStore = store
	al.contactsOnly = contactsOnly
	al.contextBuilder.SetContactsStore(store)
}

// GetSessionManager returns the session manager for dashboard access.
func (al *AgentLoop) GetSessionManager() *session.SessionManager {
	return al.sessions
}

func (al *AgentLoop) MCPStatusSnapshot() []map[string]interface{} {
	if al.mcpRuntime == nil {
		return []map[string]interface{}{}
	}
	statuses := al.mcpRuntime.StatusSnapshot()
	out := make([]map[string]interface{}, 0, len(statuses))
	for _, st := range statuses {
		out = append(out, map[string]interface{}{
			"server_name": st.ServerName,
			"enabled":     st.Enabled,
			"command":     st.Command,
			"connected":   st.Connected,
			"tool_count":  st.ToolCount,
			"error":       st.Error,
		})
	}
	return out
}

func (al *AgentLoop) ProcessDirect(ctx context.Context, content, sessionKey string) (string, error) {
	return al.ProcessDirectWithChannel(ctx, content, sessionKey, "cli", "direct")
}

func (al *AgentLoop) ProcessDirectWithChannel(ctx context.Context, content, sessionKey, channel, chatID string) (string, error) {
	msg := bus.InboundMessage{
		Channel:    channel,
		SenderID:   "cron",
		ChatID:     chatID,
		Content:    content,
		SessionKey: sessionKey,
	}

	return al.processMessage(ctx, msg)
}

func (al *AgentLoop) ProcessInbound(ctx context.Context, msg bus.InboundMessage) (string, error) {
	return al.processMessage(ctx, msg)
}

func configExpandHome(path string) string {
	if path == "" {
		return path
	}
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func (al *AgentLoop) processMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	// Add message preview to log
	preview := utils.Truncate(msg.Content, 80)
	logger.InfoCF("agent", fmt.Sprintf("Processing message from %s:%s: %s", msg.Channel, msg.SenderID, preview),
		map[string]interface{}{
			"channel":     msg.Channel,
			"chat_id":     msg.ChatID,
			"sender_id":   msg.SenderID,
			"session_key": msg.SessionKey,
		})

	// Route system messages to processSystemMessage
	if msg.Channel == "system" {
		return al.processSystemMessage(ctx, msg)
	}

	// Contact gate: when contacts_only is enabled, only registered contacts get responses.
	// Internal channels (cli, cron) always bypass this check.
	if al.contactsOnly && al.contactsStore != nil {
		if msg.Channel != "cli" && msg.Channel != "cron" {
			if !al.contactsStore.IsRegistered(msg.SessionKey) {
				logger.InfoCF("agent", "Message ignored: contact not registered (contacts_only=true)",
					map[string]interface{}{
						"channel":     msg.Channel,
						"sender_id":   msg.SenderID,
						"session_key": msg.SessionKey,
					})
				return "", nil
			}
		}
	}

	// Process as user message
	return al.runAgentLoop(ctx, processOptions{
		SessionKey:      msg.SessionKey,
		Channel:         msg.Channel,
		ChatID:          msg.ChatID,
		UserMessage:     msg.Content,
		DefaultResponse: "I've completed processing but have no response to give.",
		EnableSummary:   true,
		SendResponse:    false,
	})
}

func (al *AgentLoop) processSystemMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	// Verify this is a system message
	if msg.Channel != "system" {
		return "", fmt.Errorf("processSystemMessage called with non-system message channel: %s", msg.Channel)
	}

	logger.InfoCF("agent", "Processing system message",
		map[string]interface{}{
			"sender_id": msg.SenderID,
			"chat_id":   msg.ChatID,
		})

	// Parse origin from chat_id (format: "channel:chat_id")
	var originChannel, originChatID string
	if idx := strings.Index(msg.ChatID, ":"); idx > 0 {
		originChannel = msg.ChatID[:idx]
		originChatID = msg.ChatID[idx+1:]
	} else {
		// Fallback
		originChannel = "cli"
		originChatID = msg.ChatID
	}

	// Use the origin session for context
	sessionKey := fmt.Sprintf("%s:%s", originChannel, originChatID)

	// Process as system message with routing back to origin
	return al.runAgentLoop(ctx, processOptions{
		SessionKey:      sessionKey,
		Channel:         originChannel,
		ChatID:          originChatID,
		UserMessage:     fmt.Sprintf("[System: %s] %s", msg.SenderID, msg.Content),
		DefaultResponse: "Background task completed.",
		EnableSummary:   false,
		SendResponse:    true, // Send response back to original channel
	})
}

// runAgentLoop is the core message processing logic.
// It handles context building, LLM calls, tool execution, and response handling.
func (al *AgentLoop) runAgentLoop(ctx context.Context, opts processOptions) (string, error) {
	// 1. Update tool contexts
	al.updateToolContexts(opts.Channel, opts.ChatID)

	// 2. Build messages
	history := al.sessions.GetHistory(opts.SessionKey)
	summary := al.sessions.GetSummary(opts.SessionKey)
	messages := al.contextBuilder.BuildMessages(
		history,
		summary,
		opts.UserMessage,
		nil,
		opts.Channel,
		opts.ChatID,
	)
	mcpPolicy := al.getSessionMCPPolicy(opts.SessionKey)
	if len(mcpPolicy.Allowed) > 0 {
		allowedNames := make([]string, 0, len(mcpPolicy.Allowed))
		for name := range mcpPolicy.Allowed {
			allowedNames = append(allowedNames, name)
		}
		messages = append(messages, providers.Message{
			Role: "system",
			Content: "MCP access for this contact is restricted. " +
				"You can only use MCP servers: " + strings.Join(allowedNames, ", ") +
				". If MCP access is needed outside this list, ask for permission/update.",
		})
	}

	// 3. Save user message to session
	al.sessions.AddMessage(opts.SessionKey, "user", opts.UserMessage)

	// 4. Run LLM iteration loop
	finalContent, iteration, err := al.runLLMIteration(ctx, messages, opts, mcpPolicy)
	if err != nil {
		return "", err
	}

	// 5. Handle empty response
	if finalContent == "" {
		finalContent = opts.DefaultResponse
	}

	// 6. Save final assistant message to session
	al.sessions.AddMessage(opts.SessionKey, "assistant", finalContent)
	al.sessions.Save(al.sessions.GetOrCreate(opts.SessionKey))

	// 7. Optional: summarization
	if opts.EnableSummary {
		al.maybeSummarize(opts.SessionKey)
	}

	// 8. Optional: send response via bus
	if opts.SendResponse {
		al.publishOutboundWithContactDelay(ctx, bus.OutboundMessage{
			Channel: opts.Channel,
			ChatID:  opts.ChatID,
			Content: finalContent,
		}, opts.SessionKey)
	}

	// 9. Log response
	responsePreview := utils.Truncate(finalContent, 120)
	logger.InfoCF("agent", fmt.Sprintf("Response: %s", responsePreview),
		map[string]interface{}{
			"session_key":  opts.SessionKey,
			"iterations":   iteration,
			"final_length": len(finalContent),
		})

	return finalContent, nil
}

func (al *AgentLoop) publishOutboundWithContactDelay(ctx context.Context, msg bus.OutboundMessage, sessionKey string) {
	if al.contactsStore != nil && sessionKey != "" {
		ci := al.contactsStore.GetContactForSession(sessionKey)
		if ci != nil && ci.ResponseDelaySeconds > 0 {
			delay := ci.ResponseDelaySeconds
			logger.InfoCF("agent", "Applying contact response delay", map[string]interface{}{
				"session_key":            sessionKey,
				"channel":                msg.Channel,
				"chat_id":                msg.ChatID,
				"response_delay_seconds": delay,
			})

			timer := time.NewTimer(time.Duration(delay) * time.Second)
			defer timer.Stop()

			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}
	}

	al.bus.PublishOutbound(msg)
}

// runLLMIteration executes the LLM call loop with tool handling.
// Returns the final content, iteration count, and any error.
func (al *AgentLoop) runLLMIteration(ctx context.Context, messages []providers.Message, opts processOptions, mcpPolicy sessionMCPPolicy) (string, int, error) {
	iteration := 0
	var finalContent string

	for iteration < al.maxIterations {
		iteration++

		logger.DebugCF("agent", "LLM iteration",
			map[string]interface{}{
				"iteration": iteration,
				"max":       al.maxIterations,
			})

		// Build tool definitions
		toolDefs := al.getToolDefinitionsForPolicy(mcpPolicy)
		providerToolDefs := make([]providers.ToolDefinition, 0, len(toolDefs))
		for _, td := range toolDefs {
			providerToolDefs = append(providerToolDefs, providers.ToolDefinition{
				Type: td["type"].(string),
				Function: providers.ToolFunctionDefinition{
					Name:        td["function"].(map[string]interface{})["name"].(string),
					Description: td["function"].(map[string]interface{})["description"].(string),
					Parameters:  td["function"].(map[string]interface{})["parameters"].(map[string]interface{}),
				},
			})
		}

		// Log LLM request details
		logger.DebugCF("agent", "LLM request",
			map[string]interface{}{
				"iteration":         iteration,
				"model":             al.model,
				"messages_count":    len(messages),
				"tools_count":       len(providerToolDefs),
				"max_tokens":        8192,
				"temperature":       0.7,
				"system_prompt_len": len(messages[0].Content),
			})

		// Log full messages (detailed)
		logger.DebugCF("agent", "Full LLM request",
			map[string]interface{}{
				"iteration":     iteration,
				"messages_json": formatMessagesForLog(messages),
				"tools_json":    formatToolsForLog(providerToolDefs),
			})

		// Call LLM
		response, err := al.provider.Chat(ctx, messages, providerToolDefs, al.model, map[string]interface{}{
			"max_tokens":  8192,
			"temperature": 0.7,
		})

		if err != nil {
			logger.ErrorCF("agent", "LLM call failed",
				map[string]interface{}{
					"iteration": iteration,
					"error":     err.Error(),
				})
			return "", iteration, fmt.Errorf("LLM call failed: %w", err)
		}

		// Check if no tool calls - we're done
		if len(response.ToolCalls) == 0 {
			finalContent = response.Content
			logger.InfoCF("agent", "LLM response without tool calls (direct answer)",
				map[string]interface{}{
					"iteration":     iteration,
					"content_chars": len(finalContent),
				})
			break
		}

		// Log tool calls
		toolNames := make([]string, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			toolNames = append(toolNames, tc.Name)
		}
		logger.InfoCF("agent", "LLM requested tool calls",
			map[string]interface{}{
				"tools":     toolNames,
				"count":     len(toolNames),
				"iteration": iteration,
			})

		// Build assistant message with tool calls
		assistantMsg := providers.Message{
			Role:    "assistant",
			Content: response.Content,
		}
		for _, tc := range response.ToolCalls {
			argumentsJSON, _ := json.Marshal(tc.Arguments)
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, providers.ToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: &providers.FunctionCall{
					Name:      tc.Name,
					Arguments: string(argumentsJSON),
				},
			})
		}
		messages = append(messages, assistantMsg)

		// Save assistant message with tool calls to session
		al.sessions.AddFullMessage(opts.SessionKey, assistantMsg)

		// Execute tool calls
		for _, tc := range response.ToolCalls {
			// Log tool call with arguments preview
			argsJSON, _ := json.Marshal(tc.Arguments)
			argsPreview := utils.Truncate(string(argsJSON), 200)
			logger.InfoCF("agent", fmt.Sprintf("Tool call: %s(%s)", tc.Name, argsPreview),
				map[string]interface{}{
					"tool":      tc.Name,
					"iteration": iteration,
				})

			if !al.isToolAllowedByPolicy(tc.Name, mcpPolicy) {
				result := "Error: MCP tool is not allowed for this contact."
				toolResultMsg := providers.Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
				}
				messages = append(messages, toolResultMsg)
				al.sessions.AddFullMessage(opts.SessionKey, toolResultMsg)
				continue
			}

			result, err := al.tools.ExecuteWithContext(ctx, tc.Name, tc.Arguments, opts.Channel, opts.ChatID)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
			}

			toolResultMsg := providers.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			}
			messages = append(messages, toolResultMsg)

			// Save tool result message to session
			al.sessions.AddFullMessage(opts.SessionKey, toolResultMsg)
		}
	}

	return finalContent, iteration, nil
}

func (al *AgentLoop) getSessionMCPPolicy(sessionKey string) sessionMCPPolicy {
	p := sessionMCPPolicy{Allowed: map[string]bool{}}
	if al.contactsStore == nil || sessionKey == "" {
		return p
	}
	ci := al.contactsStore.GetContactForSession(sessionKey)
	if ci == nil || len(ci.AllowedMCPs) == 0 {
		return p
	}
	for _, name := range ci.AllowedMCPs {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		p.Allowed[name] = true
	}
	return p
}

func (al *AgentLoop) getToolDefinitionsForPolicy(p sessionMCPPolicy) []map[string]interface{} {
	defs := al.tools.GetDefinitions()
	if len(p.Allowed) == 0 {
		return defs
	}

	filtered := make([]map[string]interface{}, 0, len(defs))
	for _, d := range defs {
		fn, _ := d["function"].(map[string]interface{})
		name, _ := fn["name"].(string)
		if al.isToolAllowedByPolicy(name, p) {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

func (al *AgentLoop) isToolAllowedByPolicy(toolName string, p sessionMCPPolicy) bool {
	if len(p.Allowed) == 0 {
		return true
	}
	mcpName := extractMCPServerName(toolName)
	if mcpName == "" {
		return true // non-MCP tools remain available
	}
	return p.Allowed[mcpName]
}

func extractMCPServerName(toolName string) string {
	name := strings.TrimSpace(toolName)
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, "mcp.") {
		parts := strings.Split(name, ".")
		if len(parts) >= 3 {
			return parts[1]
		}
	}
	if strings.HasPrefix(name, "mcp_") {
		parts := strings.Split(name, "_")
		if len(parts) >= 3 {
			return parts[1]
		}
	}
	if strings.Contains(name, "__") {
		parts := strings.Split(name, "__")
		if len(parts) >= 2 && parts[0] == "mcp" {
			return parts[1]
		}
	}
	return ""
}

// updateToolContexts updates the context for tools that need channel/chatID info.
func (al *AgentLoop) updateToolContexts(channel, chatID string) {
	if tool, ok := al.tools.Get("message"); ok {
		if mt, ok := tool.(*tools.MessageTool); ok {
			mt.SetContext(channel, chatID)
		}
	}
	if tool, ok := al.tools.Get("spawn"); ok {
		if st, ok := tool.(*tools.SpawnTool); ok {
			st.SetContext(channel, chatID)
		}
	}
}

// maybeSummarize triggers summarization if the session history exceeds thresholds.
func (al *AgentLoop) maybeSummarize(sessionKey string) {
	newHistory := al.sessions.GetHistory(sessionKey)
	tokenEstimate := al.estimateTokens(newHistory)
	threshold := al.contextWindow * 75 / 100

	if len(newHistory) > 20 || tokenEstimate > threshold {
		if _, loading := al.summarizing.LoadOrStore(sessionKey, true); !loading {
			go func() {
				defer al.summarizing.Delete(sessionKey)
				al.summarizeSession(sessionKey)
			}()
		}
	}
}

// GetStartupInfo returns information about loaded tools and skills for logging.
func (al *AgentLoop) GetStartupInfo() map[string]interface{} {
	info := make(map[string]interface{})
	info["agent_id"] = al.AgentID()
	info["mcp_servers"] = map[string]interface{}{
		"count":       len(al.mcpServers),
		"tools_count": al.mcpToolsCount,
	}

	// Tools info
	tools := al.tools.List()
	info["tools"] = map[string]interface{}{
		"count": len(tools),
		"names": tools,
	}

	// Skills info
	info["skills"] = al.contextBuilder.GetSkillsInfo()

	return info
}

// formatMessagesForLog formats messages for logging
func formatMessagesForLog(messages []providers.Message) string {
	if len(messages) == 0 {
		return "[]"
	}

	var result string
	result += "[\n"
	for i, msg := range messages {
		result += fmt.Sprintf("  [%d] Role: %s\n", i, msg.Role)
		if msg.ToolCalls != nil && len(msg.ToolCalls) > 0 {
			result += "  ToolCalls:\n"
			for _, tc := range msg.ToolCalls {
				result += fmt.Sprintf("    - ID: %s, Type: %s, Name: %s\n", tc.ID, tc.Type, tc.Name)
				if tc.Function != nil {
					result += fmt.Sprintf("      Arguments: %s\n", utils.Truncate(tc.Function.Arguments, 200))
				}
			}
		}
		if msg.Content != "" {
			content := utils.Truncate(msg.Content, 200)
			result += fmt.Sprintf("  Content: %s\n", content)
		}
		if msg.ToolCallID != "" {
			result += fmt.Sprintf("  ToolCallID: %s\n", msg.ToolCallID)
		}
		result += "\n"
	}
	result += "]"
	return result
}

// formatToolsForLog formats tool definitions for logging
func formatToolsForLog(tools []providers.ToolDefinition) string {
	if len(tools) == 0 {
		return "[]"
	}

	var result string
	result += "[\n"
	for i, tool := range tools {
		result += fmt.Sprintf("  [%d] Type: %s, Name: %s\n", i, tool.Type, tool.Function.Name)
		result += fmt.Sprintf("      Description: %s\n", tool.Function.Description)
		if len(tool.Function.Parameters) > 0 {
			result += fmt.Sprintf("      Parameters: %s\n", utils.Truncate(fmt.Sprintf("%v", tool.Function.Parameters), 200))
		}
	}
	result += "]"
	return result
}

// summarizeSession summarizes the conversation history for a session.
func (al *AgentLoop) summarizeSession(sessionKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	history := al.sessions.GetHistory(sessionKey)
	summary := al.sessions.GetSummary(sessionKey)

	// Keep last 4 messages for continuity
	if len(history) <= 4 {
		return
	}

	toSummarize := history[:len(history)-4]

	// Oversized Message Guard
	// Skip messages larger than 50% of context window to prevent summarizer overflow
	maxMessageTokens := al.contextWindow / 2
	validMessages := make([]providers.Message, 0)
	omitted := false

	for _, m := range toSummarize {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		// Estimate tokens for this message
		msgTokens := len(m.Content) / 4
		if msgTokens > maxMessageTokens {
			omitted = true
			continue
		}
		validMessages = append(validMessages, m)
	}

	if len(validMessages) == 0 {
		return
	}

	// Multi-Part Summarization
	// Split into two parts if history is significant
	var finalSummary string
	if len(validMessages) > 10 {
		mid := len(validMessages) / 2
		part1 := validMessages[:mid]
		part2 := validMessages[mid:]

		s1, _ := al.summarizeBatch(ctx, part1, "")
		s2, _ := al.summarizeBatch(ctx, part2, "")

		// Merge them
		mergePrompt := fmt.Sprintf("Merge these two conversation summaries into one cohesive summary:\n\n1: %s\n\n2: %s", s1, s2)
		resp, err := al.provider.Chat(ctx, []providers.Message{{Role: "user", Content: mergePrompt}}, nil, al.model, map[string]interface{}{
			"max_tokens":  1024,
			"temperature": 0.3,
		})
		if err == nil {
			finalSummary = resp.Content
		} else {
			finalSummary = s1 + " " + s2
		}
	} else {
		finalSummary, _ = al.summarizeBatch(ctx, validMessages, summary)
	}

	if omitted && finalSummary != "" {
		finalSummary += "\n[Note: Some oversized messages were omitted from this summary for efficiency.]"
	}

	if finalSummary != "" {
		al.sessions.SetSummary(sessionKey, finalSummary)
		al.sessions.TruncateHistory(sessionKey, 4)
		al.sessions.Save(al.sessions.GetOrCreate(sessionKey))
	}
}

// summarizeBatch summarizes a batch of messages.
func (al *AgentLoop) summarizeBatch(ctx context.Context, batch []providers.Message, existingSummary string) (string, error) {
	prompt := "Provide a concise summary of this conversation segment, preserving core context and key points.\n"
	if existingSummary != "" {
		prompt += "Existing context: " + existingSummary + "\n"
	}
	prompt += "\nCONVERSATION:\n"
	for _, m := range batch {
		prompt += fmt.Sprintf("%s: %s\n", m.Role, m.Content)
	}

	response, err := al.provider.Chat(ctx, []providers.Message{{Role: "user", Content: prompt}}, nil, al.model, map[string]interface{}{
		"max_tokens":  1024,
		"temperature": 0.3,
	})
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

// estimateTokens estimates the number of tokens in a message list.
func (al *AgentLoop) estimateTokens(messages []providers.Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content) / 4 // Simple heuristic: 4 chars per token
	}
	return total
}
