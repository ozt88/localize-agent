package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"time"

	"localize-agent/workflow/internal/inkparse"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// dcfcPrefixRe matches DC/FC stat-check prefixes — same pattern as parser.go.
var dcfcPrefixRe = regexp.MustCompile(`^[A-Z]{2}\d+\s+\w+-`)

// parseOutput matches go-ink-parse JSON output structure.
type parseOutput struct {
	TotalFiles       int                     `json:"total_files"`
	TotalBlocks      int                     `json:"total_blocks"`
	TotalTextEntries int                     `json:"total_text_entries"`
	Results          []inkparse.ParseResult  `json:"results"`
}

// dbRow holds a row from pipeline_items_v2.
type dbRow struct {
	ID          string
	SourceRaw   string
	SourceHash  string
	KORaw       sql.NullString
	KOFormatted sql.NullString
}

// changedRow holds the old and new values for a changed row.
type changedRow struct {
	ID            string
	OldSourceRaw  string
	OldSourceHash string
	OldKORaw      sql.NullString
	OldKOFormatted sql.NullString
	NewSourceRaw  string
	NewSourceHash string
	NewKORaw      sql.NullString
	NewKOFormatted sql.NullString
}

func main() {
	os.Exit(run())
}

func run() int {
	var (
		parseResultPath string
		dsn             string
		dryRun          bool
	)

	flag.StringVar(&parseResultPath, "parse-result", "", "path to parse_result.json from go-ink-parse")
	flag.StringVar(&dsn, "dsn", "postgres://postgres@127.0.0.1:5433/localize_agent?sslmode=disable", "PostgreSQL DSN")
	flag.BoolVar(&dryRun, "dry-run", true, "if true, only report changes without writing (default: true)")
	flag.Parse()

	if parseResultPath == "" {
		fmt.Fprintf(os.Stderr, "error: --parse-result is required\n")
		return 1
	}

	// 1. Load parse result
	data, err := os.ReadFile(parseResultPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading parse result: %v\n", err)
		return 1
	}
	var parsed parseOutput
	if err := json.Unmarshal(data, &parsed); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing JSON: %v\n", err)
		return 1
	}

	// Flatten blocks into map by ID
	blockMap := make(map[string]inkparse.DialogueBlock)
	for _, r := range parsed.Results {
		for _, b := range r.Blocks {
			blockMap[b.ID] = b
		}
	}
	fmt.Printf("Loaded %d blocks from parse result\n", len(blockMap))

	// 2. Connect to PostgreSQL
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error connecting to DB: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "error pinging DB: %v\n", err)
		return 1
	}
	fmt.Println("Connected to PostgreSQL")

	// 3. Query all IDs from parse result
	ids := make([]string, 0, len(blockMap))
	for id := range blockMap {
		ids = append(ids, id)
	}

	// Query in batches of 1000 to avoid parameter limit
	var dbRows []dbRow
	batchSize := 1000
	for i := 0; i < len(ids); i += batchSize {
		end := i + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]

		// Build placeholder string
		placeholders := make([]string, len(batch))
		args := make([]any, len(batch))
		for j, id := range batch {
			placeholders[j] = fmt.Sprintf("$%d", j+1)
			args[j] = id
		}

		query := fmt.Sprintf(
			"SELECT id, source_raw, source_hash, ko_raw, ko_formatted FROM pipeline_items_v2 WHERE id IN (%s)",
			joinStrings(placeholders, ","),
		)

		rows, err := db.Query(query, args...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error querying DB batch %d: %v\n", i/batchSize, err)
			return 1
		}
		for rows.Next() {
			var r dbRow
			if err := rows.Scan(&r.ID, &r.SourceRaw, &r.SourceHash, &r.KORaw, &r.KOFormatted); err != nil {
				rows.Close()
				fmt.Fprintf(os.Stderr, "error scanning row: %v\n", err)
				return 1
			}
			dbRows = append(dbRows, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "error iterating rows: %v\n", err)
			return 1
		}
	}
	fmt.Printf("Found %d matching rows in DB\n", len(dbRows))

	// 4. Identify changed rows
	var changed []changedRow
	for _, row := range dbRows {
		block, ok := blockMap[row.ID]
		if !ok {
			continue
		}
		if row.SourceRaw == block.Text {
			continue // no change
		}
		newHash := sourceHash(block.Text)
		cr := changedRow{
			ID:            row.ID,
			OldSourceRaw:  row.SourceRaw,
			OldSourceHash: row.SourceHash,
			OldKORaw:      row.KORaw,
			OldKOFormatted: row.KOFormatted,
			NewSourceRaw:  block.Text,
			NewSourceHash: newHash,
		}
		// Strip DC/FC prefix from ko_raw/ko_formatted
		if row.KORaw.Valid {
			stripped := dcfcPrefixRe.ReplaceAllString(row.KORaw.String, "")
			cr.NewKORaw = sql.NullString{String: stripped, Valid: true}
		}
		if row.KOFormatted.Valid {
			stripped := dcfcPrefixRe.ReplaceAllString(row.KOFormatted.String, "")
			cr.NewKOFormatted = sql.NullString{String: stripped, Valid: true}
		}
		changed = append(changed, cr)
	}
	fmt.Printf("Found %d changed rows (DC/FC prefix stripped)\n", len(changed))

	// 5. UNIQUE collision check — skip rows that would collide
	// Also check collisions within the changed set itself (two rows mapping to same new hash).
	collisions := 0
	var safeChanged []changedRow
	newHashSeen := make(map[string]string) // newSourceHash -> first ID claiming it
	for _, cr := range changed {
		// Check if another changed row already claims this hash
		if otherID, ok := newHashSeen[cr.NewSourceHash]; ok {
			collisions++
			fmt.Printf("  SKIP (internal collision): %s -> same hash as %s\n", cr.ID, otherID)
			continue
		}
		// Check if an existing DB row (not being changed) already has this hash
		var existingID string
		err := db.QueryRow(
			"SELECT id FROM pipeline_items_v2 WHERE source_hash = $1 AND id != $2",
			cr.NewSourceHash, cr.ID,
		).Scan(&existingID)
		if err == nil {
			collisions++
			fmt.Printf("  SKIP (DB collision): %s -> hash collision with %s\n", cr.ID, existingID)
			continue
		}
		newHashSeen[cr.NewSourceHash] = cr.ID
		safeChanged = append(safeChanged, cr)
	}
	fmt.Printf("\nCollision summary: %d skipped, %d safe to update\n", collisions, len(safeChanged))

	// 6. Count ko_raw/ko_formatted strips
	koRawStripped := 0
	koFormattedStripped := 0
	for _, cr := range safeChanged {
		if cr.OldKORaw.Valid && cr.NewKORaw.Valid && cr.OldKORaw.String != cr.NewKORaw.String {
			koRawStripped++
		}
		if cr.OldKOFormatted.Valid && cr.NewKOFormatted.Valid && cr.OldKOFormatted.String != cr.NewKOFormatted.String {
			koFormattedStripped++
		}
	}

	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Total detected: %d\n", len(changed))
	fmt.Printf("  Collisions skipped: %d\n", collisions)
	fmt.Printf("  Safe to update: %d\n", len(safeChanged))
	fmt.Printf("  ko_raw stripped: %d\n", koRawStripped)
	fmt.Printf("  ko_formatted stripped: %d\n", koFormattedStripped)

	if dryRun {
		fmt.Println("\n[DRY RUN] No changes written. Use --dry-run=false to execute.")
		return 0
	}

	// 7. Execute in transaction
	tx, err := db.Begin()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error starting transaction: %v\n", err)
		return 1
	}

	// Create snapshot table
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS source_cleanup_snapshot (
			id TEXT PRIMARY KEY,
			old_source_raw TEXT NOT NULL,
			old_source_hash TEXT NOT NULL,
			old_ko_raw TEXT,
			old_ko_formatted TEXT,
			created_at TEXT NOT NULL
		)
	`)
	if err != nil {
		tx.Rollback()
		fmt.Fprintf(os.Stderr, "error creating snapshot table: %v\n", err)
		return 1
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Insert snapshot rows and update pipeline_items_v2
	snapshotInserted := 0
	updated := 0
	for _, cr := range safeChanged {
		// Insert snapshot
		_, err := tx.Exec(
			`INSERT INTO source_cleanup_snapshot (id, old_source_raw, old_source_hash, old_ko_raw, old_ko_formatted, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (id) DO NOTHING`,
			cr.ID, cr.OldSourceRaw, cr.OldSourceHash, cr.OldKORaw, cr.OldKOFormatted, now,
		)
		if err != nil {
			tx.Rollback()
			fmt.Fprintf(os.Stderr, "error inserting snapshot for %s: %v\n", cr.ID, err)
			return 1
		}
		snapshotInserted++

		// Update pipeline_items_v2
		_, err = tx.Exec(
			`UPDATE pipeline_items_v2 SET source_raw = $1, source_hash = $2, ko_raw = $3, ko_formatted = $4 WHERE id = $5`,
			cr.NewSourceRaw, cr.NewSourceHash, cr.NewKORaw, cr.NewKOFormatted, cr.ID,
		)
		if err != nil {
			tx.Rollback()
			fmt.Fprintf(os.Stderr, "error updating %s: %v\n", cr.ID, err)
			return 1
		}
		updated++
	}

	if err := tx.Commit(); err != nil {
		fmt.Fprintf(os.Stderr, "error committing transaction: %v\n", err)
		return 1
	}

	fmt.Printf("\nExecution complete:\n")
	fmt.Printf("  Snapshot rows inserted: %d\n", snapshotInserted)
	fmt.Printf("  Pipeline rows updated: %d\n", updated)
	return 0
}

func sourceHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
