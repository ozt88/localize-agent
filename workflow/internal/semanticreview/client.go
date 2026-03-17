package semanticreview

import (
	"fmt"

	"localize-agent/workflow/pkg/platform"
	"localize-agent/workflow/pkg/shared"
)

func newReviewClientAndProfile(cfg Config, traceSink platform.LLMTraceSink, warmup string, responseFormat any) (llmClient, platform.LLMProfile, string, error) {
	normalizedBackend, err := platform.NormalizeLLMBackend(cfg.LLMBackend)
	if err != nil {
		return nil, platform.LLMProfile{}, "", err
	}

	profile := platform.LLMProfile{
		ProviderID:     normalizedBackend,
		ModelID:        cfg.Model,
		Agent:          cfg.Agent,
		Warmup:         warmup,
		ResetHistory:   true,
		ResponseFormat: responseFormat,
		Options: map[string]any{
			"temperature": 0,
		},
	}

	var client llmClient
	switch normalizedBackend {
	case platform.LLMBackendOpencode:
		providerID, modelID, err := platform.ParseModel(cfg.Model)
		if err != nil {
			return nil, platform.LLMProfile{}, "", err
		}
		profile.ProviderID = providerID
		profile.ModelID = modelID
		client = platform.NewSessionLLMClient(cfg.ServerURL, cfg.TimeoutSec, &shared.MetricCollector{}, traceSink)
	case platform.LLMBackendOllama:
		client = platform.NewOllamaLLMClient(cfg.ServerURL, cfg.TimeoutSec, &shared.MetricCollector{}, traceSink)
	default:
		return nil, platform.LLMProfile{}, "", fmt.Errorf("unsupported llm backend: %s", normalizedBackend)
	}

	return client, profile, cfg.ServerURL, nil
}
