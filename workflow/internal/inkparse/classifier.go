package inkparse

import "strings"

// Classify determines the content type of a dialogue block based on
// source file name patterns, tag metadata, and structural signals.
// Returns one of the Content* constants.
func Classify(block *DialogueBlock) string {
	// Priority 1: Tag-based classification (strongest semantic signal)
	if hasSpellTag(block.Tags) {
		return ContentSpell
	}
	if hasItemTag(block.Tags) && isItemFilePrefix(block.SourceFile) {
		return ContentItem
	}

	// Priority 2: File name prefix patterns
	prefix := filePrefix(block.SourceFile)
	switch prefix {
	case "TS", "AR", "CB", "EP", "DS", "DN", "Q", "LL", "GG", "GW",
		"JC", "RM", "SH", "ST", "WL", "VL", "OP", "LP", "SO", "PC",
		"UP", "Snell", "Ettir", "Meek", "Default":
		return ContentDialogue
	case "TU":
		// TU_ = Tutorial files
		return ContentSystem
	case "TE":
		// TE_ = Test/system features
		return ContentSystem
	}

	// Priority 3: Structural signals
	if block.Speaker != "" {
		return ContentDialogue
	}

	// Short text without speaker or DC tags -> UI label candidate
	if len([]rune(block.Text)) < 50 && block.Speaker == "" && !hasDCTag(block.Tags) {
		return ContentUI
	}

	// Default: dialogue (most content in this game is narrative)
	return ContentDialogue
}

// filePrefix extracts the prefix before the first underscore in a filename.
func filePrefix(name string) string {
	idx := strings.IndexByte(name, '_')
	if idx > 0 {
		return name[:idx]
	}
	return name
}

// hasSpellTag checks if any tag indicates spell/ability content.
func hasSpellTag(tags []string) bool {
	for _, t := range tags {
		lower := strings.ToLower(t)
		if lower == "spell" || lower == "ability" || strings.HasPrefix(lower, "spell_") || strings.HasPrefix(lower, "ability_") {
			return true
		}
	}
	return false
}

// hasItemTag checks if any tag indicates item/object content.
func hasItemTag(tags []string) bool {
	for _, t := range tags {
		if t == "OBJ" || strings.HasPrefix(t, "ITEM") || strings.HasPrefix(t, "LOOT") {
			return true
		}
	}
	return false
}

// isItemFilePrefix checks if the source file prefix suggests item/encounter content.
func isItemFilePrefix(name string) bool {
	prefix := filePrefix(name)
	return prefix == "Enc" || prefix == "CB"
}

// hasDCTag checks if any tag is a DC (difficulty check) tag.
func hasDCTag(tags []string) bool {
	for _, t := range tags {
		if strings.HasPrefix(t, "DC") || strings.HasPrefix(t, "FC") {
			return true
		}
	}
	return false
}
