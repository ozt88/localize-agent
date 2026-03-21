package main

import (
	"fmt"
	"regexp"
	"strings"
)

func containsGlossaryTerm(en, term string) bool {
	en = strings.TrimSpace(en)
	term = strings.TrimSpace(term)
	if en == "" || term == "" {
		return false
	}
	if strings.EqualFold(en, term) {
		return true
	}
	suffixPattern := `(?:s|'s|ed|ing|ism|ist|ic|ally|ians?)?`
	pattern := `(?i)(^|[^A-Za-z0-9])` + regexp.QuoteMeta(term) + suffixPattern + `([^A-Za-z0-9]|$)`
	re := regexp.MustCompile(pattern)
	return re.FindStringIndex(en) != nil
}

func main() {
	tests := []struct {
		en, term string
		want     bool
	}{
		{"They're Azgalists?", "Azgalist", true},
		{"Azgalism failed me.", "Azgalism", true},
		{"classic Azgalian joke", "Azgalian", true},
		{"the Freestriders are many things", "Freestrider", true},
		{"Freestriderism is new", "Freestrider", true},
		{"esoterically picking up vibes", "Esoteric", true},
		{"esoteric pockets", "Esoteric", true},
		{"Gorm lifts his head", "Gorm", true},
		{"body burned down to its frays", "Fray", true},
		{"A BURNING FRAY", "Fray", true},
		{"frayed fellow", "frayed", true},
		{"Azgal Youth activists", "Azgal", true},
		{"Azgal-dwarves got an expired PPG", "Azgal", true},
	}
	pass, fail := 0, 0
	for _, tt := range tests {
		got := containsGlossaryTerm(tt.en, tt.term)
		status := "OK"
		if got != tt.want {
			status = "FAIL"
			fail++
		} else {
			pass++
		}
		short := tt.en
		if len(short) > 50 {
			short = short[:50]
		}
		fmt.Printf("[%s] %q contains %q = %v (want %v)\n", status, short, tt.term, got, tt.want)
	}
	fmt.Printf("\nPass: %d, Fail: %d\n", pass, fail)
}
