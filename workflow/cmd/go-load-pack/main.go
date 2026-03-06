package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/internal/platform"
)

func main() {
	var packIn string
	var dbPath string
	var runName string

	flag.StringVar(&packIn, "pack-in", "", "input pack JSON path ({items:[...]})")
	flag.StringVar(&dbPath, "db", "workflow/output/evaluation_unified.db", "unified evaluation DB path")
	flag.StringVar(&runName, "run-name", "default", "logical run name")
	flag.Parse()

	if packIn == "" {
		fmt.Fprintln(os.Stderr, "--pack-in is required")
		os.Exit(2)
	}

	raw, err := os.ReadFile(packIn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	var pack struct {
		Items []contracts.EvalPackItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &pack); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	store, err := platform.NewSQLiteEvalStore(dbPath, runName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer store.Close()

	n, err := store.LoadPack(pack.Items)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("loaded run=%s db=%s inserted=%d total_input=%d\n", runName, dbPath, n, len(pack.Items))
}
