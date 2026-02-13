package config

import "strings"

type secretAccessor struct {
	Path string
	Get  func(*Config) string
	Set  func(*Config, string)
}

var secretAccessors = []secretAccessor{
	{
		Path: "dashboard.token",
		Get:  func(c *Config) string { return c.Dashboard.Token },
		Set:  func(c *Config, v string) { c.Dashboard.Token = v },
	},
	{
		Path: "tools.web.search.api_key",
		Get:  func(c *Config) string { return c.Tools.Web.Search.APIKey },
		Set:  func(c *Config, v string) { c.Tools.Web.Search.APIKey = v },
	},
	{
		Path: "providers.anthropic.api_key",
		Get:  func(c *Config) string { return c.Providers.Anthropic.APIKey },
		Set:  func(c *Config, v string) { c.Providers.Anthropic.APIKey = v },
	},
	{
		Path: "providers.openai.api_key",
		Get:  func(c *Config) string { return c.Providers.OpenAI.APIKey },
		Set:  func(c *Config, v string) { c.Providers.OpenAI.APIKey = v },
	},
	{
		Path: "providers.openrouter.api_key",
		Get:  func(c *Config) string { return c.Providers.OpenRouter.APIKey },
		Set:  func(c *Config, v string) { c.Providers.OpenRouter.APIKey = v },
	},
	{
		Path: "providers.groq.api_key",
		Get:  func(c *Config) string { return c.Providers.Groq.APIKey },
		Set:  func(c *Config, v string) { c.Providers.Groq.APIKey = v },
	},
	{
		Path: "providers.zhipu.api_key",
		Get:  func(c *Config) string { return c.Providers.Zhipu.APIKey },
		Set:  func(c *Config, v string) { c.Providers.Zhipu.APIKey = v },
	},
	{
		Path: "providers.zai.api_key",
		Get:  func(c *Config) string { return c.Providers.ZAI.APIKey },
		Set:  func(c *Config, v string) { c.Providers.ZAI.APIKey = v },
	},
	{
		Path: "providers.vllm.api_key",
		Get:  func(c *Config) string { return c.Providers.VLLM.APIKey },
		Set:  func(c *Config, v string) { c.Providers.VLLM.APIKey = v },
	},
	{
		Path: "providers.gemini.api_key",
		Get:  func(c *Config) string { return c.Providers.Gemini.APIKey },
		Set:  func(c *Config, v string) { c.Providers.Gemini.APIKey = v },
	},
	{
		Path: "channels.telegram.token",
		Get:  func(c *Config) string { return c.Channels.Telegram.Token },
		Set:  func(c *Config, v string) { c.Channels.Telegram.Token = v },
	},
	{
		Path: "channels.discord.token",
		Get:  func(c *Config) string { return c.Channels.Discord.Token },
		Set:  func(c *Config, v string) { c.Channels.Discord.Token = v },
	},
	{
		Path: "channels.feishu.app_secret",
		Get:  func(c *Config) string { return c.Channels.Feishu.AppSecret },
		Set:  func(c *Config, v string) { c.Channels.Feishu.AppSecret = v },
	},
	{
		Path: "channels.feishu.encrypt_key",
		Get:  func(c *Config) string { return c.Channels.Feishu.EncryptKey },
		Set:  func(c *Config, v string) { c.Channels.Feishu.EncryptKey = v },
	},
	{
		Path: "channels.feishu.verification_token",
		Get:  func(c *Config) string { return c.Channels.Feishu.VerificationToken },
		Set:  func(c *Config, v string) { c.Channels.Feishu.VerificationToken = v },
	},
	{
		Path: "channels.qq.app_secret",
		Get:  func(c *Config) string { return c.Channels.QQ.AppSecret },
		Set:  func(c *Config, v string) { c.Channels.QQ.AppSecret = v },
	},
	{
		Path: "channels.dingtalk.client_secret",
		Get:  func(c *Config) string { return c.Channels.DingTalk.ClientSecret },
		Set:  func(c *Config, v string) { c.Channels.DingTalk.ClientSecret = v },
	},
}

func MaskSecret(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 5 {
		return "*****" + value
	}
	return "*****" + value[len(value)-5:]
}

func SecretMaskMap(cfg *Config) map[string]string {
	result := make(map[string]string)
	if cfg == nil {
		return result
	}
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()
	for _, accessor := range secretAccessors {
		value := accessor.Get(cfg)
		if value != "" {
			result[accessor.Path] = MaskSecret(value)
		}
	}
	return result
}

func ApplySecretUpdates(cfg *Config, updates map[string]string) {
	if cfg == nil || len(updates) == 0 {
		return
	}
	for _, accessor := range secretAccessors {
		if value, ok := updates[accessor.Path]; ok && strings.TrimSpace(value) != "" {
			accessor.Set(cfg, strings.TrimSpace(value))
		}
	}
}

func ClearSecrets(cfg *Config) {
	if cfg == nil {
		return
	}
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	for _, accessor := range secretAccessors {
		accessor.Set(cfg, "")
	}
}
