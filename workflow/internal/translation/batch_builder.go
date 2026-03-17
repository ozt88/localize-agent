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

		cpMeta := checkpointMetaForID(rt, id)
		textRole := cpMeta.TextRole
		speakerHint := cpMeta.SpeakerHint
		isShortContext := false
		if ctx, ok := rt.lineContexts[id]; ok {
			if textRole == "" {
				textRole = ctx.TextRole
			}
			if speakerHint == "" {
				speakerHint = ctx.SpeakerHint
			}
			isShortContext = ctx.LineIsShortContextDependent
		}

		profile := effectiveTextProfile(rt, id, enText)
		prepared := preparePromptText(enText, curText, profile)
		retryReason := firstNonEmpty(retryReasonForID(rt, id), cpMeta.RetryReason)
		if cpMeta.TranslationPolicy == "preserve" || shouldPreserveInternalUILabel(enText, textRole, retryReason, cpMeta.SourceFile) {
			prepared.passthrough = true
		}
		if prepared.passthrough {
			base := curObj
			base["Text"] = enText
			out.metas[id] = itemMeta{
				id:              id,
				sourceRaw:       enText,
				enText:          enText,
				curText:         curText,
				curObj:          curObj,
				contextEN:       cpMeta.ContextEN,
				prevEN:          cpMeta.PrevEN,
				nextEN:          cpMeta.NextEN,
				prevKO:          cpMeta.PrevKO,
				nextKO:          cpMeta.NextKO,
				textRole:        textRole,
				speakerHint:     speakerHint,
				retryReason:     retryReason,
				translationPolicy: cpMeta.TranslationPolicy,
				sourceType:      cpMeta.SourceType,
				sourceFile:      cpMeta.SourceFile,
				resourceKey:     cpMeta.ResourceKey,
				metaPathLabel:   cpMeta.MetaPathLabel,
				sceneHint:       cpMeta.SceneHint,
				segmentID:       cpMeta.SegmentID,
				segmentPos:      cpMeta.SegmentPos,
				choiceBlockID:   cpMeta.ChoiceBlockID,
				prevLineID:      cpMeta.PrevLineID,
				nextLineID:      cpMeta.NextLineID,
				profile:         profile,
				passthrough:     true,
				translationLane: laneDefault,
			}
			continue
		}
		chunkLines, chunkLineIndex := chunkPromptLines(rt, id)
		chunkEN := strings.Join(chunkLines, "\n")
		prevEN := firstNonEmpty(cpMeta.PrevEN, neighborPromptText(rt, id, -1, true))
		nextEN := firstNonEmpty(cpMeta.NextEN, neighborPromptText(rt, id, 1, true))
		prevKO := sanitizePromptKoreanReference(prevEN, cpMeta.PrevKO)
		nextKO := sanitizePromptKoreanReference(nextEN, cpMeta.NextKO)
		includeRetryContext := rt.cfg.UseCheckpointCurrent || retryReason != ""
		currentKOPrompt := ""
		prevKOPrompt := ""
		nextKOPrompt := ""
		if includeRetryContext {
			currentKOPrompt = firstNonEmpty(
				sanitizePromptKoreanReference(enText, cpMeta.CurrentKO),
				sanitizePromptKoreanReference(enText, prepared.current),
			)
			prevKOPrompt = prevKO
			nextKOPrompt = nextKO
		}
		lane := laneDefault
		lane = decideTranslationLane(enText, profile, textRole, isShortContext)
		statCheck := normalizeStatCheck(prepared.choicePrefix)
		choiceMode := inferChoiceMode(textRole, profile, statCheck)
		contextEN := ""
		if !isUIRole(textRole) {
			contextEN = firstNonEmpty(cpMeta.ContextEN, buildContextEN(rt, id, chunkEN, profile, isShortContext))
		}
		out.runItems = append(out.runItems, translationTask{
			ID:           id,
			BodyEN:       prepared.source,
			ContextEN:    contextEN,
			ContextLines: chunkLines,
			ContextLine:  chunkLineIndex,
			StatCheck:    statCheck,
			ChoiceMode:   choiceMode,
			IsStatCheck:  statCheck != "",
			CurrentKO:    currentKOPrompt,
			PrevEN:       prevEN,
			NextEN:       nextEN,
			PrevKO:       prevKOPrompt,
			NextKO:       nextKOPrompt,
			TextRole:     textRole,
			SpeakerHint:  speakerHint,
			RetryReason:  retryReason,
			Glossary:     matchedGlossaryEntries(rt.glossaryEntries, enText),
			SourceType:   cpMeta.SourceType,
			SourceFile:   cpMeta.SourceFile,
			ResourceKey:  cpMeta.ResourceKey,
			MetaPath:     cpMeta.MetaPathLabel,
			SegmentID:    cpMeta.SegmentID,
			SegmentPos:   cpMeta.SegmentPos,
			ChoiceBlock:  cpMeta.ChoiceBlockID,
			GroupKey:     profileGroupKey(profile),
			Lane:         lane,
			Profile:      profile,
		})
		out.metas[id] = itemMeta{
			id:              id,
			sourceRaw:       enText,
			enText:          prepared.source,
			curText:         prepared.current,
			contextEN:       contextEN,
			prevEN:          prevEN,
			nextEN:          nextEN,
			prevKO:          prevKO,
			nextKO:          nextKO,
			textRole:        textRole,
			speakerHint:     speakerHint,
			retryReason:     retryReason,
			translationPolicy: cpMeta.TranslationPolicy,
			sourceType:      cpMeta.SourceType,
			sourceFile:      cpMeta.SourceFile,
			resourceKey:     cpMeta.ResourceKey,
			metaPathLabel:   cpMeta.MetaPathLabel,
			sceneHint:       cpMeta.SceneHint,
			segmentID:       cpMeta.SegmentID,
			segmentPos:      cpMeta.SegmentPos,
			choiceBlockID:   cpMeta.ChoiceBlockID,
			prevLineID:      cpMeta.PrevLineID,
			nextLineID:      cpMeta.NextLineID,
			curObj:          curObj,
			mapTags:         prepared.tagMaps,
			profile:         profile,
			choicePrefix:    prepared.choicePrefix,
			statCheck:       statCheck,
			choiceMode:      choiceMode,
			isStatCheck:     statCheck != "",
			controlPrefix:   prepared.controlPrefix,
			emphasisSpans:   prepared.emphasisSpans,
			passthrough:     false,
			translationLane: lane,
		}
	}

	return out
}

func checkpointMetaForID(rt translationRuntime, id string) checkpointPromptMeta {
	if rt.checkpointMetas == nil {
		return checkpointPromptMeta{}
	}
	return rt.checkpointMetas[id]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func retryReasonForID(rt translationRuntime, id string) string {
	if rt.retryReasons == nil {
		return ""
	}
	return strings.TrimSpace(rt.retryReasons[id])
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
	return ""
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

func chunkPromptLines(rt translationRuntime, id string) ([]string, int) {
	ctx, ok := rt.lineContexts[id]
	if !ok || len(ctx.Chunk.LineIDs) == 0 {
		return nil, -1
	}
	lines := make([]string, 0, len(ctx.Chunk.LineIDs))
	targetLine := -1
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
		if lineID == id {
			targetLine = len(lines)
		}
		lines = append(lines, formatContextLine(rt, lineID, prepared.source))
	}
	if len(lines) == 0 {
		return nil, -1
	}
	return lines, targetLine
}

func formatContextLine(rt translationRuntime, id, text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	ctx, ok := rt.lineContexts[id]
	if !ok || ctx.TextRole != "dialogue" {
		return text
	}
	speaker := strings.TrimSpace(ctx.SpeakerHint)
	if speaker == "" {
		if meta, ok := rt.checkpointMetas[id]; ok {
			speaker = strings.TrimSpace(meta.SpeakerHint)
		}
	}
	if speaker == "" {
		return text
	}
	return speaker + ": " + text
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func normalizeStatCheck(choicePrefix string) string {
	prefix := strings.TrimSpace(choicePrefix)
	if prefix == "" {
		return ""
	}
	prefix = strings.TrimSuffix(prefix, "-")
	fields := strings.Fields(prefix)
	if len(fields) < 2 {
		return ""
	}
	head := strings.ToUpper(fields[0])
	attr := strings.ToUpper(fields[1])
	switch {
	case strings.HasPrefix(head, "ROLL"):
		num := strings.TrimPrefix(head, "ROLL")
		if num == "" {
			return ""
		}
		return attr + " " + num
	case strings.HasPrefix(head, "DC"):
		num := strings.TrimPrefix(head, "DC")
		if num == "" {
			return ""
		}
		return attr + " " + num
	case strings.HasPrefix(head, "FC"):
		num := strings.TrimPrefix(head, "FC")
		if num == "" {
			return ""
		}
		return attr + " " + num
	default:
		return ""
	}
}

func inferChoiceMode(textRole string, profile textProfile, statCheck string) string {
	if textRole == "choice" {
		if statCheck != "" {
			return "choice_stat_check"
		}
		return "choice"
	}
	if statCheck != "" && profile.Kind == textKindChoice {
		return "stat_check_action"
	}
	return ""
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
	case "ui_label", "ui_description", "tooltip", "button":
		profile.Kind = textKindNarration
	}
	if ctx.LineHasEmphasis {
		profile.HasRichText = true
	}
	return profile
}

func buildContextEN(rt translationRuntime, id, chunkEN string, profile textProfile, isShortContext bool) string {
	if meta := checkpointMetaForID(rt, id); isUIRole(meta.TextRole) {
		return ""
	}
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
