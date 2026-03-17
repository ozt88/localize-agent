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
	gameplayPrefixRe = regexp.MustCompile(`^(ROLL\d+\s+[A-Za-z]+(?:-|\s+)|DC\d+\s+[A-Za-z]+(?:-|\s+)|FC\d+\s+[A-Za-z]+(?:-|\s+)|BUY\d+[^-]*?(?:-|\s+))`)
	narrationLeadRe  = regexp.MustCompile(`^(He|She|They|It|You|Your|The\s+[A-Z][a-z]+|A\s+[a-z]+|An\s+[a-z]+)\b`)
	loreCueRe        = regexp.MustCompile(`\b([A-Za-z]+(ism|ocracy)|[A-Z][a-z]{3,})\b`)
	internalUILabelTechRe = regexp.MustCompile(`(?i)(cfx|rig|target|aim|armature|indicator)`)
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
	kind := profile.Kind
	if kind == textKindDialogue || kind == textKindNarration {
		kind = textKindDialogue
	}
	if profile.HasRichText {
		return kind + "+rich"
	}
	return kind
}

func isUIRole(textRole string) bool {
	switch strings.TrimSpace(textRole) {
	case "ui_label", "ui_description", "tooltip", "button":
		return true
	default:
		return false
	}
}

func shouldPreserveInternalUILabel(source, textRole, retryReason, sourceFile string) bool {
	if strings.TrimSpace(textRole) != "ui_label" {
		return false
	}
	reason := strings.TrimSpace(retryReason)
	file := strings.ToLower(strings.TrimSpace(sourceFile))
	if !strings.Contains(reason, "prefab_static_missing_from_canonical_source") && !strings.Contains(file, ".prefab") {
		return false
	}
	s := strings.TrimSpace(source)
	if s == "" {
		return false
	}
	if strings.ContainsAny(s, "_.") {
		return true
	}
	if internalUILabelTechRe.MatchString(s) {
		return true
	}
	return false
}

func decideTranslationLane(enText string, profile textProfile, textRole string, isShortContext bool) string {
	trimmed := strings.TrimSpace(enText)
	n := len([]rune(trimmed))
	switch {
	case isUIRole(textRole) && n <= 120:
		return laneHigh
	case isUIRole(textRole):
		return laneDefault
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
