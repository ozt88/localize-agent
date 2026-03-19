package translation

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var emphasisPairRe = regexp.MustCompile(`(?is)<(i|b)>(.*?)</(i|b)>`)
var (
	pureControlTokenRe   = regexp.MustCompile(`^(?:\.[A-Za-z0-9_'\-]+==[^\s]+-|\.[A-Za-z0-9_'\-]+[<>]=?\d+-|[A-Za-z0-9_'\-]+==[^\s]+-|SPELL [A-Za-z0-9_'\-]+-)$`)
	controlQuotedTailRe  = regexp.MustCompile(`^((?:\.[A-Za-z0-9_'\-]+==[^\s]+-))(".*)$`)
)

type preparedPromptText struct {
	source        string
	current       string
	tagMaps       []mapping
	choicePrefix  string
	emphasisSpans []emphasisSpan
	controlPrefix string
	passthrough   bool
}

func preparePromptText(sourceRaw, currentRaw string, profile textProfile) preparedPromptText {
	source := sourceRaw
	current := currentRaw
	emphasisSpans := []emphasisSpan{}

	if profile.HasRichText {
		source, emphasisSpans = liftEmphasisTags(source)
		current, _ = liftEmphasisTags(current)
	}

	choicePrefix := ""
	if profile.Kind == textKindChoice {
		choicePrefix = gameplayPrefixRe.FindString(source)
		if choicePrefix != "" {
			source = strings.TrimSpace(strings.TrimPrefix(source, choicePrefix))
		}
		if currentPrefix := gameplayPrefixRe.FindString(current); currentPrefix != "" {
			current = strings.TrimSpace(strings.TrimPrefix(current, currentPrefix))
		}
	}

	controlPrefix := ""
	passthrough := false
	if pureControlTokenRe.MatchString(strings.TrimSpace(source)) {
		passthrough = true
	} else if looksNonEnglishPassthroughSource(source) {
		passthrough = true
	} else if m := controlQuotedTailRe.FindStringSubmatch(source); len(m) == 3 {
		controlPrefix = m[1]
		source = m[2]
		if m2 := controlQuotedTailRe.FindStringSubmatch(current); len(m2) == 3 {
			current = m2[2]
		}
	}

	// Strip straight double-quote characters before sending to LLM.
	// The game UI already frames dialogue visually, and unbalanced quotes
	// in fragment text confuse both translator and scorer models.
	source = strings.ReplaceAll(source, "\"", "")
	current = strings.ReplaceAll(current, "\"", "")

	maskedSource, tagMaps := maskTags(source)
	maskedCurrent, _ := maskTags(current)
	return preparedPromptText{
		source:        maskedSource,
		current:       maskedCurrent,
		tagMaps:       tagMaps,
		choicePrefix:  choicePrefix,
		emphasisSpans: emphasisSpans,
		controlPrefix: controlPrefix,
		passthrough:   passthrough,
	}
}

func looksNonEnglishPassthroughSource(source string) bool {
	stripped := stripSimpleTags(strings.TrimSpace(source))
	if stripped == "" {
		return false
	}
	lower := " " + strings.ToLower(stripped) + " "
	for _, marker := range []string{" the ", " and ", " of ", " is ", " are ", " your ", " you ", " this ", " that ", " with ", " from ", " for ", " to "} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	asciiLetters := 0
	nonASCIILetters := 0
	for _, r := range stripped {
		if !unicode.IsLetter(r) {
			continue
		}
		if r <= unicode.MaxASCII {
			asciiLetters++
			continue
		}
		nonASCIILetters++
	}
	if nonASCIILetters < 3 {
		return false
	}
	return asciiLetters+nonASCIILetters >= 8
}

func liftEmphasisTags(text string) (string, []emphasisSpan) {
	idx := 0
	spans := []emphasisSpan{}
	out := emphasisPairRe.ReplaceAllStringFunc(text, func(s string) string {
		m := emphasisPairRe.FindStringSubmatch(s)
		if len(m) != 4 || !strings.EqualFold(m[1], m[3]) {
			return s
		}
		openMarker := fmt.Sprintf("[[E%d]]", idx)
		closeMarker := fmt.Sprintf("[[/E%d]]", idx)
		spans = append(spans, emphasisSpan{
			openMarker:  openMarker,
			closeMarker: closeMarker,
			openTag:     "<" + strings.ToLower(m[1]) + ">",
			closeTag:    "</" + strings.ToLower(m[1]) + ">",
		})
		idx++
		return openMarker + m[2] + closeMarker
	})
	return out, spans
}

func restorePreparedText(proposed string, meta itemMeta) (string, error) {
	if meta.passthrough {
		return meta.sourceRaw, nil
	}
	text := strings.TrimSpace(proposed)
	if meta.isStatCheck && meta.statCheck != "" {
		text = stripExistingStatCheckPrefix(text, meta.statCheck)
		prefix := localizedStatCheckPrefix(meta.statCheck)
		if prefix != "" && !strings.HasPrefix(text, prefix) {
			text = prefix + text
		}
	} else if meta.choicePrefix != "" && !strings.HasPrefix(text, meta.choicePrefix) {
		text = meta.choicePrefix + text
	}
	if meta.controlPrefix != "" && !strings.HasPrefix(text, meta.controlPrefix) {
		text = meta.controlPrefix + text
	}
	withTags, err := restoreEmphasisTags(text, meta.emphasisSpans)
	if err != nil {
		return text, err
	}
	return restoreTags(withTags, meta.mapTags)
}

func localizedStatCheckPrefix(statCheck string) string {
	parts := strings.Fields(strings.TrimSpace(statCheck))
	if len(parts) != 2 {
		return ""
	}
	label := map[string]string{
		"STR": "힘",
		"DEX": "민첩",
		"INT": "지능",
		"WIS": "지혜",
		"CON": "건강",
		"CHA": "매력",
	}[strings.ToUpper(parts[0])]
	if label == "" {
		label = strings.ToUpper(parts[0])
	}
	return "[" + label + " " + parts[1] + "] "
}

func stripExistingStatCheckPrefix(text, statCheck string) string {
	parts := strings.Fields(strings.TrimSpace(statCheck))
	if len(parts) != 2 {
		return text
	}
	number := regexp.QuoteMeta(parts[1])
	re := regexp.MustCompile(`^\[[^\[\]]{1,12}\s+` + number + `\]\s*`)
	return strings.TrimSpace(re.ReplaceAllString(strings.TrimSpace(text), ""))
}

func restoreEmphasisTags(text string, spans []emphasisSpan) (string, error) {
	out := text
	for _, span := range spans {
		openCount := strings.Count(out, span.openMarker)
		closeCount := strings.Count(out, span.closeMarker)
		if openCount != closeCount {
			return out, fmt.Errorf("emphasis marker mismatch")
		}
		if openCount > 1 {
			return out, fmt.Errorf("duplicate emphasis markers")
		}
		out = strings.ReplaceAll(out, span.openMarker, span.openTag)
		out = strings.ReplaceAll(out, span.closeMarker, span.closeTag)
	}
	return out, nil
}
