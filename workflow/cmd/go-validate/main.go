package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type issue struct {
	ID      string `json:"id"`
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

var (
	tagRe   = regexp.MustCompile(`\{[^{}]*\}`)
	mfTagRe = regexp.MustCompile(`\{mf\|[^{}]+\|[^{}]+\}`)
)

var dntTerms = []string{"Rogue Trader", "Overseer", "Servo-Skull"}

var fallbackGlossary = map[string]string{
	"Willpower":    "의지력",
	"Toughness":    "강인함",
	"Perception":   "지각력",
	"Fellowship":   "친화력",
	"Intelligence": "지능",
	"Strength":     "근력",
	"Agility":      "민첩성",
}

func loadStrings(path string) (map[string]map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	s, ok := root["strings"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid file: %s (missing 'strings' object)", path)
	}
	out := map[string]map[string]any{}
	for k, v := range s {
		if m, ok := v.(map[string]any); ok {
			out[k] = m
		}
	}
	return out, nil
}

func readIDs(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := []string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		t := strings.TrimSpace(sc.Text())
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		out = append(out, t)
	}
	return out, sc.Err()
}

func extractTags(text string) []string {
	return tagRe.FindAllString(text, -1)
}

func extractNonMFTags(text string) []string {
	tags := extractTags(text)
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if !mfTagRe.MatchString(t) {
			out = append(out, t)
		}
	}
	return out
}

func maskTags(text string) string {
	return tagRe.ReplaceAllString(text, "")
}

func hasPunctSpacingIssue(text string) bool {
	outside := maskTags(text)
	exemptNext := map[rune]bool{']': true, ')': true, '}': true, '"': true, '\'': true, '.': true, ',': true, '!': true, '?': true, ';': true, ':': true}
	r := []rune(outside)
	for i, ch := range r {
		if ch != '.' && ch != '!' && ch != '?' {
			continue
		}
		if i == len(r)-1 {
			continue
		}
		nxt := r[i+1]
		if nxt == ' ' || nxt == '\n' || nxt == '\t' || exemptNext[nxt] {
			continue
		}
		return true
	}
	return false
}

func containsENWord(text, word string) bool {
	if strings.TrimSpace(word) == "" {
		return false
	}
	lowerText := strings.ToLower(text)
	lowerWord := strings.ToLower(word)
	from := 0
	for {
		idx := strings.Index(lowerText[from:], lowerWord)
		if idx < 0 {
			return false
		}
		start := from + idx
		end := start + len(lowerWord)
		leftOK := start == 0 || !isASCIIAlpha(rune(lowerText[start-1]))
		rightOK := end >= len(lowerText) || !isASCIIAlpha(rune(lowerText[end]))
		if leftOK && rightOK {
			return true
		}
		from = start + 1
	}
}

func isASCIIAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func evaluateToneWarning(text string) bool {
	outside := strings.TrimSpace(maskTags(text))
	if outside == "" {
		return false
	}
	endings := []string{"다.", "다!", "다?", "한다.", "된다."}
	for _, e := range endings {
		if strings.HasSuffix(outside, e) {
			return true
		}
	}
	return false
}

func hasLiteralStyleIssue(ko string) bool {
	outside := maskTags(ko)
	bad := []string{"\\n", "각하(각하)", "당신의,", "표류를 시작", "넓고 다소 광기 어린", "방어도 뒤에 숨겨진"}
	for _, b := range bad {
		if strings.Contains(outside, b) {
			return true
		}
	}
	return regexp.MustCompile(`\bRogue Trader\b`).FindStringIndex(outside) != nil
}

func hasContextualGlossaryIssue(en, ko string) bool {
	el := strings.ToLower(en)
	if strings.Contains(el, "armour of affected indifference") && strings.Contains(ko, "방어도") {
		return true
	}
	if strings.Contains(el, "for our {mf|lord|lady} von valancius") && strings.Contains(ko, "가문을 위해") {
		return true
	}
	if strings.Contains(el, "non-essential systems") && strings.Contains(ko, "비필수") {
		return true
	}
	return false
}

func hasHonorificStyleIssue(en, ko string) bool {
	hasLordship := strings.Contains(en, "{mf|Lordship|Ladyship}")
	hasLord := strings.Contains(en, "{mf|Lord|Lady}")
	if !hasLordship && !hasLord {
		return false
	}
	if regexp.MustCompile(`각하\s*\{mf\|Lord\|Lady\}(께서|께서는)?`).FindStringIndex(ko) != nil {
		return true
	}
	if regexp.MustCompile(`각하\s*\{mf\|Lordship\|Ladyship\}`).FindStringIndex(ko) != nil {
		return true
	}
	if hasLord {
		return false
	}
	if strings.Contains(ko, "각하") || strings.Contains(ko, "{mf|각하|각하}") {
		return false
	}
	if strings.Contains(ko, "당신의 {mf|Lordship|Ladyship}") {
		return true
	}
	return true
}

func flattenGlossary(path string) map[string]string {
	out := map[string]string{}
	raw, err := os.ReadFile(path)
	if err != nil {
		for k, v := range fallbackGlossary {
			out[k] = v
		}
		return out
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		for k, v := range fallbackGlossary {
			out[k] = v
		}
		return out
	}
	for _, v := range root {
		sec, ok := v.(map[string]any)
		if !ok {
			continue
		}
		for en, koRaw := range sec {
			ko, ok := koRaw.(string)
			if !ok {
				continue
			}
			if strings.TrimSpace(en) == "" || strings.TrimSpace(ko) == "" {
				continue
			}
			if _, exists := out[en]; !exists {
				out[en] = ko
			}
		}
	}
	for k, v := range fallbackGlossary {
		if _, ok := out[k]; !ok {
			out[k] = v
		}
	}
	return out
}

func validateEntry(entryID, en, ko string, glossary map[string]string) []issue {
	issues := []issue{}

	enTags := extractNonMFTags(en)
	koTags := extractNonMFTags(ko)
	if len(enTags) != len(koTags) {
		issues = append(issues, issue{ID: entryID, Level: "error", Code: "TAG_MISMATCH", Message: "Tag/placeholder sequence must be identical to EN source."})
	} else {
		for i := range enTags {
			if enTags[i] != koTags[i] {
				issues = append(issues, issue{ID: entryID, Level: "error", Code: "TAG_MISMATCH", Message: "Tag/placeholder sequence must be identical to EN source."})
				break
			}
		}
	}

	if hasPunctSpacingIssue(ko) {
		issues = append(issues, issue{ID: entryID, Level: "error", Code: "PUNCT_SPACE", Message: "Punctuation spacing error found outside tags."})
	}

	for _, term := range dntTerms {
		if strings.Contains(en, term) && !strings.Contains(ko, term) {
			issues = append(issues, issue{ID: entryID, Level: "error", Code: "DNT_TERM", Message: fmt.Sprintf("Do-not-translate term missing in KO: %s", term)})
		}
	}

	for enTerm, koTerm := range glossary {
		if containsENWord(en, enTerm) {
			if enTerm == "von Valancius" && strings.Contains(strings.ToLower(en), "for our {mf|lord|lady} von valancius") {
				if strings.Contains(ko, "폰 발란시우스 각하") {
					continue
				}
			}
			if enTerm == "von Valancius" && strings.Contains(ko, "폰 발란시우스") {
				continue
			}
			if !strings.Contains(ko, koTerm) {
				issues = append(issues, issue{ID: entryID, Level: "error", Code: "GLOSSARY", Message: fmt.Sprintf("Glossary violation: %s -> %s", enTerm, koTerm)})
			}
		}
	}

	if evaluateToneWarning(ko) {
		issues = append(issues, issue{ID: entryID, Level: "warning", Code: "TONE_WARN", Message: "Potential non-honorific sentence ending detected."})
	}
	if hasLiteralStyleIssue(ko) {
		issues = append(issues, issue{ID: entryID, Level: "warning", Code: "LITERAL_STYLE_WARN", Message: "Potential literal/awkward Korean pattern detected."})
	}
	if hasContextualGlossaryIssue(en, ko) {
		issues = append(issues, issue{ID: entryID, Level: "warning", Code: "GLOSSARY_NATURALNESS_WARN", Message: "Glossary-applied wording may sound unnatural in this context."})
	}
	if hasHonorificStyleIssue(en, ko) {
		issues = append(issues, issue{ID: entryID, Level: "warning", Code: "HONORIFIC_STYLE", Message: "Use natural honorific phrasing for {mf|Lord|Lady}/{mf|Lordship|Ladyship} context."})
	}

	return issues
}

func writeReport(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func main() {
	var sourceEN string
	var targetKO string
	var idsFile string
	var report string
	var failOnWarn bool
	var glossaryFile string

	flag.StringVar(&sourceEN, "source-en", "enGB_original.json", "")
	flag.StringVar(&targetKO, "target-ko", "", "")
	flag.StringVar(&idsFile, "ids", "", "")
	flag.StringVar(&report, "report", "workflow/output/validation_report.json", "")
	flag.BoolVar(&failOnWarn, "fail-on-warn", false, "")
	flag.StringVar(&glossaryFile, "glossary-file", "workflow/context/universal_glossary.json", "")
	flag.Parse()

	if strings.TrimSpace(targetKO) == "" {
		fmt.Fprintln(os.Stderr, "--target-ko is required")
		os.Exit(2)
	}

	src, err := loadStrings(sourceEN)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	tgt, err := loadStrings(targetKO)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	glossary := flattenGlossary(glossaryFile)

	ids := []string{}
	if strings.TrimSpace(idsFile) != "" {
		ids, err = readIDs(idsFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	} else {
		ids = make([]string, 0, len(tgt))
		for k := range tgt {
			ids = append(ids, k)
		}
	}

	issues := []issue{}
	for _, entryID := range ids {
		enText := ""
		if srcEntry, ok := src[entryID]; ok {
			if v, ok := srcEntry["Text"].(string); ok {
				enText = v
			}
		}
		koText := ""
		if tgtEntry, ok := tgt[entryID]; ok {
			if v, ok := tgtEntry["Text"].(string); ok {
				koText = v
			}
		}
		issues = append(issues, validateEntry(entryID, enText, koText, glossary)...)
	}

	errCount := 0
	warnCount := 0
	for _, it := range issues {
		if it.Level == "error" {
			errCount++
		} else if it.Level == "warning" {
			warnCount++
		}
	}

	output := map[string]any{
		"summary": map[string]any{
			"validated_count": len(ids),
			"errors":          errCount,
			"warnings":        warnCount,
			"ok":              errCount == 0 && (!failOnWarn || warnCount == 0),
		},
		"issues": issues,
	}
	if err := writeReport(report, output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("Validation report written: %s\n", report)
	fmt.Printf("errors=%d, warnings=%d\n", errCount, warnCount)

	if errCount > 0 {
		os.Exit(2)
	}
	if failOnWarn && warnCount > 0 {
		os.Exit(3)
	}
}
