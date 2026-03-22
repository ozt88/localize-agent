package inkparse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// InjectReport tracks the results of ink JSON translation injection.
type InjectReport struct {
	SourceFile string // source file name
	Total      int    // total text blocks encountered
	Replaced   int    // blocks with translation applied
	Missing    int    // blocks with no translation found
}

// InjectTranslations replaces "^text" nodes in ink JSON with Korean translations.
// It walks the same tree structure as Parse(), computing SourceHash for each text block,
// then looks up the hash in the translations map (source_hash -> ko_formatted).
// Returns modified JSON bytes (with BOM), an InjectReport, and any error.
func InjectTranslations(data []byte, sourceFile string, translations map[string]string) ([]byte, *InjectReport, error) {
	// Strip UTF-8 BOM if present
	data = bytes.TrimPrefix(data, utf8BOM)

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("unmarshal: %w", err)
	}

	rootArr, ok := root["root"].([]any)
	if !ok {
		return nil, nil, fmt.Errorf("missing or invalid 'root' array")
	}

	report := &InjectReport{
		SourceFile: sourceFile,
	}

	inj := &injector{
		translations: translations,
		report:       report,
		blockCount:   map[string]int{},
	}

	// Walk root array for knot containers (same traversal as parser.go)
	for _, elem := range rootArr {
		if dict, ok := elem.(map[string]any); ok {
			for knotName, knotVal := range dict {
				if isMetaKey(knotName) {
					continue
				}
				if knotArr, ok := knotVal.([]any); ok {
					inj.walkContainer(knotArr, knotName, "", "")
				}
			}
		}
	}

	// Marshal modified tree back to JSON
	jsonBytes, err := json.Marshal(root)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal: %w", err)
	}

	// Prepend UTF-8 BOM
	out := append([]byte{0xEF, 0xBB, 0xBF}, jsonBytes...)
	return out, report, nil
}

// injector holds state during injection tree traversal.
type injector struct {
	translations map[string]string
	report       *InjectReport
	blockCount   map[string]int
}

// walkContainer mirrors parser.go walkContainer for injection.
func (inj *injector) walkContainer(arr []any, knot, gate, choice string) {
	meta := extractMeta(arr)

	if meta != nil {
		if name, ok := meta["#n"].(string); ok {
			if strings.HasPrefix(name, "g-") {
				gate = name
			} else if strings.HasPrefix(name, "c-") {
				choice = name
			} else if name != "" {
				gate = name
			}
		}
	}

	// Walk flat content for text replacement
	inj.walkFlatContent(arr, knot, gate, choice)

	// Recurse into sub-arrays
	for _, elem := range arr {
		if subArr, ok := elem.([]any); ok {
			inj.walkContainer(subArr, knot, gate, choice)
		}
	}

	// Recurse into named sub-containers in metadata dict
	if meta != nil {
		for key, val := range meta {
			if isMetaKey(key) || isInternalKey(key) {
				continue
			}
			if subArr, ok := val.([]any); ok {
				if strings.HasPrefix(key, "c-") {
					inj.walkContainer(subArr, knot, gate, key)
				} else if strings.HasPrefix(key, "g-") {
					inj.walkContainer(subArr, knot, key, "")
				} else {
					inj.walkContainer(subArr, knot, key, "")
				}
			}
		}
	}
}

// walkFlatContent mirrors parser.go walkFlatContent but replaces "^text" nodes.
func (inj *injector) walkFlatContent(arr []any, knot, gate, choice string) {
	// Collect text node indices for the current block
	var textNodeIndices []int
	var textBuf strings.Builder
	inEv := false
	inStr := false
	inTag := false

	path := buildPath(knot, gate, choice)

	flushBlock := func() {
		text := textBuf.String()
		if text == "" {
			textNodeIndices = nil
			return
		}

		hash := SourceHash(text)
		inj.report.Total++
		inj.blockCount[path]++

		if ko, ok := inj.translations[hash]; ok {
			// Replace: first node gets full Korean text, rest get "^"
			if len(textNodeIndices) > 0 {
				arr[textNodeIndices[0]] = "^" + ko
				for _, idx := range textNodeIndices[1:] {
					arr[idx] = "^"
				}
			}
			inj.report.Replaced++
		} else {
			inj.report.Missing++
		}

		textBuf.Reset()
		textNodeIndices = nil
	}

	for i := 0; i < len(arr); i++ {
		elem := arr[i]

		// Skip sub-arrays
		if _, ok := elem.([]any); ok {
			continue
		}

		// Skip last element if metadata dict or null
		if i == len(arr)-1 {
			if _, ok := elem.(map[string]any); ok {
				break
			}
			if elem == nil {
				break
			}
		}
		// Skip second-to-last null (metadata pair)
		if i == len(arr)-2 && elem == nil {
			if _, ok := arr[len(arr)-1].(map[string]any); ok {
				break
			}
		}

		switch v := elem.(type) {
		case string:
			if inEv {
				if v == "/ev" {
					inEv = false
				}
				continue
			}
			if inStr {
				if v == "/str" {
					inStr = false
				}
				continue
			}

			switch v {
			case "ev":
				inEv = true
			case "/ev":
				inEv = false
			case "str":
				inStr = true
			case "/str":
				inStr = false
			case "#":
				inTag = true
			case "/#":
				inTag = false
			case "<>":
				// glue, skip
			case "\n":
				if textBuf.Len() > 0 {
					textBuf.WriteString("\n")
				}
			case "end", "done":
				// control flow markers
			default:
				if inTag {
					continue
				}
				if strings.HasPrefix(v, "^") {
					text := v[1:]
					textBuf.WriteString(text)
					textNodeIndices = append(textNodeIndices, i)
				}
			}

		case map[string]any:
			if _, ok := v["->"]; ok {
				// Divert: flush current block
				flushBlock()
			}

		case nil:
			// skip
		}
	}

	// Flush remaining text
	flushBlock()
}
