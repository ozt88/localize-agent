package translation

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	tagRe         = regexp.MustCompile(`(\$[A-Za-z0-9_]+|<[^>]+>|\{[^}]+\})`)
	placeholderRe = regexp.MustCompile(`\[T\d+\]`)
)

func maskTags(text string) (string, []mapping) {
	maps := []mapping{}
	idx := 0
	masked := tagRe.ReplaceAllStringFunc(text, func(s string) string {
		p := fmt.Sprintf("[T%d]", idx)
		maps = append(maps, mapping{placeholder: p, original: s})
		idx++
		return p
	})
	return masked, maps
}

func verifyPlaceholders(text string, maps []mapping) error {
	found := placeholderRe.FindAllString(text, -1)
	if len(found) != len(maps) {
		return fmt.Errorf("placeholder count mismatch: expected %d, found %d", len(maps), len(found))
	}
	for i := range maps {
		if found[i] != maps[i].placeholder {
			return fmt.Errorf("placeholder order mismatch at index %d", i)
		}
	}
	return nil
}

func restoreTags(text string, maps []mapping) (string, error) {
	if err := verifyPlaceholders(text, maps); err != nil {
		return "", err
	}
	out := text
	for _, m := range maps {
		out = strings.ReplaceAll(out, m.placeholder, m.original)
	}
	return out, nil
}

func maskNoErr(text string) string {
	masked, _ := maskTags(text)
	return masked
}
