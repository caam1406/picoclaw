package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/mcp"
)

// ClientResolver looks up the current live MCP client for a server.
// This allows MCPTool to survive server reconnections without holding a stale pointer.
type ClientResolver interface {
	Client(serverName string) (*mcp.Client, bool)
}

type MCPTool struct {
	localName      string
	serverName     string
	description    string
	parameters     map[string]interface{}
	resolver       ClientResolver
	remoteToolName string
}

func NewMCPTool(serverName string, remote mcp.RemoteTool, resolver ClientResolver) *MCPTool {
	local := fmt.Sprintf("mcp.%s.%s", sanitizeToolToken(serverName), sanitizeToolToken(remote.Name))
	desc := strings.TrimSpace(remote.Description)
	if desc == "" {
		desc = fmt.Sprintf("MCP tool %s from server %s", remote.Name, serverName)
	}
	params := remote.InputSchema
	if params == nil {
		params = map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}
	return &MCPTool{
		localName:      local,
		serverName:     serverName,
		description:    desc,
		parameters:     params,
		resolver:       resolver,
		remoteToolName: remote.Name,
	}
}

func (t *MCPTool) Name() string {
	return t.localName
}

func (t *MCPTool) Description() string {
	return t.description
}

func (t *MCPTool) Parameters() map[string]interface{} {
	return t.parameters
}

func (t *MCPTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	if t.resolver == nil {
		return "", fmt.Errorf("mcp client resolver is not initialized")
	}
	client, ok := t.resolver.Client(t.serverName)
	if !ok || client == nil {
		return "", fmt.Errorf("mcp server %q is not connected", t.serverName)
	}
	return client.CallTool(ctx, t.remoteToolName, args)
}

func sanitizeToolToken(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return "unknown"
	}
	v = strings.ReplaceAll(v, " ", "_")
	v = strings.ReplaceAll(v, "/", "_")
	return v
}
