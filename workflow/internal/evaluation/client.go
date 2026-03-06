package evaluation

import (
	"fmt"

	"localize-agent/workflow/internal/platform"
	"localize-agent/workflow/internal/shared"
)

type evalClient struct {
	llm llmClient

	transProfile platform.LLMProfile
	evalProfile  platform.LLMProfile
	transShape   string
	evalShape    string
}

type llmClient interface {
	EnsureContext(key string, profile platform.LLMProfile) error
	SendPrompt(key string, profile platform.LLMProfile, prompt string) (string, error)
}

func newEvalClient(backend, serverURL, transModel, transAgent string, ts *translateSkill, evalModel, evalAgent string, es *evaluateSkill, timeoutSec int, metrics *shared.MetricCollector, traceSink platform.LLMTraceSink) (*evalClient, error) {
	normalizedBackend, err := platform.NormalizeLLMBackend(backend)
	if err != nil {
		return nil, err
	}

	transProfile := platform.LLMProfile{
		ProviderID: normalizedBackend,
		ModelID:    transModel,
		Agent:      transAgent,
		Warmup:     ts.warmup(),
	}
	evalProfile := platform.LLMProfile{
		ProviderID: normalizedBackend,
		ModelID:    evalModel,
		Agent:      evalAgent,
		Warmup:     es.warmup(),
	}

	var client llmClient
	switch normalizedBackend {
	case platform.LLMBackendOpencode:
		transProvider, transModelID, err := platform.ParseModel(transModel)
		if err != nil {
			return nil, fmt.Errorf("invalid trans-model %q: %w", transModel, err)
		}
		evalProvider, evalModelID, err := platform.ParseModel(evalModel)
		if err != nil {
			return nil, fmt.Errorf("invalid eval-model %q: %w", evalModel, err)
		}
		transProfile.ProviderID = transProvider
		transProfile.ModelID = transModelID
		evalProfile.ProviderID = evalProvider
		evalProfile.ModelID = evalModelID
		client = platform.NewSessionLLMClient(serverURL, timeoutSec, metrics, traceSink)
	case platform.LLMBackendOllama:
		client = platform.NewOllamaLLMClient(serverURL, timeoutSec, metrics, traceSink)
	default:
		return nil, fmt.Errorf("unsupported llm backend: %s", normalizedBackend)
	}

	return &evalClient{
		llm:          client,
		transProfile: transProfile,
		evalProfile:  evalProfile,
		transShape:   ts.shapeHint(),
		evalShape:    es.shapeHint(),
	}, nil
}

func (c *evalClient) profile(kind string) platform.LLMProfile {
	if kind == kindEval {
		return c.evalProfile
	}
	return c.transProfile
}

func (c *evalClient) ensureContext(slotKey, kind string) error {
	return c.llm.EnsureContext(slotKey+"#"+kind, c.profile(kind))
}

func (c *evalClient) sendPrompt(slotKey, kind, prompt string) (string, error) {
	return c.llm.SendPrompt(slotKey+"#"+kind, c.profile(kind), prompt)
}

func (c *evalClient) evalShapeHint() string {
	return c.evalShape
}

func (c *evalClient) transShapeHint() string {
	return c.transShape
}
