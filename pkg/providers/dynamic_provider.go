package providers

import (
	"context"
	"fmt"
	"sync"

	"github.com/sipeed/picoclaw/pkg/config"
)

// DynamicProvider resolves provider configuration at request time so API key/model
// updates from dashboard can take effect without restarting the gateway.
type DynamicProvider struct {
	cfg *config.Config

	mu        sync.Mutex
	lastSig   string
	lastModel string
	provider  LLMProvider
}

func NewDynamicProvider(cfg *config.Config) *DynamicProvider {
	return &DynamicProvider{cfg: cfg}
}

func (p *DynamicProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string, options map[string]interface{}) (*LLMResponse, error) {
	if p.cfg == nil {
		return nil, fmt.Errorf("dynamic provider: config not set")
	}

	resolvedModel := model
	if resolvedModel == "" {
		resolvedModel = p.GetDefaultModel()
	}

	// Snapshot config to avoid data races while dashboard updates configuration.
	snapshot := p.cfg.Clone()
	provider, sig, err := p.getOrCreateProvider(snapshot, resolvedModel)
	if err != nil {
		return nil, err
	}

	_ = sig
	return provider.Chat(ctx, messages, tools, resolvedModel, options)
}

func (p *DynamicProvider) GetDefaultModel() string {
	if p.cfg == nil {
		return ""
	}
	snapshot := p.cfg.Clone()
	return snapshot.Agents.Defaults.Model
}

func (p *DynamicProvider) getOrCreateProvider(cfgSnapshot *config.Config, model string) (LLMProvider, string, error) {
	sig := providerSignatureForModel(cfgSnapshot, model)

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.provider != nil && p.lastSig == sig && p.lastModel == model {
		return p.provider, sig, nil
	}

	next, err := CreateProviderForModel(cfgSnapshot, model)
	if err != nil {
		return nil, sig, err
	}

	p.provider = next
	p.lastSig = sig
	p.lastModel = model
	return p.provider, sig, nil
}

func providerSignatureForModel(cfg *config.Config, model string) string {
	if cfg == nil {
		return model + "|nil"
	}

	return model + "|" +
		cfg.Providers.OpenRouter.APIKey + "|" + cfg.Providers.OpenRouter.APIBase + "|" +
		cfg.Providers.Anthropic.APIKey + "|" + cfg.Providers.Anthropic.APIBase + "|" +
		cfg.Providers.OpenAI.APIKey + "|" + cfg.Providers.OpenAI.APIBase + "|" +
		cfg.Providers.Gemini.APIKey + "|" + cfg.Providers.Gemini.APIBase + "|" +
		cfg.Providers.Zhipu.APIKey + "|" + cfg.Providers.Zhipu.APIBase + "|" +
		cfg.Providers.ZAI.APIKey + "|" + cfg.Providers.ZAI.APIBase + "|" +
		cfg.Providers.Groq.APIKey + "|" + cfg.Providers.Groq.APIBase + "|" +
		cfg.Providers.VLLM.APIKey + "|" + cfg.Providers.VLLM.APIBase
}
