package translation

import (
	"regexp"
	"strings"
)

const (
	textKindChoice    = "choice"
	textKindNarration = "narration"
	textKindDialogue  = "dialogue"
	laneDefault       = "default"
	laneHigh          = "high"
)

type textProfile struct {
	Kind        string
	HasRichText bool
}

var (
	gameplayPrefixRe = regexp.MustCompile(`^(ROLL\d+\s+[A-Za-z]+(?:-|\s+)|DC\d+\s+[A-Za-z]+(?:-|\s+)|BUY\d+[^-]*?(?:-|\s+))`)
	narrationLeadRe  = regexp.MustCompile(`^(He|She|They|It|You|Your|The\s+[A-Z][a-z]+|A\s+[a-z]+|An\s+[a-z]+)\b`)
	loreCueRe        = regexp.MustCompile(`\b([A-Za-z]+(ism|ocracy)|[A-Z][a-z]{3,})\b`)
)

func classifyTextProfile(enText string) textProfile {
	t := strings.TrimSpace(enText)
	profile := textProfile{
		Kind:        textKindDialogue,
		HasRichText: strings.Contains(t, "<") && strings.Contains(t, ">"),
	}
	if t == "" {
		return profile
	}
	if gameplayPrefixRe.MatchString(t) {
		profile.Kind = textKindChoice
		return profile
	}
	if narrationLeadRe.MatchString(t) {
		profile.Kind = textKindNarration
		return profile
	}
	return profile
}

func profileGroupKey(profile textProfile) string {
	if profile.HasRichText {
		return profile.Kind + "+rich"
	}
	return profile.Kind
}

func decideTranslationLane(enText string, profile textProfile, textRole string, isShortContext bool) string {
	trimmed := strings.TrimSpace(enText)
	n := len([]rune(trimmed))
	switch {
	case profile.Kind == textKindChoice:
		return laneHigh
	case profile.HasRichText && (isShortContext || n <= 80):
		return laneHigh
	case (textRole == "reaction" || textRole == "fragment") && n <= 64:
		return laneHigh
	case isShortContext && n <= 48:
		return laneHigh
	case loreCueRe.MatchString(trimmed) && n >= 60:
		return laneHigh
	default:
		return laneDefault
	}
}
