package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"localize-agent/workflow/pkg/shared"
)

type item struct {
	ID    string
	Score int
	Len   int
	WC    int
}

var (
	tagRE         = regexp.MustCompile(`\{[^{}]*\}`)
	placeholderRE = regexp.MustCompile(`\[T[0-9]+\]`)
	spaceRE       = regexp.MustCompile(`\s+`)
	punctRE       = regexp.MustCompile(`[.!?;:]`)
)

func loadStrings(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	sobj, ok := root["strings"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid strings object: %s", path)
	}
	out := make(map[string]string, len(sobj))
	for id, v := range sobj {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		txt, _ := m["Text"].(string)
		out[id] = txt
	}
	return out, nil
}

func main() {
	var sourcePath, currentPath, outPath string
	var n int
	var mode string
	var shortN int
	flag.StringVar(&sourcePath, "source", "enGB_original.json", "")
	flag.StringVar(&currentPath, "current", "enGB_new.json", "")
	flag.StringVar(&outPath, "out", "workflow/output/samples/ids_quality_50.txt", "")
	flag.IntVar(&n, "n", 50, "")
	flag.StringVar(&mode, "mode", "long", "sampling mode: long or mixed")
	flag.IntVar(&shortN, "short-n", 50, "short item count for mixed mode")
	flag.Parse()

	src, err := loadStrings(sourcePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cur, err := loadStrings(currentPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cands := make([]item, 0, len(src))
	for id, txt := range src {
		if _, ok := cur[id]; !ok {
			continue
		}
		plain := tagRE.ReplaceAllString(txt, "")
		plain = placeholderRE.ReplaceAllString(plain, "")
		plain = strings.TrimSpace(plain)
		if plain == "" {
			continue
		}
		wc := len(strings.Fields(spaceRE.ReplaceAllString(plain, " ")))
		l := len([]rune(plain))
		tagc := len(tagRE.FindAllString(txt, -1))
		newline := 0
		if strings.Contains(txt, "\n") {
			newline = 1
		}
		punct := len(punctRE.FindAllString(plain, -1))
		score := l + (wc * 3) + (tagc * 15) + (newline * 40) + (punct * 4)
		cands = append(cands, item{ID: id, Score: score, Len: l, WC: wc})
	}

	var picked []item
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "mixed":
		longs := make([]item, 0, len(cands))
		shorts := make([]item, 0, len(cands))
		for _, c := range cands {
			if c.Len >= 220 && c.Len <= 900 && c.WC >= 28 && c.Score >= 320 {
				longs = append(longs, c)
			}
			if c.Len >= 25 && c.Len <= 120 && c.WC >= 6 {
				shorts = append(shorts, c)
			}
		}
		sort.Slice(longs, func(i, j int) bool {
			if longs[i].Score == longs[j].Score {
				return longs[i].ID < longs[j].ID
			}
			return longs[i].Score > longs[j].Score
		})
		sort.Slice(shorts, func(i, j int) bool {
			if shorts[i].Len == shorts[j].Len {
				return shorts[i].ID < shorts[j].ID
			}
			return shorts[i].Len > shorts[j].Len
		})
		longN := n - shortN
		if longN < 0 {
			longN = 0
		}
		if len(shorts) > shortN {
			shorts = shorts[:shortN]
		}
		if len(longs) > longN {
			longs = longs[:longN]
		}
		picked = append(picked, longs...)
		picked = append(picked, shorts...)
	default:
		longs := make([]item, 0, len(cands))
		for _, c := range cands {
			if c.Len >= 220 && c.Len <= 900 && c.WC >= 28 && c.Score >= 320 {
				longs = append(longs, c)
			}
		}
		sort.Slice(longs, func(i, j int) bool {
			if longs[i].Score == longs[j].Score {
				return longs[i].ID < longs[j].ID
			}
			return longs[i].Score > longs[j].Score
		})
		if len(longs) > n {
			longs = longs[:n]
		}
		picked = longs
	}

	lines := make([]string, 0, len(picked))
	for _, c := range picked {
		lines = append(lines, c.ID)
	}
	if err := os.MkdirAll("workflow/output/samples", 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := shared.AtomicWriteFile(outPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d ids -> %s\n", len(lines), outPath)
}
