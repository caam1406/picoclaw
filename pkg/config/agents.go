package config

import "sort"

type ResolvedAgentConfig struct {
	AgentID    string
	Settings   AgentDefaults
	MCPServers []MCPServerConfig
}

func (c *Config) GetDefaultAgentID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.Agents.DefaultProfile != "" {
		return c.Agents.DefaultProfile
	}
	return "default"
}

func (c *Config) ListAgentIDs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	defaultID := c.Agents.DefaultProfile
	if defaultID == "" {
		defaultID = "default"
	}

	ids := []string{defaultID}
	seen := map[string]bool{defaultID: true}
	for id := range c.Agents.Profiles {
		if id == "" || seen[id] {
			continue
		}
		ids = append(ids, id)
		seen[id] = true
	}
	if len(ids) > 1 {
		rest := append([]string{}, ids[1:]...)
		sort.Strings(rest)
		ids = append([]string{defaultID}, rest...)
	}
	return ids
}

func (c *Config) ResolveAgentConfig(agentID string) ResolvedAgentConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	defaultAgentID := c.Agents.DefaultProfile
	if defaultAgentID == "" {
		defaultAgentID = "default"
	}
	if agentID == "" {
		agentID = defaultAgentID
	}

	settings := c.Agents.Defaults
	var mcpServers []MCPServerConfig

	if profile, ok := c.Agents.Profiles[agentID]; ok {
		if profile.Workspace != "" {
			settings.Workspace = profile.Workspace
		}
		if profile.Model != "" {
			settings.Model = profile.Model
		}
		if profile.MaxTokens > 0 {
			settings.MaxTokens = profile.MaxTokens
		}
		if profile.Temperature != 0 {
			settings.Temperature = profile.Temperature
		}
		if profile.MaxToolIterations > 0 {
			settings.MaxToolIterations = profile.MaxToolIterations
		}
		mcpServers = append([]MCPServerConfig{}, profile.MCPServers...)
	}

	return ResolvedAgentConfig{
		AgentID:    agentID,
		Settings:   settings,
		MCPServers: mcpServers,
	}
}

func (c *Config) HasAgentID(agentID string) bool {
	if agentID == "" {
		return false
	}
	for _, id := range c.ListAgentIDs() {
		if id == agentID {
			return true
		}
	}
	return false
}

func (c *Config) ListMCPNamesForAgent(agentID string) []string {
	resolved := c.ResolveAgentConfig(agentID)
	seen := map[string]bool{}
	out := make([]string, 0, len(resolved.MCPServers))
	for _, m := range resolved.MCPServers {
		name := m.Name
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
