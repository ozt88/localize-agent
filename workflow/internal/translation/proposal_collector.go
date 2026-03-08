package translation

import "localize-agent/workflow/internal/shared"

func collectProposals(rt translationRuntime, slotKey string, runItems []translationTask) (map[string]proposal, int, int) {
	proposals := map[string]proposal{}
	skippedInvalid := 0
	skippedTranslatorErr := 0
	grouped := groupRunItemsByKind(runItems)
	for _, group := range grouped {
		p, si, se := collectProposalsForGroup(rt, slotKey, group)
		for id, prop := range p {
			proposals[id] = prop
		}
		skippedInvalid += si
		skippedTranslatorErr += se
	}
	return proposals, skippedInvalid, skippedTranslatorErr
}

func collectProposalsForGroup(rt translationRuntime, slotKey string, runItems []translationTask) (map[string]proposal, int, int) {
	proposals := map[string]proposal{}
	skippedInvalid := 0
	skippedTranslatorErr := 0
	expectedIDs := map[string]bool{}
	for _, it := range runItems {
		expectedIDs[it.ID] = true
	}
	shape := rt.skill.shapeHint()
	client := rt.clientForLane(runItems[0].Lane)
	sessionKey := client.sessionKey(slotKey)
	plainOutput := client.usesPlainTranslatorOutput()

	if len(runItems) == 1 {
		one := runItems[0]
		raw, err := shared.CallWithRetry(func() (string, error) {
			return client.sendPrompt(sessionKey, buildSinglePrompt(one, shape, plainOutput))
		}, rt.cfg.MaxAttempts, rt.cfg.BackoffSec)
		if err != nil {
			skippedTranslatorErr++
			return proposals, skippedInvalid, skippedTranslatorErr
		}
		if plainOutput {
			text := extractPlainTranslation(raw)
			if text == "" {
				skippedInvalid++
				return proposals, skippedInvalid, skippedTranslatorErr
			}
			if isDegenerateProposal(one.BodyEN, text) {
				skippedInvalid++
				return proposals, skippedInvalid, skippedTranslatorErr
			}
			proposals[one.ID] = proposal{ID: one.ID, ProposedKO: text}
			return proposals, skippedInvalid, skippedTranslatorErr
		}
		objs := extractObjects(raw)
		if len(objs) == 0 {
			skippedInvalid++
			return proposals, skippedInvalid, skippedTranslatorErr
		}
		picked := proposal{}
		okPicked := false
		for _, obj := range objs {
			if expectedIDs[obj.ID] {
				picked = obj
				okPicked = true
				break
			}
		}
		if !okPicked {
			picked = objs[0]
			picked.ID = one.ID
		}
		if isDegenerateProposal(one.BodyEN, picked.ProposedKO) {
			skippedInvalid++
			return proposals, skippedInvalid, skippedTranslatorErr
		}
		proposals[one.ID] = picked
		return proposals, skippedInvalid, skippedTranslatorErr
	}

	raw, err := shared.CallWithRetry(func() (string, error) {
		return client.sendPrompt(sessionKey, buildBatchPrompt(runItems, shape, plainOutput))
	}, rt.cfg.MaxAttempts, rt.cfg.BackoffSec)
	if err != nil {
		for _, one := range runItems {
			r2, e2 := shared.CallWithRetry(func() (string, error) {
				return client.sendPrompt(sessionKey, buildSinglePrompt(one, shape, plainOutput))
			}, rt.cfg.MaxAttempts, rt.cfg.BackoffSec)
			if e2 != nil {
				skippedTranslatorErr++
				continue
			}
			if plainOutput {
				text := extractPlainTranslation(r2)
				if text == "" {
					skippedInvalid++
					continue
				}
				if isDegenerateProposal(one.BodyEN, text) {
					skippedInvalid++
					continue
				}
				proposals[one.ID] = proposal{ID: one.ID, ProposedKO: text}
				continue
			}
			objs := extractObjects(r2)
			if len(objs) == 0 {
				skippedInvalid++
				continue
			}
			if isDegenerateProposal(one.BodyEN, objs[0].ProposedKO) {
				skippedInvalid++
				continue
			}
			proposals[one.ID] = objs[0]
		}
		return proposals, skippedInvalid, skippedTranslatorErr
	}
	if plainOutput {
		indexed := extractIndexedTranslations(raw)
		for idx, one := range runItems {
			text, ok := indexed[idx]
			if !ok {
				continue
			}
			if isDegenerateProposal(one.BodyEN, text) {
				skippedInvalid++
				continue
			}
			proposals[one.ID] = proposal{ID: one.ID, ProposedKO: text}
		}
	} else {
		objs := extractObjects(raw)
		for _, p := range objs {
			if expectedIDs[p.ID] {
				src := ""
				for _, it := range runItems {
					if it.ID == p.ID {
						src = it.BodyEN
						break
					}
				}
				if isDegenerateProposal(src, p.ProposedKO) {
					skippedInvalid++
					continue
				}
				proposals[p.ID] = p
			}
		}
	}

	for _, one := range runItems {
		if _, ok := proposals[one.ID]; ok {
			continue
		}
		r2, e2 := shared.CallWithRetry(func() (string, error) {
			return client.sendPrompt(sessionKey, buildSinglePrompt(one, shape, plainOutput))
		}, rt.cfg.MaxAttempts, rt.cfg.BackoffSec)
		if e2 != nil {
			skippedTranslatorErr++
			continue
		}
		if plainOutput {
			text := extractPlainTranslation(r2)
			if text == "" {
				skippedInvalid++
				continue
			}
			if isDegenerateProposal(one.BodyEN, text) {
				skippedInvalid++
				continue
			}
			proposals[one.ID] = proposal{ID: one.ID, ProposedKO: text}
			continue
		}
		o2 := extractObjects(r2)
		if len(o2) == 0 {
			skippedInvalid++
			continue
		}
		if isDegenerateProposal(one.BodyEN, o2[0].ProposedKO) {
			skippedInvalid++
			continue
		}
		proposals[one.ID] = o2[0]
	}
	return proposals, skippedInvalid, skippedTranslatorErr
}

func groupRunItemsByKind(runItems []translationTask) [][]translationTask {
	grouped := map[string][]translationTask{}
	order := make([]string, 0, len(runItems))
	for _, item := range runItems {
		key := runItemGroupKey(item)
		if len(grouped[key]) == 0 {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], item)
	}
	out := make([][]translationTask, 0, len(grouped))
	for _, key := range order {
		if len(grouped[key]) > 0 {
			out = append(out, grouped[key])
		}
	}
	return out
}

func runItemGroupKey(item translationTask) string {
	return item.Lane + "::" + item.GroupKey
}
