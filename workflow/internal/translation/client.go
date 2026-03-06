package translation

import (
	"fmt"

	"localize-agent/workflow/internal/platform"
	"localize-agent/workflow/internal/shared"
)

type serverClient struct {
	llm     llmClient
	profile platform.LLMProfile
}

type llmClient interface {
	EnsureContext(key string, profile platform.LLMProfile) error
	SendPrompt(key string, profile platform.LLMProfile, prompt string) (string, error)
}

func newServerClient(backend, serverURL, model, agent string, skill *translateSkill, timeoutSec int, metrics *shared.MetricCollector, traceSink platform.LLMTraceSink) (*serverClient, error) {
	normalizedBackend, err := platform.NormalizeLLMBackend(backend)
	if err != nil {
		return nil, err
	}
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
		client = platform.NewOllamaLLMClient(serverURL, timeoutSec, metrics, traceSink)
	default:
		return nil, fmt.Errorf("unsupported llm backend: %s", normalizedBackend)
	}
	return &serverClient{
		llm:     client,
		profile: profile,
	}, nil
}

func (c *serverClient) ensureContext(key string) error {
	return c.llm.EnsureContext(key, c.profile)
}

func (c *serverClient) sendPrompt(key, prompt string) (string, error) {
	return c.llm.SendPrompt(key, c.profile, prompt)
}
