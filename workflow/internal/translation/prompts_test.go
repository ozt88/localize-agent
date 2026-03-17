package translation

import (
	"strings"
	"testing"
)

func TestNewTranslateSkill_MergesDefaultAndProjectRules(t *testing.T) {
	skill := newTranslateSkill("ctx", "PROJECT_RULE")
	warmup := skill.warmup()
	if !containsAll(warmup,
		"Reply to this warmup with exactly: OK",
		"Translate only the `en` field.",
		"`context_en` or `contexts` are reference-only scene context.",
		"`items[*].ctx` and `items[*].line` point to the matching reference line in `contexts`.",
		"Choice lines represent player actions, not narration.",
		"If `stat_check` is present, preserve it as a bracketed gameplay prefix in Korean.",
		"`[PLAYER OPTION]` inside `context_en` or `contexts` is a reference-only anchor.",
		"Do not repair, complete, or reinterpret missing source text.",
		"Do not invent missing quotation closure or missing continuation.",
		"Output must be valid JSON only.",
		"Return only the contract defined by the project-local translator system prompt.",
		"PROJECT_RULE",
	) {
		t.Fatalf("warmup did not include merged rules:\n%s", warmup)
	}
}

func TestExtractObjects_ArrayPayload(t *testing.T) {
	raw := `[{"id":"a","proposed_ko":"alpha"},{"id":"b","proposed_ko":"beta"}]`
	got := extractObjects(raw)
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("ids=%q,%q", got[0].ID, got[1].ID)
	}
}

func TestBuildSinglePrompt_MinimalPayloadOnly(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:          "id-choice",
		BodyEN:      "Tell [[E0]]him[[/E0]] to leave.",
		ContextEN:   "Give back the papers.\nHis flesh turns to [[E1]]fine dust[[/E1]].",
		TextRole:    "dialogue",
		SpeakerHint: "Snell",
	}, `{"type":"object"}`, false)

	if !containsAll(prompt,
		"without repairing or completing broken source fragments",
		`"id":"id-choice"`,
		`"en":"Tell [[E0]]him[[/E0]] to leave."`,
		`"context_en":"Give back the papers.\nHis flesh turns to [[E1]]fine dust[[/E1]]."`,
		`"text_role":"dialogue"`,
		`"speaker_hint":"Snell"`,
	) {
		t.Fatalf("prompt missing minimal fields:\n%s", prompt)
	}
	for _, forbidden := range []string{
		`"choice_prefix"`,
		`"chunk_id"`,
		`"parent_segment_id"`,
		`"line_is_imperative"`,
		`"line_is_short_context_dependent"`,
		`"meta_path_label"`,
		`"segment_id"`,
		`"segment_pos"`,
		`"choice_block_id"`,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt still contains %s:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildSinglePrompt_IncludesGlossaryMappings(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:     "id-glossary",
		BodyEN: "Cure Wounds",
		Glossary: []glossaryEntry{
			{Source: "Cure Wounds", Target: "상처 치유", Mode: "translate"},
			{Source: "SPELL", Target: "SPELL", Mode: "preserve"},
		},
	}, `{"type":"object"}`, false)

	if !containsAll(prompt,
		`"glossary":[{"source":"Cure Wounds","target":"상처 치유","mode":"translate"},{"source":"SPELL","target":"SPELL","mode":"preserve"}]`,
		"glossary is present",
		"mandatory terminology",
	) {
		t.Fatalf("prompt missing glossary mappings:\n%s", prompt)
	}
}

func TestBuildBatchPrompt_IncludesGlossaryMappings(t *testing.T) {
	prompt := buildBatchPrompt([]translationTask{{
		ID:     "id-glossary-batch",
		BodyEN: "Collected Spells",
		Glossary: []glossaryEntry{
			{Source: "Collected Spells", Target: "수집한 주문", Mode: "translate"},
		},
	}}, `{"type":"object"}`, true)

	if !containsAll(prompt,
		`"glossary":[{"source":"Collected Spells","target":"수집한 주문","mode":"translate"}]`,
		"glossary is present",
		"mandatory terminology",
	) {
		t.Fatalf("batch prompt missing glossary mappings:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_ChoiceMarkerIsEmbeddedInRenderedPayload(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:        "id-choice",
		BodyEN:    "Tell him to leave.",
		ContextEN: "Guard: Stop.\nTell him to leave.\nAsk about the papers.",
		TextRole:  "choice",
	}, `{"type":"object"}`, false)

	if !containsAll(prompt,
		`"en":"Tell him to leave."`,
		`"context_en":"Guard: Stop.\n[PLAYER OPTION] Tell him to leave.\nAsk about the papers."`,
		`"focused_context_en":"Guard: Stop.\n[[BODY_EN]] [PLAYER OPTION] Tell him to leave. [[/BODY_EN]]\nAsk about the papers."`,
	) {
		t.Fatalf("choice marker missing in prompt:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_IncludesStatCheckField(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:        "id-choice",
		BodyEN:    "Force him to return the papers.",
		TextRole:  "choice",
		StatCheck: "STR 14",
	}, `{"type":"object"}`, false)

	if !containsAll(prompt,
		`"en":"Force him to return the papers."`,
		`"stat_check":"STR 14"`,
	) {
		t.Fatalf("stat check missing in prompt:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_StatCheckActionAlsoUsesChoiceMarker(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:         "id-stat-action",
		BodyEN:     "Crack the safe.",
		TextRole:   "dialogue",
		ChoiceMode: "stat_check_action",
		StatCheck:  "DEX 25",
	}, `{"type":"object"}`, false)

	if !containsAll(prompt,
		`"en":"Crack the safe."`,
		`"stat_check":"DEX 25"`,
	) {
		t.Fatalf("stat-check action marker missing in prompt:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_RetryContextIncludesNeighborKOAndReason(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:          "id-retry",
		BodyEN:      "Tell him to leave.",
		ContextEN:   "Give back the papers.\nHis flesh turns to fine dust.",
		CurrentKO:   "그에게 떠나라고 말해.",
		PrevEN:      "Give back the papers.",
		NextEN:      "His flesh turns to fine dust.",
		PrevKO:      "서류를 돌려줘.",
		NextKO:      "그의 살은 고운 먼지로 변한다.",
		TextRole:    "dialogue",
		SpeakerHint: "Snell",
		RetryReason: "context_fit: tone and flow mismatch with surrounding lines",
	}, `{"type":"object"}`, false)

	if !containsAll(prompt,
		`"current_ko":"그에게 떠나라고 말해."`,
		`"prev_en":"Give back the papers."`,
		`"next_en":"His flesh turns to fine dust."`,
		`"prev_ko":"서류를 돌려줘."`,
		`"next_ko":"그의 살은 고운 먼지로 변한다."`,
		`"retry_reason":"context_fit: tone and flow mismatch with surrounding lines"`,
	) {
		t.Fatalf("prompt missing retry context fields:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_IncludesContinuityGuidance(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:        "id-retry-guidance",
		BodyEN:    "Tell him to leave.",
		CurrentKO: "그에게 떠나라고 말해.",
		PrevKO:    "서류를 돌려줘.",
		NextKO:    "그의 살은 고운 먼지로 변한다.",
	}, `{"type":"object"}`, true)

	if strings.Contains(prompt, "continuity references") {
		t.Fatalf("prompt should keep continuity guidance in warmup, not item prompt:\n%s", prompt)
	}
}

func TestBuildBatchPrompt_IncludesContinuityGuidance(t *testing.T) {
	prompt := buildBatchPrompt([]translationTask{{
		ID:        "id-1",
		BodyEN:    "Tell him to leave.",
		CurrentKO: "그에게 떠나라고 말해.",
		PrevKO:    "서류를 돌려줘.",
		NextKO:    "그의 살은 고운 먼지로 변한다.",
	}}, `{"type":"object"}`, true)

	if strings.Contains(prompt, "continuity references") || strings.Contains(prompt, "Never translate the first line of context_en by default") || strings.Contains(prompt, "never reuse another item's translation") {
		t.Fatalf("prompt should keep repeated guidance in warmup, not batch prompt:\n%s", prompt)
	}
}

func TestBuildBatchPrompt_UsesContextPoolPayload(t *testing.T) {
	prompt := buildBatchPrompt([]translationTask{
		{
			ID:           "id-1",
			BodyEN:       "Stop.",
			ContextEN:    "Tinn: Stop.\nPlayer: Why?",
			ContextLines: []string{"Tinn: Stop.", "Player: Why?"},
			ContextLine:  0,
			TextRole:     "dialogue",
			SpeakerHint:  "Tinn",
		},
		{
			ID:           "id-2",
			BodyEN:       "Why?",
			ContextEN:    "Tinn: Stop.\nPlayer: Why?",
			ContextLines: []string{"Tinn: Stop.", "Player: Why?"},
			ContextLine:  1,
			TextRole:     "dialogue",
			SpeakerHint:  "Player",
		},
	}, `{"type":"object"}`, true)

	if !containsAll(prompt,
		`"contexts":[["Tinn: Stop.","Player: Why?"]]`,
		`"items":[{"id":"id-1","ctx":0,"line":0,"en":"Stop."`,
		`"id":"id-2","ctx":0,"line":1,"en":"Why?"`,
	) {
		t.Fatalf("batch prompt should use context pool payload:\n%s", prompt)
	}
	if strings.Contains(prompt, `"focused_context_en"`) || strings.Contains(prompt, `"prev_en"`) || strings.Contains(prompt, `"next_en"`) {
		t.Fatalf("batch prompt should not duplicate context fields:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_OllamaPlainOutput(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:          "id-1",
		BodyEN:      "It is piss pot. Enjoy.",
		ContextEN:   "But can know for sure? No.\nIt is piss pot. Enjoy.",
		TextRole:    "dialogue",
		SpeakerHint: "Ost",
	}, `{"type":"object"}`, true)

	if !containsAll(prompt,
		"Return one valid JSON array with exactly 1 Korean string and nothing else.",
		"without repairing or completing broken source fragments",
		"Input JSON:",
		`"en":"It is piss pot. Enjoy."`,
		`"context_en":"But can know for sure? No.\nIt is piss pot. Enjoy."`,
		`"focused_context_en":"But can know for sure? No.\n[[BODY_EN]] It is piss pot. Enjoy. [[/BODY_EN]]"`,
	) {
		t.Fatalf("prompt missing plain-output guidance:\n%s", prompt)
	}
	for _, forbidden := range []string{
		"Translate only en.",
		"do not translate the first line of context_en unless it is also en",
		"continuity references",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt should move repeated guidance to warmup, found %q:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildSinglePrompt_IncludesBrokenFragmentHandlingGuidance(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:       "id-fragment",
		BodyEN:   `He gives a short laugh. "I'm in a similar seat myself.`,
		TextRole: "dialogue",
	}, `{"type":"object"}`, true)

	if !containsAll(prompt,
		"Do not add prose before or after the JSON array.",
		"If the English is truncated, open-ended, or has an unbalanced quote, translate only the visible fragment naturally.",
		"Preserve gameplay prefixes, action markers, narration cues, and mixed action-plus-dialogue structure as meaningful source content.",
		`"fragment_pattern":"narration_open_quote"`,
		"If [PLAYER OPTION] appears inside context, it is a reference-only anchor and must not be copied into output.",
	) {
		t.Fatalf("prompt missing fragment-handling guidance:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_IncludesActionOpenQuoteHints(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:       "id-action-quote",
		BodyEN:   `(Sigh.) "Fine. What do you want to know?`,
		TextRole: "choice",
	}, `{"type":"object"}`, true)

	if !containsAll(prompt,
		`"fragment_pattern":"action_open_quote"`,
		`"action_cue_en":"(Sigh.)"`,
		`"spoken_fragment_en":"Fine. What do you want to know?"`,
		"reference-only structure hints",
	) {
		t.Fatalf("prompt missing action-open-quote hints:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_IncludesDCOpenQuoteHints(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:         "id-dc-quote",
		BodyEN:     `DC10 dex-"Right, but do you hustle?`,
		TextRole:   "narration",
		ChoiceMode: "stat_check_action",
		StatCheck:  "DEX 10",
	}, `{"type":"object"}`, true)

	if !containsAll(prompt,
		`"fragment_pattern":"dc_open_quote"`,
		`"spoken_fragment_en":"Right, but do you hustle?"`,
		`"stat_check":"DEX 10"`,
		"reference-only structure hints",
	) {
		t.Fatalf("prompt missing dc-open-quote hints:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_IncludesNarrationOpenQuoteHints(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:       "id-narration-quote",
		BodyEN:   `She laughs. "I like you, Cleric. But no.`,
		TextRole: "dialogue",
	}, `{"type":"object"}`, true)

	if !containsAll(prompt,
		`"fragment_pattern":"narration_open_quote"`,
		`"action_cue_en":"She laughs."`,
		`"spoken_fragment_en":"I like you, Cleric. But no."`,
		"reference-only structure hints",
	) {
		t.Fatalf("prompt missing narration-open-quote hints:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_IncludesBareOpenQuoteHints(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:       "id-open-quote",
		BodyEN:   `"I can be trusted. I'm not like those other people.`,
		TextRole: "dialogue",
	}, `{"type":"object"}`, true)

	if !containsAll(prompt,
		`"fragment_pattern":"open_quote"`,
		`"spoken_fragment_en":"I can be trusted. I'm not like those other people."`,
		"reference-only structure hints",
	) {
		t.Fatalf("prompt missing bare open-quote hints:\n%s", prompt)
	}
	if strings.Contains(prompt, `"action_cue_en"`) {
		t.Fatalf("bare open quote should not include action cue:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_IncludesSpeechThenNarrationQuoteHints(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:       "id-speech-then-narration",
		BodyEN:   `Here we go," the goblin mutters as he brings out his sling in one swift motion.`,
		TextRole: "dialogue",
	}, `{"type":"object"}`, true)

	if !containsAll(prompt,
		`"fragment_pattern":"speech_then_narration_quote"`,
		`"action_cue_en":"the goblin mutters as he brings out his sling in one swift motion."`,
		`"spoken_fragment_en":"Here we go,"`,
		"reference-only structure hints",
	) {
		t.Fatalf("prompt missing speech-then-narration hints:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_IncludesSpeechThenActionQuoteHints(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:       "id-speech-then-action",
		BodyEN:   `Uh, uh...!" (Start shaking.)`,
		TextRole: "fragment",
	}, `{"type":"object"}`, true)

	if !containsAll(prompt,
		`"fragment_pattern":"speech_then_action_quote"`,
		`"action_cue_en":"(Start shaking.)"`,
		`"spoken_fragment_en":"Uh, uh...!"`,
		"reference-only structure hints",
	) {
		t.Fatalf("prompt missing speech-then-action hints:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_IncludesDefinitionDashHintsForGlossary(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:       "id-definition",
		BodyEN:   "Millennium Empire - Lexandro's dream of an empire to last throughout the Eras.",
		TextRole: "glossary",
	}, `{"type":"object"}`, true)

	if !containsAll(prompt,
		`"structure_pattern":"definition_dash"`,
		`"lead_term_en":"Millennium Empire"`,
		`"definition_body_en":"Lexandro's dream of an empire to last throughout the Eras."`,
	) {
		t.Fatalf("prompt missing definition-dash hints:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_OmitsDefinitionDashHintsForOrdinaryDashDialogue(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:       "id-ordinary-dash",
		BodyEN:   "To do so, either double click - or hold shift/the left trigger.",
		TextRole: "dialogue",
	}, `{"type":"object"}`, true)

	for _, forbidden := range []string{
		`"structure_pattern"`,
		`"lead_term_en"`,
		`"definition_body_en"`,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("ordinary dash dialogue should not include %s:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildSinglePrompt_IncludesExpositoryEntryHintsForLongGlossary(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:       "id-expository",
		BodyEN:   "Varjomieli. A type of lichdom created through primal stasis, usually by powerful druids or elven wizards. Unlike ordinary liches, they do not inspire as much dread nor hold as much power, due to their balance with the natural world.",
		TextRole: "glossary",
	}, `{"type":"object"}`, true)

	if !containsAll(prompt,
		`"structure_pattern":"expository_entry"`,
		`"definition_body_en":"Varjomieli. A type of lichdom created through primal stasis, usually by powerful druids or elven wizards. Unlike ordinary liches, they do not inspire as much dread nor hold as much power, due to their balance with the natural world."`,
	) {
		t.Fatalf("prompt missing expository-entry hints:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_OmitsExpositoryEntryHintsForOrdinaryLongDialogue(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:       "id-long-dialogue",
		BodyEN:   "If you ever feel discontent about his behavior, remember: this is a person. He is not a representative of his entire people. Just as there are humans who are rude and boisterous and egotistical, there are owlfolk of the same cloth.",
		TextRole: "dialogue",
	}, `{"type":"object"}`, true)

	for _, forbidden := range []string{
		`"structure_pattern":"expository_entry"`,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("ordinary long dialogue should not include %s:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildSinglePrompt_IncludesLongDiscourseHintsForLongDialogue(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:       "id-long-discourse",
		BodyEN:   "If you're lucky, you'll get a dangerous gig in the tunnels. If not, the only way for you to get enough money for food, beyond the basic handouts, is joining a syndicate. They recruit young, to keep you in the juvenile-stockade if you get caught. Starting as an errand-boy. Ending up as... whatever they need for their enterprises.",
		TextRole: "dialogue",
	}, `{"type":"object"}`, true)

	if !containsAll(prompt,
		`"structure_pattern":"long_discourse"`,
		`"definition_body_en":"If you're lucky, you'll get a dangerous gig in the tunnels.`,
		"render the whole line as fluent Korean long-form dialogue or narration",
	) {
		t.Fatalf("prompt missing long_discourse hints:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_OmitsFragmentHintsForOrdinaryDialogue(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:       "id-ordinary",
		BodyEN:   "I agree. We should leave now.",
		TextRole: "dialogue",
	}, `{"type":"object"}`, true)

	for _, forbidden := range []string{
		`"fragment_pattern"`,
		`"action_cue_en"`,
		`"spoken_fragment_en"`,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("ordinary dialogue should not include %s:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildSinglePrompt_OmitsFragmentHintsForBalancedQuoteDialogue(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:       "id-balanced",
		BodyEN:   `He says, "We should leave now."`,
		TextRole: "dialogue",
	}, `{"type":"object"}`, true)

	for _, forbidden := range []string{
		`"fragment_pattern"`,
		`"action_cue_en"`,
		`"spoken_fragment_en"`,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("balanced quote dialogue should not include %s:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildBatchPrompt_IncludesBrokenFragmentHandlingGuidance(t *testing.T) {
	prompt := buildBatchPrompt([]translationTask{{
		ID:       "id-fragment",
		BodyEN:   `(Turn to Snell.) "You know this sphinx?`,
		TextRole: "choice",
	}}, `{"type":"object"}`, true)

	if !containsAll(prompt,
		"Translate each items[*].en into Korean without repairing or completing broken source fragments.",
		"If the English is truncated, open-ended, or has an unbalanced quote, translate only the visible fragment naturally.",
		"If [PLAYER OPTION] appears inside context, it is a reference-only anchor and must not be copied into output.",
		"Do not add prose before or after the JSON array.",
	) {
		t.Fatalf("batch prompt missing fragment-handling guidance:\n%s", prompt)
	}
}

func TestBuildFocusedContextEN_MarksExactBodyLine(t *testing.T) {
	got := buildFocusedContextEN("A\nB\nC", "B")
	want := "A\n[[BODY_EN]] B [[/BODY_EN]]\nC"
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

func TestBuildSinglePrompt_IncludesOnlyResourceLocatorFields(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:          "id-resource",
		BodyEN:      "New Game",
		ContextEN:   "Main Menu",
		TextRole:    "ui",
		SourceType:  "resource",
		SourceFile:  "UIElements.bytes",
		ResourceKey: "UI_1",
		MetaPath:    "Assets/Resources/localization/UIElements.bytes:UI_1",
		SegmentID:   "seg-ui-1",
		SegmentPos:  intPtr(7),
		ChoiceBlock: "choice-ui",
	}, `{"type":"object"}`, false)

	if !containsAll(prompt,
		`"resource_key":"UI_1"`,
	) {
		t.Fatalf("prompt missing pack metadata fields:\n%s", prompt)
	}
	for _, forbidden := range []string{
		`"source_type"`,
		`"source_file"`,
		`"meta_path_label"`,
		`"segment_id"`,
		`"segment_pos"`,
		`"choice_block_id"`,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt should not contain %s:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildSinglePrompt_IncludesSourceFileOnlyForShortTextassetCue(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:         "id-short",
		BodyEN:     "No.",
		TextRole:   "dialogue",
		SourceType: "textasset",
		SourceFile: "AR_Viira",
		MetaPath:   "Assets/TextAsset/AR_Viira.bytes:12",
	}, `{"type":"object"}`, false)

	if !strings.Contains(prompt, `"source_file":"AR_Viira"`) {
		t.Fatalf("prompt missing source_file:\n%s", prompt)
	}
	if strings.Contains(prompt, `"meta_path_label"`) {
		t.Fatalf("prompt should not contain meta_path_label:\n%s", prompt)
	}
}

func TestBuildSinglePrompt_OmitsSourceFileForContextRichTextasset(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:         "id-long",
		BodyEN:     "Tell him to leave before the others arrive.",
		ContextEN:  "We should move quickly.\nTell him to leave before the others arrive.",
		TextRole:   "dialogue",
		SourceType: "textasset",
		SourceFile: "AR_Viira",
	}, `{"type":"object"}`, false)

	if strings.Contains(prompt, `"source_file":"AR_Viira"`) {
		t.Fatalf("prompt should omit source_file for context-rich text:\n%s", prompt)
	}
}

func TestExtractPlainTranslation_StripsQuotedLine(t *testing.T) {
	got := extractPlainTranslation("\"이건 그냥 쓰레기통이에요. 즐기세요.\"")
	if got != "이건 그냥 쓰레기통이에요. 즐기세요." {
		t.Fatalf("got=%q", got)
	}
}

func TestExtractIndexedTranslations(t *testing.T) {
	raw := "0\t첫째 번역\n1\t둘째 번역"
	got := extractIndexedTranslations(raw)
	if len(got) != 2 || got[0] != "첫째 번역" || got[1] != "둘째 번역" {
		t.Fatalf("got=%v", got)
	}
}

func TestExtractStringArray(t *testing.T) {
	got := extractStringArray(`["첫째","둘째"]`)
	if len(got) != 2 || got[0] != "첫째" || got[1] != "둘째" {
		t.Fatalf("got=%v", got)
	}
}

func TestExtractStringArray_StripsFenceAndEscapedOuterBrackets(t *testing.T) {
	got := extractStringArray("```json\n\\[\\\"첫째\\\",\\\"둘째\\\"\\]\n```")
	if len(got) != 2 || got[0] != "첫째" || got[1] != "둘째" {
		t.Fatalf("got=%v", got)
	}
}

func TestExtractStringArray_PreservesEscapedInnerQuotes(t *testing.T) {
	raw := `["(한숨을 쉰다.) \"좋아. 뭘 알고 싶은데?"]`
	got := extractStringArray(raw)
	if len(got) != 1 {
		t.Fatalf("len=%d, got=%v", len(got), got)
	}
	if got[0] != `(한숨을 쉰다.) "좋아. 뭘 알고 싶은데?` {
		t.Fatalf("got=%q", got[0])
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
