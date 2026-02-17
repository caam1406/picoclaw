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

// ensure Runtime satisfies the ClientResolver interface used by MCPTool
var _ interface {
	Client(string) (*Client, bool)
} = (*Runtime)(nil)

type Runtime struct {
	agentID string
	servers []config.MCPServerConfig

	mu      sync.RWMutex
	clients map[string]*Client
	tools   []RemoteTool
	status  map[string]*ServerStatus

	ctx    context.Context
	cancel context.CancelFunc
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
	r.ctx, r.cancel = context.WithCancel(ctx)

	var wg sync.WaitGroup
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

		wg.Add(1)
		go func(srv config.MCPServerConfig) {
			defer wg.Done()
			r.connectServer(r.ctx, srv)
		}(s)
	}
	wg.Wait()
}

// connectServer attempts to start a single MCP server and list its tools.
// On success it stores the client, tools and status. On failure it updates status.
func (r *Runtime) connectServer(ctx context.Context, s config.MCPServerConfig) {
	name := strings.TrimSpace(s.Name)

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
		return
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
		return
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

	// Start watcher goroutine for automatic reconnection on crash
	go r.watchClient(s)
}

// watchClient monitors a connected MCP server and attempts reconnection
// with exponential backoff if the process exits unexpectedly.
func (r *Runtime) watchClient(s config.MCPServerConfig) {
	name := strings.TrimSpace(s.Name)
	const maxRetries = 5
	backoffs := []time.Duration{5 * time.Second, 10 * time.Second, 30 * time.Second, 60 * time.Second, 60 * time.Second}

	for attempt := 0; attempt < maxRetries; attempt++ {
		r.mu.RLock()
		client, ok := r.clients[name]
		r.mu.RUnlock()
		if !ok || client == nil {
			return
		}

		// Wait for client to close
		select {
		case <-r.ctx.Done():
			return
		case <-client.Closed():
		}

		// Client closed — check if it was an intentional stop
		select {
		case <-r.ctx.Done():
			return
		default:
		}

		delay := backoffs[attempt]
		logger.WarnCF("mcp", "MCP server disconnected, attempting reconnect", map[string]interface{}{
			"agent_id": r.agentID,
			"server":   name,
			"attempt":  attempt + 1,
			"delay_s":  delay.Seconds(),
		})

		r.mu.Lock()
		if st := r.status[name]; st != nil {
			st.Connected = false
			st.Error = "disconnected, reconnecting..."
		}
		// Remove old tools from this server
		filtered := make([]RemoteTool, 0, len(r.tools))
		for _, t := range r.tools {
			if t.ServerName != name {
				filtered = append(filtered, t)
			}
		}
		r.tools = filtered
		delete(r.clients, name)
		r.mu.Unlock()

		select {
		case <-r.ctx.Done():
			return
		case <-time.After(delay):
		}

		// Attempt reconnect
		serverCtx, cancel := context.WithTimeout(r.ctx, 15*time.Second)
		newClient, err := StartClient(serverCtx, name, s.Command, s.Args, s.Env)
		cancel()
		if err != nil {
			r.mu.Lock()
			if st := r.status[name]; st != nil {
				st.Error = err.Error()
			}
			r.mu.Unlock()
			logger.WarnCF("mcp", "MCP reconnect failed", map[string]interface{}{
				"agent_id": r.agentID,
				"server":   name,
				"attempt":  attempt + 1,
				"error":    err.Error(),
			})
			continue
		}

		listCtx, listCancel := context.WithTimeout(r.ctx, 20*time.Second)
		tools, err := newClient.ListTools(listCtx)
		listCancel()
		if err != nil {
			r.mu.Lock()
			if st := r.status[name]; st != nil {
				st.Error = err.Error()
			}
			r.mu.Unlock()
			logger.WarnCF("mcp", "MCP reconnect: failed to list tools", map[string]interface{}{
				"agent_id": r.agentID,
				"server":   name,
				"attempt":  attempt + 1,
				"error":    err.Error(),
			})
			newClient.Close()
			continue
		}

		r.mu.Lock()
		r.clients[name] = newClient
		r.tools = append(r.tools, tools...)
		if st := r.status[name]; st != nil {
			st.Connected = true
			st.ToolCount = len(tools)
			st.Error = ""
		}
		r.mu.Unlock()

		logger.InfoCF("mcp", "MCP server reconnected", map[string]interface{}{
			"agent_id":   r.agentID,
			"server":     name,
			"tool_count": len(tools),
			"attempt":    attempt + 1,
		})

		// Reset attempt counter on success — keep watching
		attempt = -1 // will become 0 after loop increment
	}

	// Exhausted retries
	logger.ErrorCF("mcp", "MCP server reconnection exhausted", map[string]interface{}{
		"agent_id":    r.agentID,
		"server":      name,
		"max_retries": maxRetries,
	})
	r.mu.Lock()
	if st := r.status[name]; st != nil {
		st.Connected = false
		st.Error = "reconnection exhausted"
	}
	r.mu.Unlock()
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
	if r.cancel != nil {
		r.cancel()
	}
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
