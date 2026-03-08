package translation

type normalizedPromptInput struct {
	ID          string `json:"id"`
	BodyEN      string `json:"body_en"`
	ContextEN   string `json:"context_en,omitempty"`
	TextRole    string `json:"text_role,omitempty"`
	SpeakerHint string `json:"speaker_hint,omitempty"`
}

func normalizePromptInput(task translationTask) normalizedPromptInput {
	return normalizedPromptInput{
		ID:          task.ID,
		BodyEN:      task.BodyEN,
		ContextEN:   task.ContextEN,
		TextRole:    task.TextRole,
		SpeakerHint: task.SpeakerHint,
	}
}
