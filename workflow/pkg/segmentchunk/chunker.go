package segmentchunk

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
)

type segment struct {
	SegmentID     string   `json:"segment_id"`
	SourceFile    string   `json:"source_file"`
	SceneHint     string   `json:"scene_hint"`
	BlockKind     string   `json:"block_kind"`
	ChoiceBlockID *string  `json:"choice_block_id"`
	SegmentSize   int      `json:"segment_size"`
	SourceText    string   `json:"source_text"`
	LineIDs       []string `json:"line_ids"`
	SourceLines   []string `json:"source_lines"`
	TextRoles     []string `json:"text_roles"`
	SpeakerHints  []string `json:"speaker_hints"`
	MetaPathLabel string   `json:"meta_path_label"`
}

type chunk struct {
	ChunkID         string   `json:"chunk_id"`
	ParentSegmentID string   `json:"parent_segment_id"`
	ChunkPos        int      `json:"chunk_pos"`
	ChunkCount      int      `json:"chunk_count"`
	SourceFile      string   `json:"source_file"`
	SceneHint       string   `json:"scene_hint"`
	BlockKind       string   `json:"block_kind"`
	ChoiceBlockID   *string  `json:"choice_block_id"`
	SourceText      string   `json:"source_text"`
	LineIDs         []string `json:"line_ids"`
	SourceLines     []string `json:"source_lines"`
	TextRoles       []string `json:"text_roles"`
	SpeakerHints    []string `json:"speaker_hints"`
	MetaPathLabel   string   `json:"meta_path_label"`
}

type TranslatorPackage struct {
	Format       string              `json:"format"`
	Instructions packageInstructions `json:"instructions"`
	Segments     []packageSegment    `json:"segments"`
}

type packageInstructions struct {
	TranslateUnit       string `json:"translate_unit"`
	ReturnUnit          string `json:"return_unit"`
	RequiredOutputShape any    `json:"required_output_shape,omitempty"`
	Rules               []string `json:"rules,omitempty"`
}

type packageSegment struct {
	SegmentID     string        `json:"segment_id"`
	SourceFile    string        `json:"source_file"`
	SceneHint     string        `json:"scene_hint"`
	BlockKind     string        `json:"block_kind"`
	ChoiceBlockID *string       `json:"choice_block_id"`
	SegmentSize   int           `json:"segment_size"`
	SourceText    string        `json:"source_text"`
	Lines         []packageLine `json:"lines"`
}

type packageLine struct {
	LineID                      string   `json:"line_id"`
	SegmentPos                  int      `json:"segment_pos"`
	SourceText                  string   `json:"source_text"`
	TextRole                    string   `json:"text_role"`
	SpeakerHint                 *string  `json:"speaker_hint"`
	PrevLineID                  *string  `json:"prev_line_id"`
	NextLineID                  *string  `json:"next_line_id"`
	LineIsShortContextDependent bool     `json:"line_is_short_context_dependent"`
	LineHasEmphasis             bool     `json:"line_has_emphasis"`
	LineIsImperative            bool     `json:"line_is_imperative"`
	Tags                        []string `json:"tags"`
}

type ChunkedTranslatorPackage struct {
	Format       string              `json:"format"`
	Instructions packageInstructions `json:"instructions"`
	Chunks       []packageChunk      `json:"chunks"`
}

type packageChunk struct {
	ChunkID         string        `json:"chunk_id"`
	ParentSegmentID string        `json:"parent_segment_id"`
	ChunkPos        int           `json:"chunk_pos"`
	ChunkCount      int           `json:"chunk_count"`
	SourceFile      string        `json:"source_file"`
	SceneHint       string        `json:"scene_hint"`
	BlockKind       string        `json:"block_kind"`
	ChoiceBlockID   *string       `json:"choice_block_id"`
	SourceText      string        `json:"source_text"`
	Lines           []packageLine `json:"lines"`
}

type config struct {
	MaxLines int
	MinLines int
}

func DefaultConfig() config {
	return config{
		MaxLines: 5,
		MinLines: 2,
	}
}

func buildChunks(segments []segment, cfg config) []chunk {
	if cfg.MaxLines <= 0 {
		cfg.MaxLines = 5
	}
	if cfg.MinLines <= 0 {
		cfg.MinLines = 2
	}
	out := make([]chunk, 0, len(segments))
	for _, seg := range segments {
		out = append(out, chunkSegment(seg, cfg)...)
	}
	return out
}

func BuildTranslatorPackageChunks(pkg TranslatorPackage, cfg config) ChunkedTranslatorPackage {
	chunks := make([]packageChunk, 0, len(pkg.Segments))
	for _, seg := range pkg.Segments {
		chunks = append(chunks, chunkPackageSegment(seg, cfg)...)
	}
	instructions := pkg.Instructions
	instructions.TranslateUnit = "chunk"
	instructions.Rules = appendUniqueRule(instructions.Rules,
		"Each chunk is a contiguous slice of a parent segment.",
		"Translate with chunk context, but return translations aligned by line_id.",
	)
	return ChunkedTranslatorPackage{
		Format:       "esoteric-ebb-translator-package-chunked.v1",
		Instructions: instructions,
		Chunks:       chunks,
	}
}

func chunkSegment(seg segment, cfg config) []chunk {
	if len(seg.SourceLines) == 0 || len(seg.LineIDs) == 0 {
		return nil
	}
	if seg.BlockKind == "choice_block" || len(seg.SourceLines) <= cfg.MaxLines {
		return []chunk{makeChunk(seg, 0, len(seg.SourceLines), 1, 1)}
	}

	ranges := make([][2]int, 0, (len(seg.SourceLines)+cfg.MaxLines-1)/cfg.MaxLines)
	start := 0
	for i := 1; i < len(seg.SourceLines); i++ {
		curLen := i - start
		if curLen < cfg.MinLines {
			continue
		}
		if curLen >= cfg.MaxLines {
			ranges = append(ranges, [2]int{start, i})
			start = i
			continue
		}
		if shouldBreak(seg, i, curLen, cfg.MaxLines) {
			ranges = append(ranges, [2]int{start, i})
			start = i
		}
	}
	if start < len(seg.SourceLines) {
		ranges = append(ranges, [2]int{start, len(seg.SourceLines)})
	}

	chunks := make([]chunk, 0, len(ranges))
	for idx, r := range ranges {
		chunks = append(chunks, makeChunk(seg, r[0], r[1], idx+1, len(ranges)))
	}
	return chunks
}

func chunkPackageSegment(seg packageSegment, cfg config) []packageChunk {
	internal := segment{
		SegmentID:     seg.SegmentID,
		SourceFile:    seg.SourceFile,
		SceneHint:     seg.SceneHint,
		BlockKind:     seg.BlockKind,
		ChoiceBlockID: seg.ChoiceBlockID,
		SegmentSize:   seg.SegmentSize,
		SourceText:    seg.SourceText,
		LineIDs:       make([]string, 0, len(seg.Lines)),
		SourceLines:   make([]string, 0, len(seg.Lines)),
		TextRoles:     make([]string, 0, len(seg.Lines)),
		SpeakerHints:  make([]string, 0, len(seg.Lines)),
	}
	lineMap := make(map[string]packageLine, len(seg.Lines))
	for _, line := range seg.Lines {
		internal.LineIDs = append(internal.LineIDs, line.LineID)
		internal.SourceLines = append(internal.SourceLines, line.SourceText)
		internal.TextRoles = append(internal.TextRoles, line.TextRole)
		if line.SpeakerHint != nil {
			internal.SpeakerHints = append(internal.SpeakerHints, *line.SpeakerHint)
		} else {
			internal.SpeakerHints = append(internal.SpeakerHints, "")
		}
		lineMap[line.LineID] = line
	}

	baseChunks := chunkSegment(internal, cfg)
	out := make([]packageChunk, 0, len(baseChunks))
	for _, c := range baseChunks {
		lines := make([]packageLine, 0, len(c.LineIDs))
		for _, lineID := range c.LineIDs {
			line := lineMap[lineID]
			lines = append(lines, line)
		}
		out = append(out, packageChunk{
			ChunkID:         c.ChunkID,
			ParentSegmentID: c.ParentSegmentID,
			ChunkPos:        c.ChunkPos,
			ChunkCount:      c.ChunkCount,
			SourceFile:      c.SourceFile,
			SceneHint:       c.SceneHint,
			BlockKind:       c.BlockKind,
			ChoiceBlockID:   c.ChoiceBlockID,
			SourceText:      c.SourceText,
			Lines:           lines,
		})
	}
	return out
}

func shouldBreak(seg segment, idx, curLen, maxLines int) bool {
	if curLen >= maxLines {
		return true
	}
	prevRole := roleAt(seg.TextRoles, idx-1)
	curRole := roleAt(seg.TextRoles, idx)
	prevSpeaker := hintAt(seg.SpeakerHints, idx-1)
	curSpeaker := hintAt(seg.SpeakerHints, idx)

	if isDialogueLike(prevRole) != isDialogueLike(curRole) {
		return true
	}
	if prevSpeaker != "" && curSpeaker != "" && prevSpeaker != curSpeaker {
		return true
	}
	if isStrongBoundary(seg.SourceLines[idx-1]) && !isShortContextDependent(seg.SourceLines[idx], curRole) {
		return true
	}
	return false
}

func makeChunk(seg segment, start, end, pos, count int) chunk {
	lines := append([]string(nil), seg.SourceLines[start:end]...)
	lineIDs := append([]string(nil), seg.LineIDs[start:end]...)
	roles := sliceOrNil(seg.TextRoles, start, end)
	speakers := sliceOrNil(seg.SpeakerHints, start, end)
	return chunk{
		ChunkID:         buildChunkID(seg.SegmentID, pos, lineIDs),
		ParentSegmentID: seg.SegmentID,
		ChunkPos:        pos,
		ChunkCount:      count,
		SourceFile:      seg.SourceFile,
		SceneHint:       seg.SceneHint,
		BlockKind:       seg.BlockKind,
		ChoiceBlockID:   seg.ChoiceBlockID,
		SourceText:      strings.Join(lines, "\n"),
		LineIDs:         lineIDs,
		SourceLines:     lines,
		TextRoles:       roles,
		SpeakerHints:    speakers,
		MetaPathLabel:   seg.MetaPathLabel,
	}
}

func sliceOrNil(in []string, start, end int) []string {
	if len(in) < end {
		return nil
	}
	return append([]string(nil), in[start:end]...)
}

func roleAt(roles []string, idx int) string {
	if idx < 0 || idx >= len(roles) {
		return ""
	}
	return roles[idx]
}

func hintAt(hints []string, idx int) string {
	if idx < 0 || idx >= len(hints) {
		return ""
	}
	return hints[idx]
}

func isDialogueLike(role string) bool {
	switch role {
	case "dialogue", "reaction", "fragment", "choice":
		return true
	default:
		return false
	}
}

func isShortContextDependent(line, role string) bool {
	if role == "reaction" || role == "fragment" {
		return true
	}
	words := len(strings.Fields(line))
	return words > 0 && words <= 3
}

func isStrongBoundary(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasSuffix(line, ".") || strings.HasSuffix(line, "!") || strings.HasSuffix(line, "?")
}

func buildChunkID(segmentID string, pos int, lineIDs []string) string {
	sum := sha1.Sum([]byte(segmentID + "|" + strings.Join(lineIDs, "|")))
	return fmt.Sprintf("chunk-%02d-%s", pos, hex.EncodeToString(sum[:6]))
}

func appendUniqueRule(rules []string, extra ...string) []string {
	seen := make(map[string]bool, len(rules)+len(extra))
	out := make([]string, 0, len(rules)+len(extra))
	for _, rule := range rules {
		if strings.TrimSpace(rule) == "" || seen[rule] {
			continue
		}
		seen[rule] = true
		out = append(out, rule)
	}
	for _, rule := range extra {
		if strings.TrimSpace(rule) == "" || seen[rule] {
			continue
		}
		seen[rule] = true
		out = append(out, rule)
	}
	return out
}
