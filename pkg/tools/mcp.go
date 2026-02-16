package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/mcp"
)

type MCPTool struct {
	localName      string
	description    string
	parameters     map[string]interface{}
	client         *mcp.Client
	remoteToolName string
}

func NewMCPTool(serverName string, remote mcp.RemoteTool, client *mcp.Client) *MCPTool {
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
		description:    desc,
		parameters:     params,
		client:         client,
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
	if t.client == nil {
		return "", fmt.Errorf("mcp client is not initialized")
	}
	result, err := t.client.CallTool(ctx, t.remoteToolName, args)
	if err != nil {
		return result, err
	}
	return result, nil
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
