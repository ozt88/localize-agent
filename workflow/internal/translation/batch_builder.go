package translation

type batchBuildResult struct {
	runItems       []map[string]string
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
		runItems:       []map[string]string{},
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

		maskedEn, maps := maskTags(enText)
		maskedCur, _ := maskTags(curText)
		out.runItems = append(out.runItems, map[string]string{
			"id":         id,
			"en":         maskedEn,
			"current_ko": maskedCur,
		})
		out.metas[id] = itemMeta{
			id:      id,
			enText:  maskedEn,
			curText: maskedCur,
			curObj:  curObj,
			mapTags: maps,
		}
	}

	return out
}
