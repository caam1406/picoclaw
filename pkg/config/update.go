package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
)

func (c *Config) EnsureDashboardToken() (string, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if strings.TrimSpace(c.Dashboard.Token) != "" {
		return "", false, nil
	}

	token, err := generateToken(24)
	if err != nil {
		return "", false, err
	}

	c.Dashboard.Token = token
	return token, true, nil
}

func (c *Config) RotateDashboardToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	token, err := generateToken(24)
	if err != nil {
		return "", err
	}

	c.Dashboard.Token = token
	return token, nil
}

func generateToken(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (c *Config) ApplyUpdate(update *Config, secretUpdates map[string]string) {
	if c == nil || update == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.Agents = update.Agents
	c.Gateway = update.Gateway
	c.Storage = update.Storage

	c.Dashboard.Enabled = update.Dashboard.Enabled
	c.Dashboard.Host = update.Dashboard.Host
	c.Dashboard.Port = update.Dashboard.Port
	c.Dashboard.ContactsOnly = update.Dashboard.ContactsOnly

	c.Tools.Web.Search.MaxResults = update.Tools.Web.Search.MaxResults

	c.Channels.WhatsApp.Enabled = update.Channels.WhatsApp.Enabled
	c.Channels.WhatsApp.StorePath = update.Channels.WhatsApp.StorePath
	c.Channels.WhatsApp.AllowFrom = copyStringSlice(update.Channels.WhatsApp.AllowFrom)

	c.Channels.Telegram.Enabled = update.Channels.Telegram.Enabled
	c.Channels.Telegram.AllowFrom = copyStringSlice(update.Channels.Telegram.AllowFrom)

	c.Channels.Discord.Enabled = update.Channels.Discord.Enabled
	c.Channels.Discord.AllowFrom = copyStringSlice(update.Channels.Discord.AllowFrom)

	c.Channels.Feishu.Enabled = update.Channels.Feishu.Enabled
	c.Channels.Feishu.AppID = update.Channels.Feishu.AppID
	c.Channels.Feishu.AllowFrom = copyStringSlice(update.Channels.Feishu.AllowFrom)

	c.Channels.QQ.Enabled = update.Channels.QQ.Enabled
	c.Channels.QQ.AppID = update.Channels.QQ.AppID
	c.Channels.QQ.AllowFrom = copyStringSlice(update.Channels.QQ.AllowFrom)

	c.Channels.DingTalk.Enabled = update.Channels.DingTalk.Enabled
	c.Channels.DingTalk.ClientID = update.Channels.DingTalk.ClientID
	c.Channels.DingTalk.AllowFrom = copyStringSlice(update.Channels.DingTalk.AllowFrom)

	c.Channels.MaixCam.Enabled = update.Channels.MaixCam.Enabled
	c.Channels.MaixCam.Host = update.Channels.MaixCam.Host
	c.Channels.MaixCam.Port = update.Channels.MaixCam.Port
	c.Channels.MaixCam.AllowFrom = copyStringSlice(update.Channels.MaixCam.AllowFrom)

	c.Providers.Anthropic.APIBase = update.Providers.Anthropic.APIBase
	c.Providers.OpenAI.APIBase = update.Providers.OpenAI.APIBase
	c.Providers.OpenRouter.APIBase = update.Providers.OpenRouter.APIBase
	c.Providers.Groq.APIBase = update.Providers.Groq.APIBase
	c.Providers.Zhipu.APIBase = update.Providers.Zhipu.APIBase
	c.Providers.ZAI.APIBase = update.Providers.ZAI.APIBase
	c.Providers.VLLM.APIBase = update.Providers.VLLM.APIBase
	c.Providers.Gemini.APIBase = update.Providers.Gemini.APIBase

	ApplySecretUpdates(c, secretUpdates)
}

func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	data, err := json.Marshal(c)
	if err != nil {
		return DefaultConfig()
	}
	var clone Config
	if err := json.Unmarshal(data, &clone); err != nil {
		return DefaultConfig()
	}
	return &clone
}

func copyStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
