package v2pipeline

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
)

// csvBOM is the UTF-8 byte order mark used by Windows CSV files.
var csvBOM = []byte{0xEF, 0xBB, 0xBF}

// CSVTranslationReport tracks the results of CSV translation.
type CSVTranslationReport struct {
	FileName   string
	Total      int // data rows (excluding header)
	Translated int // successfully translated
	Skipped    int // empty ENGLISH
	Errors     int // translation failures
}

// ReadCSVFile reads a BOM-prefixed CSV file (ID,ENGLISH,KOREAN format).
func ReadCSVFile(path string) ([][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimPrefix(data, csvBOM)
	r := csv.NewReader(bytes.NewReader(data))
	r.LazyQuotes = true
	return r.ReadAll()
}

// WriteCSVFile writes rows as BOM-prefixed CSV.
func WriteCSVFile(path string, rows [][]string) error {
	var buf bytes.Buffer
	buf.Write(csvBOM)
	w := csv.NewWriter(&buf)
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return fmt.Errorf("csv write: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// TranslateCSVRows fills the KOREAN column (index 2) using translateFn.
// Per D-11: all rows are re-translated (existing KOREAN values overwritten).
// Header row (index 0) is preserved as-is.
func TranslateCSVRows(rows [][]string, translateFn func(english string) (string, error)) (*CSVTranslationReport, error) {
	report := &CSVTranslationReport{}
	if len(rows) <= 1 {
		return report, nil
	}
	report.Total = len(rows) - 1 // exclude header

	for i := 1; i < len(rows); i++ {
		// Ensure at least 3 columns
		for len(rows[i]) < 3 {
			rows[i] = append(rows[i], "")
		}

		english := rows[i][1] // ENGLISH column
		if english == "" {
			report.Skipped++
			continue
		}

		korean, err := translateFn(english)
		if err != nil {
			report.Errors++
			continue
		}
		rows[i][2] = korean
		report.Translated++
	}
	return report, nil
}
