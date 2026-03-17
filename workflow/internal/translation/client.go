package translation

import (
	"fmt"

	"localize-agent/workflow/pkg/platform"
	"localize-agent/workflow/pkg/shared"
)

type serverClient struct {
	llm           llmClient
	profile       platform.LLMProfile
	sessionPrefix string
	responseMode  string
}

type llmClient interface {
	EnsureContext(key string, profile platform.LLMProfile) error
	SendPrompt(key string, profile platform.LLMProfile, prompt string) (string, error)
}

func newServerClient(backend, serverURL, model, agent string, skill *translateSkill, timeoutSec int, metrics *shared.MetricCollector, traceSink platform.LLMTraceSink) (*serverClient, error) {
	return newServerClientWithConfig(backend, serverURL, model, agent, Config{}, skill, timeoutSec, metrics, traceSink)
}

func newServerClientWithConfig(backend, serverURL, model, agent string, cfg Config, skill *translateSkill, timeoutSec int, metrics *shared.MetricCollector, traceSink platform.LLMTraceSink) (*serverClient, error) {
	normalizedBackend, err := platform.NormalizeLLMBackend(backend)
	if err != nil {
		return nil, err
	}
	responseMode := normalizeTranslatorResponseMode(cfg.TranslatorResponseMode)
	profile := platform.LLMProfile{
		ProviderID: normalizedBackend,
		ModelID:    model,
		Agent:      agent,
		Warmup:     skill.warmup(),
	}
	var client llmClient
	switch normalizedBackend {
	case platform.LLMBackendOpencode:
		providerID, modelID, err := platform.ParseModel(model)
		if err != nil {
			return nil, err
		}
		profile.ProviderID = providerID
		profile.ModelID = modelID
		client = platform.NewSessionLLMClient(serverURL, timeoutSec, metrics, traceSink)
	case platform.LLMBackendOllama:
		if cfg.OllamaBakedSystem {
			profile.Warmup = ""
		}
		profile.KeepAlive = cfg.OllamaKeepAlive
		profile.ResetHistory = cfg.OllamaResetHistory
		if cfg.OllamaStructuredOutput && responseMode == responseModeJSON {
			profile.ResponseFormat = proposalArraySchema()
		}
		if cfg.OllamaNumCtx > 0 || cfg.OllamaTemperature >= 0 {
			profile.Options = map[string]any{}
			if cfg.OllamaNumCtx > 0 {
				profile.Options["num_ctx"] = cfg.OllamaNumCtx
			}
			if cfg.OllamaTemperature >= 0 {
				profile.Options["temperature"] = cfg.OllamaTemperature
			}
		}
		client = platform.NewOllamaLLMClient(serverURL, timeoutSec, metrics, traceSink)
	default:
		return nil, fmt.Errorf("unsupported llm backend: %s", normalizedBackend)
	}
	return &serverClient{
		llm:           client,
		profile:       profile,
		sessionPrefix: serverURL,
		responseMode:  responseMode,
	}, nil
}

func (c *serverClient) ensureContext(key string) error {
	return c.llm.EnsureContext(key, c.profile)
}

func (c *serverClient) sendPrompt(key, prompt string) (string, error) {
	return c.llm.SendPrompt(key, c.profile, prompt)
}

func (c *serverClient) usesPlainTranslatorOutput() bool {
	return c.responseMode == responseModePlain
}

func (c *serverClient) sessionKey(slotKey string) string {
	return c.sessionPrefix + "#" + slotKey
}

func normalizeTranslatorResponseMode(mode string) string {
	switch mode {
	case responseModeJSON:
		return responseModeJSON
	case responseModePlain:
		return responseModePlain
	default:
		return responseModePlain
	}
}
