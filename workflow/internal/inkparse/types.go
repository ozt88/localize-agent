package inkparse

// DialogueBlock represents one game-rendering unit of text.
// Consecutive "^text" entries within a gate/choice container are merged into one block.
type DialogueBlock struct {
	ID         string   `json:"id"`          // path-based: "KnotName/g-0/c-1/blk-0"
	Path       string   `json:"path"`        // tree path without blk suffix
	Text       string   `json:"text"`        // merged text content (^ prefix stripped)
	SourceHash string   `json:"source_hash"` // SHA-256 hex of Text
	SourceFile string   `json:"source_file"` // TextAsset filename (e.g., "AR_CoastMap")
	Knot       string   `json:"knot"`        // knot name
	Gate       string   `json:"gate"`        // gate ID (e.g., "g-0") or empty
	Choice     string   `json:"choice"`      // choice ID (e.g., "c-1") or empty
	Speaker    string   `json:"speaker"`     // from # speaker tag, empty if none
	Tags       []string `json:"tags"`        // DC_check, OBJ, XPGain, etc.
	BlockIndex int      `json:"block_index"` // sequential index within container
}

// ParseResult holds all blocks extracted from one TextAsset file.
type ParseResult struct {
	SourceFile       string          `json:"source_file"`
	Blocks           []DialogueBlock `json:"blocks"`
	TotalTextEntries int             `json:"total_text_entries"`
}
