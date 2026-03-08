package translation

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	tokenRE             = regexp.MustCompile(`(\$[A-Za-z0-9_]+|<[^>]+>|\{[^{}]+\})`)
	lowerEnglishInTagRe = regexp.MustCompile(`>([^<]*[a-z][a-z][^<]*)<`)
	politeChoiceEndRe   = regexp.MustCompile(`(하세요|하십시오|합니다|입니다|할까요|드리겠습니다)[.!?]?$`)
	fieldResidueRe      = regexp.MustCompile(`(?i)(prev_ko|next_ko|proposed_ko|risk|notes|id)"?\s*[:=]`)
)

func validateRestoredOutput(meta itemMeta, restored string) error {
	if strings.Contains(restored, "[T") || strings.Contains(restored, "]") {
		return fmt.Errorf("placeholder residue found")
	}
	if !tokenCompatible(meta.sourceRaw, restored) {
		return fmt.Errorf("token mismatch after restore")
	}
	if meta.profile.Kind == textKindChoice {
		prefix := gameplayPrefixRe.FindString(meta.sourceRaw)
		if prefix != "" && !strings.HasPrefix(restored, prefix) {
			return fmt.Errorf("choice prefix not preserved")
		}
		if politeChoiceEndRe.MatchString(strings.TrimSpace(restored)) {
			return fmt.Errorf("choice line ended in polite register")
		}
	}
	if meta.profile.HasRichText && lowerEnglishInTagRe.MatchString(restored) {
		return fmt.Errorf("english prose remained inside rich-text tags")
	}
	if fieldResidueRe.MatchString(restored) {
		return fmt.Errorf("structured field residue leaked into output")
	}
	return nil
}

func tokenCompatible(src, ko string) bool {
	srcTokens := tokenRE.FindAllString(src, -1)
	koTokens := tokenRE.FindAllString(ko, -1)
	if len(srcTokens) != len(koTokens) {
		return false
	}
	for i := range srcTokens {
		if srcTokens[i] != koTokens[i] {
			return false
		}
	}
	if strings.Count(src, "\n") != strings.Count(ko, "\n") {
		return false
	}
	return true
}
