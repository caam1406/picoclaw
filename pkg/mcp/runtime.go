package mcp

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

type Runtime struct {
	agentID string
	servers []config.MCPServerConfig

	mu      sync.RWMutex
	clients map[string]*Client
	tools   []RemoteTool
	status  map[string]*ServerStatus
}

type ServerStatus struct {
	ServerName string
	Enabled    bool
	Command    string
	Connected  bool
	ToolCount  int
	Error      string
}

func NewRuntime(agentID string, servers []config.MCPServerConfig) *Runtime {
	cloned := make([]config.MCPServerConfig, len(servers))
	copy(cloned, servers)
	return &Runtime{
		agentID: agentID,
		servers: cloned,
		clients: make(map[string]*Client),
		tools:   []RemoteTool{},
		status:  map[string]*ServerStatus{},
	}
}

func (r *Runtime) Start(ctx context.Context) {
	for _, s := range r.servers {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			continue
		}

		r.mu.Lock()
		r.status[name] = &ServerStatus{
			ServerName: name,
			Enabled:    s.Enabled,
			Command:    s.Command,
		}
		r.mu.Unlock()

		if !s.Enabled {
			continue
		}

		serverCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		client, err := StartClient(serverCtx, name, s.Command, s.Args, s.Env)
		cancel()
		if err != nil {
			r.mu.Lock()
			if st := r.status[name]; st != nil {
				st.Connected = false
				st.ToolCount = 0
				st.Error = err.Error()
			}
			r.mu.Unlock()
			logger.WarnCF("mcp", "Failed to start MCP server", map[string]interface{}{
				"agent_id": r.agentID,
				"server":   name,
				"error":    err.Error(),
			})
			continue
		}

		listCtx, listCancel := context.WithTimeout(ctx, 20*time.Second)
		tools, err := client.ListTools(listCtx)
		listCancel()
		if err != nil {
			r.mu.Lock()
			if st := r.status[name]; st != nil {
				st.Connected = false
				st.ToolCount = 0
				st.Error = err.Error()
			}
			r.mu.Unlock()
			logger.WarnCF("mcp", "Failed to list MCP tools", map[string]interface{}{
				"agent_id": r.agentID,
				"server":   name,
				"error":    err.Error(),
			})
			client.Close()
			continue
		}

		r.mu.Lock()
		r.clients[name] = client
		r.tools = append(r.tools, tools...)
		if st := r.status[name]; st != nil {
			st.Connected = true
			st.ToolCount = len(tools)
			st.Error = ""
		}
		r.mu.Unlock()

		logger.InfoCF("mcp", "MCP server connected", map[string]interface{}{
			"agent_id":   r.agentID,
			"server":     name,
			"tool_count": len(tools),
		})
	}
}

func (r *Runtime) Tools() []RemoteTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]RemoteTool, len(r.tools))
	copy(out, r.tools)
	return out
}

func (r *Runtime) Client(serverName string) (*Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.clients[serverName]
	return c, ok
}

func (r *Runtime) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, c := range r.clients {
		c.Close()
		delete(r.clients, name)
		if st := r.status[name]; st != nil {
			st.Connected = false
			st.Error = "stopped"
		}
	}
	r.tools = nil
}

func (r *Runtime) StatusSnapshot() []ServerStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.status))
	for name := range r.status {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]ServerStatus, 0, len(names))
	for _, name := range names {
		st := r.status[name]
		if st == nil {
			continue
		}
		item := *st
		if c, ok := r.clients[name]; ok && c != nil {
			closed, errMsg := c.State()
			if closed {
				item.Connected = false
				if errMsg != "" {
					item.Error = errMsg
				}
			}
		}
		out = append(out, item)
	}
	return out
}
