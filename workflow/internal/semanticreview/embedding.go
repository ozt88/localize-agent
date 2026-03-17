package semanticreview

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type embeddingPair struct {
	ID string `json:"id"`
	A  string `json:"a"`
	B  string `json:"b"`
}

type embeddingResult struct {
	ID         string  `json:"id"`
	Similarity float64 `json:"similarity"`
}

func computeSemanticSimilarities(workDir string, pairs []embeddingPair) (map[string]float64, error) {
	inputPath := filepath.Join(workDir, "semantic_review_embed_input.json")
	outputPath := filepath.Join(workDir, "semantic_review_embed_output.json")
	scriptPath := filepath.Join("workflow", "internal", "semanticreview", "scripts", "embed_compare.py")

	raw, err := json.Marshal(pairs)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(inputPath, raw, 0644); err != nil {
		return nil, err
	}
	cmd := exec.Command("python", scriptPath, "--input", inputPath, "--output", outputPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("embedding helper failed: %v: %s", err, string(out))
	}
	defer os.Remove(inputPath)

	data, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, err
	}
	defer os.Remove(outputPath)

	var rows []embeddingResult
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	out := map[string]float64{}
	for _, row := range rows {
		out[row.ID] = row.Similarity
	}
	return out, nil
}
