package semanticreview

import (
	"math"
	"regexp"
	"strings"
)

var (
	wordRe         = regexp.MustCompile(`[A-Za-z0-9_']+`)
	fieldResidueRe = regexp.MustCompile(`(?i)(prev_ko|next_ko|proposed_ko|risk|notes|id)"?\s*[:=]`)
)

func lexicalSimilarity(a, b string) float64 {
	ta := tokenSet(a)
	tb := tokenSet(b)
	if len(ta) == 0 && len(tb) == 0 {
		return 1
	}
	inter := 0
	union := map[string]struct{}{}
	for k := range ta {
		union[k] = struct{}{}
		if _, ok := tb[k]; ok {
			inter++
		}
	}
	for k := range tb {
		union[k] = struct{}{}
	}
	return float64(inter) / float64(len(union))
}

func alignmentPenalty(sourceEN, neighborEN, backEN string) float64 {
	if strings.TrimSpace(neighborEN) == "" {
		return 0
	}
	srcSim := lexicalSimilarity(sourceEN, backEN)
	neiSim := lexicalSimilarity(neighborEN, backEN)
	if neiSim <= srcSim {
		return 0
	}
	return math.Round((neiSim-srcSim)*10000) / 10000
}

func formatPenalty(ko string) float64 {
	if fieldResidueRe.MatchString(ko) {
		return 1.0
	}
	return 0
}

func buildReportItem(item ReviewItem, backEN string, semanticSimilarity float64) ReportItem {
	scoreSemantic := 1 - semanticSimilarity
	scoreLexical := 1 - lexicalSimilarity(item.SourceEN, backEN)
	scorePrev := alignmentPenalty(item.SourceEN, item.PrevEN, backEN)
	scoreNext := alignmentPenalty(item.SourceEN, item.NextEN, backEN)
	scoreFormat := formatPenalty(item.TranslatedKO)

	final := scoreSemantic*0.45 + scoreLexical*0.25 + scorePrev*0.15 + scoreNext*0.15 + scoreFormat

	var tags []string
	if scoreSemantic >= 0.35 {
		tags = append(tags, "semantic_drift")
	}
	if scoreLexical >= 0.35 {
		tags = append(tags, "lexical_drift")
	}
	if scorePrev > 0 {
		tags = append(tags, "closer_to_prev")
	}
	if scoreNext > 0 {
		tags = append(tags, "closer_to_next")
	}
	if scoreFormat > 0 {
		tags = append(tags, "format_residue")
	}

	return ReportItem{
		ID:                 item.ID,
		SourceEN:           item.SourceEN,
		TranslatedKO:       item.TranslatedKO,
		BacktranslatedEN:   backEN,
		ScoreSemantic:      round4(scoreSemantic),
		ScoreLexical:       round4(scoreLexical),
		ScorePrevAlignment: round4(scorePrev),
		ScoreNextAlignment: round4(scoreNext),
		ScoreFinal:         round4(final),
		ReasonTags:         tags,
	}
}

func buildDirectScoreReportItem(item ReviewItem, score directScoreResult) ReportItem {
	translatedKO := item.FreshKO
	if translatedKO == "" {
		translatedKO = item.TranslatedKO
	}
	if item.CurrentKO != "" && score.CurrentScore >= score.FreshScore {
		translatedKO = item.CurrentKO
	}
	report := ReportItem{
		ID:           item.ID,
		SourceEN:     item.SourceEN,
		TranslatedKO: translatedKO,
		ScoreFinal:   round4(maxFloat(score.CurrentScore, score.FreshScore)),
		CurrentScore: round4(score.CurrentScore),
		FreshScore:   round4(score.FreshScore),
	}
	if len(score.ReasonTags) > 0 {
		report.ReasonTags = append([]string(nil), score.ReasonTags...)
	}
	if strings.TrimSpace(score.ShortReason) != "" {
		report.ShortReason = strings.TrimSpace(score.ShortReason)
	}
	return report
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func tokenSet(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, m := range wordRe.FindAllString(strings.ToLower(s), -1) {
		out[m] = struct{}{}
	}
	return out
}
