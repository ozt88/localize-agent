package translation

import "localize-agent/workflow/internal/shared"

func collectProposals(rt translationRuntime, sessionKey string, runItems []map[string]string) (map[string]proposal, int, int) {
	proposals := map[string]proposal{}
	skippedInvalid := 0
	skippedTranslatorErr := 0
	expectedIDs := map[string]bool{}
	for _, it := range runItems {
		expectedIDs[it["id"]] = true
	}
	shape := rt.skill.shapeHint()

	if len(runItems) == 1 {
		one := runItems[0]
		raw, err := shared.CallWithRetry(func() (string, error) {
			return rt.client.sendPrompt(sessionKey, buildSinglePrompt(one["id"], one["en"], one["current_ko"], shape))
		}, rt.cfg.MaxAttempts, rt.cfg.BackoffSec)
		if err != nil {
			skippedTranslatorErr++
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
			picked.ID = one["id"]
		}
		if isDegenerateProposal(one["en"], picked.ProposedKO) {
			skippedInvalid++
			return proposals, skippedInvalid, skippedTranslatorErr
		}
		proposals[one["id"]] = picked
		return proposals, skippedInvalid, skippedTranslatorErr
	}

	raw, err := shared.CallWithRetry(func() (string, error) {
		return rt.client.sendPrompt(sessionKey, buildBatchPrompt(runItems, shape))
	}, rt.cfg.MaxAttempts, rt.cfg.BackoffSec)
	if err != nil {
		for _, one := range runItems {
			r2, e2 := shared.CallWithRetry(func() (string, error) {
				return rt.client.sendPrompt(sessionKey, buildSinglePrompt(one["id"], one["en"], one["current_ko"], shape))
			}, rt.cfg.MaxAttempts, rt.cfg.BackoffSec)
			if e2 != nil {
				skippedTranslatorErr++
				continue
			}
			objs := extractObjects(r2)
			if len(objs) == 0 {
				skippedInvalid++
				continue
			}
			if isDegenerateProposal(one["en"], objs[0].ProposedKO) {
				skippedInvalid++
				continue
			}
			proposals[one["id"]] = objs[0]
		}
		return proposals, skippedInvalid, skippedTranslatorErr
	}

	objs := extractObjects(raw)
	for _, p := range objs {
		if expectedIDs[p.ID] {
			src := ""
			for _, it := range runItems {
				if it["id"] == p.ID {
					src = it["en"]
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
	for _, one := range runItems {
		if _, ok := proposals[one["id"]]; ok {
			continue
		}
		r2, e2 := shared.CallWithRetry(func() (string, error) {
			return rt.client.sendPrompt(sessionKey, buildSinglePrompt(one["id"], one["en"], one["current_ko"], shape))
		}, rt.cfg.MaxAttempts, rt.cfg.BackoffSec)
		if e2 != nil {
			skippedTranslatorErr++
			continue
		}
		o2 := extractObjects(r2)
		if len(o2) == 0 {
			skippedInvalid++
			continue
		}
		if isDegenerateProposal(one["en"], o2[0].ProposedKO) {
			skippedInvalid++
			continue
		}
		proposals[one["id"]] = o2[0]
	}
	return proposals, skippedInvalid, skippedTranslatorErr
}
