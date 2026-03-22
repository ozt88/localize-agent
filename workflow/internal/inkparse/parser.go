package inkparse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// utf8BOM is the UTF-8 byte order mark.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Parse parses ink JSON data and returns dialogue blocks.
func Parse(data []byte, sourceFile string) (*ParseResult, error) {
	// Strip UTF-8 BOM if present
	data = bytes.TrimPrefix(data, utf8BOM)

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	rootArr, ok := root["root"].([]any)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'root' array")
	}

	result := &ParseResult{
		SourceFile: sourceFile,
	}

	w := &walker{
		sourceFile: sourceFile,
		result:     result,
		blockCount: map[string]int{},
	}

	// Walk root array for knot containers (dicts with knot names)
	for _, elem := range rootArr {
		if dict, ok := elem.(map[string]any); ok {
			for knotName, knotVal := range dict {
				if isMetaKey(knotName) {
					continue
				}
				if knotArr, ok := knotVal.([]any); ok {
					w.walkContainer(knotArr, knotName, "", "")
				}
			}
		}
	}

	return result, nil
}

// ParseFile reads a file and parses it.
func ParseFile(path string) (*ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return Parse(data, base)
}

// walker holds state during ink JSON tree traversal.
type walker struct {
	sourceFile string
	result     *ParseResult
	blockCount map[string]int // path -> count, for block index tracking
}

// walkContainer is the main recursive walker. It processes a container array,
// extracting text blocks and recursing into sub-containers.
func (w *walker) walkContainer(arr []any, knot, gate, choice string) {
	// Extract metadata from last element(s)
	meta := extractMeta(arr)

	// Check if this container has a name (#n) that updates gate/choice
	if meta != nil {
		if name, ok := meta["#n"].(string); ok {
			if strings.HasPrefix(name, "g-") {
				gate = name
			} else if strings.HasPrefix(name, "c-") {
				choice = name
			} else if name != "" {
				// Named hub container (like "hubFirstSide") — treat as gate
				gate = name
			}
		}
	}

	// Walk flat content (text, tags, control) in this container
	w.walkFlatContent(arr, knot, gate, choice)

	// Recurse into sub-arrays
	for _, elem := range arr {
		if subArr, ok := elem.([]any); ok {
			w.walkContainer(subArr, knot, gate, choice)
		}
	}

	// Recurse into named sub-containers in metadata dict (c-N, g-N, hubs)
	if meta != nil {
		for key, val := range meta {
			if isMetaKey(key) || isInternalKey(key) {
				continue
			}
			if subArr, ok := val.([]any); ok {
				if strings.HasPrefix(key, "c-") {
					w.walkContainer(subArr, knot, gate, key)
				} else if strings.HasPrefix(key, "g-") {
					w.walkContainer(subArr, knot, key, "")
				} else {
					// Named hub containers (e.g., "hubFirstSide")
					w.walkContainer(subArr, knot, key, "")
				}
			}
		}
	}
}

// walkFlatContent walks flat elements in a container array and extracts text blocks.
// Only processes string elements at this level (not sub-arrays).
func (w *walker) walkFlatContent(arr []any, knot, gate, choice string) {
	var textBuf strings.Builder
	var tags []string
	var speaker string
	inEv := false
	inStr := false
	inTag := false
	glueActive := false

	path := buildPath(knot, gate, choice)

	flushBlock := func() {
		text := textBuf.String()
		if text == "" {
			return
		}
		idx := w.blockCount[path]
		w.blockCount[path]++
		block := DialogueBlock{
			ID:         fmt.Sprintf("%s/blk-%d", path, idx),
			Path:       path,
			Text:       text,
			SourceHash: SourceHash(text),
			SourceFile: w.sourceFile,
			Knot:       knot,
			Gate:       gate,
			Choice:     choice,
			Speaker:    speaker,
			Tags:       tags,
			BlockIndex: idx,
		}
		w.result.Blocks = append(w.result.Blocks, block)
		w.result.TotalTextEntries++
		textBuf.Reset()
		tags = nil
		speaker = ""
	}

	for i := 0; i < len(arr); i++ {
		elem := arr[i]

		// Skip sub-arrays (handled by walkContainer recursion)
		if _, ok := elem.([]any); ok {
			continue
		}

		// Skip last element if it's metadata dict or null
		if i == len(arr)-1 {
			if _, ok := elem.(map[string]any); ok {
				break
			}
			if elem == nil {
				break
			}
		}
		// Also skip second-to-last null (metadata pair)
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
				glueActive = true
			case "\n":
				if textBuf.Len() > 0 {
					textBuf.WriteString("\n")
				}
			case "end", "done":
				// control flow markers, ignore
			default:
				if inTag {
					tagText := strings.TrimPrefix(v, "^")
					tagText = strings.TrimSpace(tagText)
					if tagText != "" {
						if isSpeakerTag(tagText) {
							speaker = tagText
						} else {
							tags = append(tags, tagText)
						}
					}
					continue
				}

				if strings.HasPrefix(v, "^") {
					text := v[1:] // strip ^ prefix
					if glueActive {
						glueActive = false
					}
					textBuf.WriteString(text)
				}
			}

		case map[string]any:
			if _, ok := v["->"]; ok {
				// Divert: end current block
				flushBlock()
				glueActive = false
			}
			// Also check for choice point: extract text from "s" sub-container
			if _, ok := v["*"]; ok {
				w.tryExtractChoiceText(arr[i:], knot, gate)
			}

		case nil:
			// Null element — skip
		}
	}

	// Flush remaining text
	flushBlock()
}

// tryExtractChoiceText extracts choice display text from a choice point definition.
// It looks for {"*":"path","flg":N} and nearby {"s":[...]} in the remaining elements.
func (w *walker) tryExtractChoiceText(remaining []any, knot, gate string) {
	var choicePath string
	var flg int
	var sContent []any

	for _, elem := range remaining {
		if dict, ok := elem.(map[string]any); ok {
			if star, ok := dict["*"].(string); ok {
				choicePath = star
				if f, ok := dict["flg"].(float64); ok {
					flg = int(f)
				}
			}
			if s, ok := dict["s"].([]any); ok {
				sContent = s
			}
		}
	}

	if choicePath == "" {
		return
	}

	// flg & 0x2 = has start content
	if flg&0x2 != 0 && sContent != nil {
		text := extractTextFromArray(sContent)
		if text != "" {
			choiceID := extractChoiceIDFromPath(choicePath)
			path := buildPath(knot, gate, choiceID)
			idx := w.blockCount[path]
			w.blockCount[path]++
			block := DialogueBlock{
				ID:         fmt.Sprintf("%s/blk-%d", path, idx),
				Path:       path,
				Text:       text,
				SourceHash: SourceHash(text),
				SourceFile: w.sourceFile,
				Knot:       knot,
				Gate:       gate,
				Choice:     choiceID,
				BlockIndex: idx,
			}
			w.result.Blocks = append(w.result.Blocks, block)
			w.result.TotalTextEntries++
		}
	}
}

// isSpeakerTag checks if a tag is a speaker/character name tag
// (vs a game command, check, conditional, or objective tag).
//
// Strategy: reject known non-speaker patterns, accept proper-case single words.
// Prefers false-positive speakers over missing character names, because
// downstream translation prompts benefit more from having speaker context
// (even occasionally wrong) than from missing it entirely.
func isSpeakerTag(tag string) bool {
	// DC/FC check tags
	if strings.HasPrefix(tag, "DC") || strings.HasPrefix(tag, "FC") {
		return false
	}
	// Conditional tags like ".DrummerIntro==1"
	if strings.HasPrefix(tag, ".") {
		return false
	}
	// ALL CAPS: OBJ, PCVFX, SFX, NPC, DEATH, etc.
	if len(tag) > 1 && tag == strings.ToUpper(tag) {
		return false
	}
	// Known ability score / role tags (lowercase)
	switch tag {
	case "speaker", "wis", "str", "int", "con", "dex", "cha", "reply":
		return true
	}
	// Reject game command patterns
	if isGameCommandTag(tag) {
		return false
	}
	// Accept proper-case single words as character names:
	// Starts with uppercase, rest lowercase, length >= 2
	if len(tag) >= 2 && tag[0] >= 'A' && tag[0] <= 'Z' {
		allLower := true
		for _, r := range tag[1:] {
			if r < 'a' || r > 'z' {
				allLower = false
				break
			}
		}
		if allLower {
			return true
		}
	}
	return false
}

// isGameCommandTag returns true for tags that are game engine commands,
// not character names. These use CamelCase, underscores, known prefixes,
// dice notation, numbers, or known non-speaker keywords.
func isGameCommandTag(tag string) bool {
	// CamelCase compound words: internal uppercase after lowercase
	// e.g., PlaySFX, CamFocus, FadeToBlack, AddItem, PCTrigger
	for i := 1; i < len(tag); i++ {
		if tag[i-1] >= 'a' && tag[i-1] <= 'z' && tag[i] >= 'A' && tag[i] <= 'Z' {
			return true
		}
	}
	// Contains underscore: vo_drummer, generic_fire, Ragn_Idle
	if strings.Contains(tag, "_") {
		return true
	}
	// Pure numbers: "1", "0", "3600"
	allDigit := len(tag) > 0
	for _, r := range tag {
		if r < '0' || r > '9' {
			allDigit = false
			break
		}
	}
	if allDigit {
		return true
	}
	// Dice notation: "1d4", "2d6", "3d10"
	if len(tag) >= 3 && tag[0] >= '0' && tag[0] <= '9' && strings.Contains(tag, "d") {
		return true
	}
	// Known non-speaker single words (not character names)
	switch tag {
	case "Minor", "Medium", "Major", "Crowns",
		"Death", "Attacking", "Casting", "Punching",
		"Biting", "Crawling", "Dancing", "Cower",
		"Combat", "Cutscene", "Appear", "Disappear",
		"Augury", "Attack":
		return true
	}
	return false
}

// extractMeta returns the metadata dict from a container array.
// ink containers have metadata as the last element (dict) or second-to-last
// if last is null.
func extractMeta(arr []any) map[string]any {
	if len(arr) == 0 {
		return nil
	}
	last := arr[len(arr)-1]
	if dict, ok := last.(map[string]any); ok {
		return dict
	}
	if last == nil && len(arr) >= 2 {
		if dict, ok := arr[len(arr)-2].(map[string]any); ok {
			return dict
		}
	}
	return nil
}

// extractTextFromArray extracts text from a simple array (for choice "s" content).
func extractTextFromArray(arr []any) string {
	var buf strings.Builder
	for _, elem := range arr {
		if s, ok := elem.(string); ok && strings.HasPrefix(s, "^") {
			buf.WriteString(s[1:])
		}
	}
	return buf.String()
}

// extractChoiceIDFromPath extracts the choice ID from a choice path string.
func extractChoiceIDFromPath(path string) string {
	parts := strings.Split(path, ".")
	for _, p := range parts {
		if strings.HasPrefix(p, "c-") {
			return p
		}
	}
	return ""
}

// buildPath builds a block path from knot, gate, choice.
func buildPath(knot, gate, choice string) string {
	parts := []string{knot}
	if gate != "" {
		parts = append(parts, gate)
	}
	if choice != "" {
		parts = append(parts, choice)
	}
	return strings.Join(parts, "/")
}

// isMetaKey returns true for ink metadata keys (not content keys).
func isMetaKey(key string) bool {
	return key == "#f" || key == "#n"
}

// isInternalKey returns true for ink internal keys that are not named containers.
// These include "s" (choice start content), "$r" style temp vars, etc.
func isInternalKey(key string) bool {
	if key == "s" {
		return true
	}
	if strings.HasPrefix(key, "$") {
		return true
	}
	if strings.HasPrefix(key, "^") {
		return true
	}
	return false
}
