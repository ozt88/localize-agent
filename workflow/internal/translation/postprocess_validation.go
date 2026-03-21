package translation

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	structuralTokenRE   = regexp.MustCompile(`(\$[A-Za-z0-9_]+|\{[^{}]+\})`)
	htmlTagRE           = regexp.MustCompile(`<[^>]+>`)
	lowerEnglishInTagRe = regexp.MustCompile(`>([^<]*[a-z][a-z][^<]*)<`)
	politeChoiceEndRe   = regexp.MustCompile(`(하세요|하십시오|합니다|입니다|할까요|드리겠습니다)[.!?]?$`)
	fieldResidueRe       = regexp.MustCompile(`(?i)(prev_ko|next_ko|proposed_ko|risk|notes|id)"?\s*[:=]`)
	placeholderResidueRe = regexp.MustCompile(`(\[T\d+\]|\[\[/?E\d+\]\])`)
)

func validateRestoredOutput(meta itemMeta, restored string) error {
	if placeholderResidueRe.MatchString(restored) {
		return fmt.Errorf("placeholder residue found")
	}
	if isOverlayOrUISource(meta) {
		if !tokenCompatibleRelaxed(meta.sourceRaw, restored) {
			return fmt.Errorf("token mismatch after restore")
		}
	} else if !tokenCompatible(meta.sourceRaw, restored) {
		return fmt.Errorf("token mismatch after restore")
	}
	if meta.profile.Kind == textKindChoice {
		if meta.isStatCheck && meta.statCheck != "" {
			prefix := localizedStatCheckPrefix(meta.statCheck)
			if prefix != "" && !strings.HasPrefix(restored, prefix) {
				return fmt.Errorf("stat-check prefix not preserved")
			}
		} else {
			prefix := gameplayPrefixRe.FindString(meta.sourceRaw)
			if prefix != "" && !strings.HasPrefix(restored, prefix) {
				return fmt.Errorf("choice prefix not preserved")
			}
		}
		if politeChoiceEndRe.MatchString(strings.TrimSpace(restored)) {
			return fmt.Errorf("choice line ended in polite register")
		}
	}
	if meta.profile.HasRichText && !isOverlayOrUISource(meta) && lowerEnglishInTagRe.MatchString(restored) {
		if !sourceAlsoHasEnglishInTags(meta.sourceRaw, restored) {
			return fmt.Errorf("english prose remained inside rich-text tags")
		}
	}
	if fieldResidueRe.MatchString(restored) {
		return fmt.Errorf("structured field residue leaked into output")
	}
	if hasUnexpectedScriptGroup(restored, meta.sourceRaw) {
		return fmt.Errorf("unexpected foreign-script contamination")
	}
	return nil
}

func tokenCompatible(src, ko string) bool {
	// Structural tokens ($var, {template}) must match exactly
	srcStructural := structuralTokenRE.FindAllString(src, -1)
	koStructural := structuralTokenRE.FindAllString(ko, -1)
	if len(srcStructural) != len(koStructural) {
		return false
	}
	for i := range srcStructural {
		if srcStructural[i] != koStructural[i] {
			return false
		}
	}
	// HTML tags: only check that each unique tag appears the same number of times
	// (LLM may reorder tags during translation, which is acceptable)
	srcTags := htmlTagRE.FindAllString(src, -1)
	koTags := htmlTagRE.FindAllString(ko, -1)
	if len(srcTags) > 0 || len(koTags) > 0 {
		srcTagCounts := make(map[string]int)
		for _, t := range srcTags {
			srcTagCounts[t]++
		}
		koTagCounts := make(map[string]int)
		for _, t := range koTags {
			koTagCounts[t]++
		}
		for tag, cnt := range srcTagCounts {
			if koTagCounts[tag] != cnt {
				return false
			}
		}
		for tag, cnt := range koTagCounts {
			if srcTagCounts[tag] != cnt {
				return false
			}
		}
	}
	return true
}

// tagNameRE extracts just the tag name from an HTML-like tag (e.g. "link" from "<link=\"1\">")
var tagNameRE = regexp.MustCompile(`^</?([A-Za-z][A-Za-z0-9_-]*)`)

// tokenCompatibleRelaxed is a relaxed version for overlay/UI items.
// Structural tokens ($var, {template}) must still match exactly.
// HTML tags are compared by tag name only (ignoring attributes),
// so <#DB5B2CFF> vs <#DB5B2C44> or <link="1"> vs <link="3"> won't fail.
func tokenCompatibleRelaxed(src, ko string) bool {
	srcStructural := structuralTokenRE.FindAllString(src, -1)
	koStructural := structuralTokenRE.FindAllString(ko, -1)
	if len(srcStructural) != len(koStructural) {
		return false
	}
	for i := range srcStructural {
		if srcStructural[i] != koStructural[i] {
			return false
		}
	}
	// HTML tags: compare by tag name only, ignore attributes
	srcTags := htmlTagRE.FindAllString(src, -1)
	koTags := htmlTagRE.FindAllString(ko, -1)
	if len(srcTags) > 0 || len(koTags) > 0 {
		srcNames := make(map[string]int)
		for _, t := range srcTags {
			name := extractTagName(t)
			srcNames[name]++
		}
		koNames := make(map[string]int)
		for _, t := range koTags {
			name := extractTagName(t)
			koNames[name]++
		}
		for name, cnt := range srcNames {
			if koNames[name] != cnt {
				return false
			}
		}
		for name, cnt := range koNames {
			if srcNames[name] != cnt {
				return false
			}
		}
	}
	return true
}

func extractTagName(tag string) string {
	m := tagNameRE.FindStringSubmatch(tag)
	if len(m) >= 2 {
		return strings.ToLower(m[1])
	}
	// For color-only tags like <#DB5B2CFF>, normalize to "#color"
	if strings.HasPrefix(tag, "<#") || strings.HasPrefix(tag, "</#") {
		return "#color"
	}
	if strings.HasPrefix(tag, "</") {
		return "/" + tag[2:len(tag)-1]
	}
	return tag
}

// sourceAlsoHasEnglishInTags checks whether the English text found inside
// rich-text tags in the output also exists in the source. If so, it's likely
// a foreign language phrase (Latin, Finnish, etc.) or proper noun that should
// remain untranslated.
func isOverlayOrUISource(meta itemMeta) bool {
	if strings.HasPrefix(meta.sourceType, "overlay") {
		return true
	}
	switch meta.textRole {
	case "ui_label", "ui_description", "tooltip", "button", "flavor_text", "system_text", "description":
		return true
	}
	return false
}

func sourceAlsoHasEnglishInTags(src, restored string) bool {
	koMatches := lowerEnglishInTagRe.FindAllStringSubmatch(restored, -1)
	for _, m := range koMatches {
		fragment := strings.TrimSpace(m[1])
		if fragment == "" {
			continue
		}
		// If the source contains this same text, it's a passthrough
		if strings.Contains(src, fragment) {
			continue
		}
		// If the source is a literal passthrough source, allow it
		if isLiteralPassthroughSource(stripSimpleTags(src)) {
			continue
		}
		return false
	}
	return true
}
