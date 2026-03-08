package translation

import "strings"

type batchBuildResult struct {
	runItems       []translationTask
	metas          map[string]itemMeta
	skippedInvalid int
	skippedLong    int
	skippedLongIDs []string
}

func buildBatch(rt translationRuntime, batchIDs []string) batchBuildResult {
	filteredIDs := batchIDs
	if len(rt.doneFromCheckpoint) > 0 {
		tmp := make([]string, 0, len(batchIDs))
		for _, id := range batchIDs {
			if !rt.doneFromCheckpoint[id] {
				tmp = append(tmp, id)
			}
		}
		filteredIDs = tmp
	}

	out := batchBuildResult{
		runItems:       []translationTask{},
		metas:          map[string]itemMeta{},
		skippedLongIDs: []string{},
	}

	for _, id := range filteredIDs {
		enObj, ok := rt.sourceStrings[id]
		if !ok {
			out.skippedInvalid++
			continue
		}
		curObj, ok := rt.currentStrings[id]
		if !ok {
			out.skippedInvalid++
			continue
		}

		enText, _ := enObj["Text"].(string)
		curText, _ := curObj["Text"].(string)
		if rt.cfg.MaxPlainLen > 0 && len([]rune(enText)) > rt.cfg.MaxPlainLen {
			out.skippedLong++
			out.skippedLongIDs = append(out.skippedLongIDs, id)
			continue
		}

		profile := effectiveTextProfile(rt, id, enText)
		prepared := preparePromptText(enText, curText, profile)
		if prepared.passthrough {
			base := curObj
			base["Text"] = enText
			out.metas[id] = itemMeta{
				id:              id,
				sourceRaw:       enText,
				enText:          enText,
				curText:         curText,
				curObj:          curObj,
				profile:         profile,
				passthrough:     true,
				translationLane: laneDefault,
			}
			continue
		}
		chunkEN := chunkPromptText(rt, id)
		textRole := ""
		speakerHint := ""
		isShortContext := false
		lane := laneDefault
		if ctx, ok := rt.lineContexts[id]; ok {
			textRole = ctx.TextRole
			speakerHint = ctx.SpeakerHint
			isShortContext = ctx.LineIsShortContextDependent
		}
		lane = decideTranslationLane(enText, profile, textRole, isShortContext)
		contextEN := buildContextEN(rt, id, chunkEN, profile, isShortContext)
		out.runItems = append(out.runItems, translationTask{
			ID:          id,
			BodyEN:      prepared.source,
			ContextEN:   contextEN,
			TextRole:    textRole,
			SpeakerHint: speakerHint,
			GroupKey:    profileGroupKey(profile),
			Lane:        lane,
			Profile:     profile,
		})
		out.metas[id] = itemMeta{
			id:              id,
			sourceRaw:       enText,
			enText:          prepared.source,
			curText:         prepared.current,
			contextEN:       contextEN,
			textRole:        textRole,
			speakerHint:     speakerHint,
			curObj:          curObj,
			mapTags:         prepared.tagMaps,
			profile:         profile,
			choicePrefix:    prepared.choicePrefix,
			controlPrefix:   prepared.controlPrefix,
			emphasisSpans:   prepared.emphasisSpans,
			passthrough:     false,
			translationLane: lane,
		}
	}

	return out
}

func neighborPromptText(rt translationRuntime, id string, delta int, source bool) string {
	if ctx, ok := rt.lineContexts[id]; ok {
		neighborID := ""
		if delta < 0 {
			neighborID = ctx.PrevLineID
		} else if delta > 0 {
			neighborID = ctx.NextLineID
		}
		if neighborID != "" {
			return promptTextForID(rt, neighborID, source)
		}
	}
	pos, ok := rt.idIndex[id]
	if !ok {
		return ""
	}
	neighborPos := pos + delta
	if neighborPos < 0 || neighborPos >= len(rt.ids) {
		return ""
	}
	neighborID := rt.ids[neighborPos]
	return promptTextForID(rt, neighborID, source)
}

func promptTextForID(rt translationRuntime, id string, source bool) string {
	var obj map[string]any
	if source {
		obj = rt.sourceStrings[id]
	} else {
		obj = rt.currentStrings[id]
	}
	if obj == nil {
		return ""
	}
	text, _ := obj["Text"].(string)
	if strings.TrimSpace(text) == "" {
		return ""
	}
	profile := classifyTextProfile(text)
	prepared := preparePromptText(text, text, profile)
	return prepared.source
}

func chunkPromptText(rt translationRuntime, id string) string {
	ctx, ok := rt.lineContexts[id]
	if !ok || len(ctx.Chunk.LineIDs) == 0 {
		return ""
	}
	lines := make([]string, 0, len(ctx.Chunk.LineIDs))
	for _, lineID := range ctx.Chunk.LineIDs {
		obj := rt.sourceStrings[lineID]
		if obj == nil {
			continue
		}
		text, _ := obj["Text"].(string)
		if strings.TrimSpace(text) == "" {
			continue
		}
		profile := effectiveTextProfile(rt, lineID, text)
		prepared := preparePromptText(text, text, profile)
		lines = append(lines, prepared.source)
	}
	return strings.Join(lines, "\n")
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func effectiveTextProfile(rt translationRuntime, id, enText string) textProfile {
	profile := classifyTextProfile(enText)
	if profile.Kind == textKindChoice {
		return profile
	}
	ctx, ok := rt.lineContexts[id]
	if !ok {
		return profile
	}
	switch ctx.TextRole {
	case "choice":
		profile.Kind = textKindChoice
	case "narration":
		profile.Kind = textKindNarration
	case "dialogue", "reaction", "fragment":
		profile.Kind = textKindDialogue
	}
	if ctx.LineHasEmphasis {
		profile.HasRichText = true
	}
	return profile
}

func buildContextEN(rt translationRuntime, id, chunkEN string, profile textProfile, isShortContext bool) string {
	if strings.TrimSpace(chunkEN) != "" {
		return chunkEN
	}
	if !(isShortContext || profile.Kind == textKindChoice || profile.HasRichText) {
		return ""
	}
	parts := []string{}
	if prev := neighborPromptText(rt, id, -1, true); strings.TrimSpace(prev) != "" {
		parts = append(parts, prev)
	}
	if next := neighborPromptText(rt, id, 1, true); strings.TrimSpace(next) != "" {
		parts = append(parts, next)
	}
	return strings.Join(parts, "\n")
}
