package translation

import (
	"fmt"
	"strings"

	"localize-agent/workflow/pkg/shared"
)

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
		prop, ok, invalid, transErr := collectSingleProposalWithFallback(rt, slotKey, one, shape)
		skippedInvalid += invalid
		skippedTranslatorErr += transErr
		if ok {
			proposals[one.ID] = prop
		}
		return proposals, skippedInvalid, skippedTranslatorErr
	}

	raw, err := shared.CallWithRetry(func() (string, error) {
		return client.sendPrompt(sessionKey, buildBatchPrompt(runItems, shape, plainOutput))
	}, rt.cfg.MaxAttempts, rt.cfg.BackoffSec)
	if err != nil {
		for _, one := range runItems {
			prop, ok, invalid, transErr := collectSingleProposalWithFallback(rt, slotKey, one, shape)
			skippedInvalid += invalid
			skippedTranslatorErr += transErr
			if ok {
				proposals[one.ID] = prop
			}
		}
		return proposals, skippedInvalid, skippedTranslatorErr
	}
	if plainOutput {
		rows := extractStringArray(raw)
		duplicateRows := findDuplicateBatchOutputs(runItems, rows)
		for idx, one := range runItems {
			if idx >= len(rows) {
				continue
			}
			if duplicateRows[idx] {
				fmt.Printf("[translate-invalid] id=%s mode=batch reason=duplicate_batch_output\n", one.ID)
				skippedInvalid++
				continue
			}
			text := strings.TrimSpace(rows[idx])
			if reason := degenerateProposalReason(one.BodyEN, text); reason != "" {
				fmt.Printf("[translate-invalid] id=%s mode=batch reason=%s\n", one.ID, reason)
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
				if reason := degenerateProposalReason(src, p.ProposedKO); reason != "" {
					fmt.Printf("[translate-invalid] id=%s mode=batch_object reason=%s\n", p.ID, reason)
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
		prop, ok, invalid, transErr := collectSingleProposalWithFallback(rt, slotKey, one, shape)
		skippedInvalid += invalid
		skippedTranslatorErr += transErr
		if ok {
			proposals[one.ID] = prop
		}
	}
	return proposals, skippedInvalid, skippedTranslatorErr
}

func findDuplicateBatchOutputs(runItems []translationTask, rows []string) map[int]bool {
	if len(runItems) == 0 || len(rows) < 2 {
		return nil
	}
	type seenEntry struct {
		idx    int
		bodyEN string
	}
	seen := map[string]seenEntry{}
	dups := map[int]bool{}
	for idx, one := range runItems {
		if idx >= len(rows) {
			break
		}
		textKey := normalizedComparable(strings.TrimSpace(rows[idx]))
		if textKey == "" {
			continue
		}
		bodyKey := normalizedComparable(strings.TrimSpace(one.BodyEN))
		if prev, ok := seen[textKey]; ok {
			if prev.bodyEN != bodyKey {
				dups[prev.idx] = true
				dups[idx] = true
			}
			continue
		}
		seen[textKey] = seenEntry{idx: idx, bodyEN: bodyKey}
	}
	if len(dups) == 0 {
		return nil
	}
	return dups
}

func collectSingleProposalWithFallback(rt translationRuntime, slotKey string, one translationTask, shape string) (proposal, bool, int, int) {
	clients := []*serverClient{rt.clientForLane(one.Lane)}
	if one.Lane != laneHigh && rt.highClient != nil {
		high := rt.highClient
		already := false
		for _, c := range clients {
			if c == high {
				already = true
				break
			}
		}
		if !already {
			clients = append(clients, high)
		}
	}
	invalid := 0
	transErr := 0
	for _, client := range clients {
		if client == rt.highClient && (invalid > 0 || transErr > 0) {
			fmt.Printf("[translate-fallback] id=%s lane=%s retry=high invalid=%d err=%d\n", one.ID, one.Lane, invalid, transErr)
		}
		prop, ok, inv, terr := collectSingleProposal(rt, slotKey, one, shape, client)
		invalid += inv
		transErr += terr
		if ok {
			return prop, true, invalid, transErr
		}
	}
	return proposal{}, false, invalid, transErr
}

func collectSingleProposal(rt translationRuntime, slotKey string, one translationTask, shape string, client *serverClient) (proposal, bool, int, int) {
	sessionKey := client.sessionKey(slotKey)
	plainOutput := client.usesPlainTranslatorOutput()
	raw, err := shared.CallWithRetry(func() (string, error) {
		return client.sendPrompt(sessionKey, buildSinglePrompt(one, shape, plainOutput))
	}, rt.cfg.MaxAttempts, rt.cfg.BackoffSec)
	if err != nil {
		return proposal{}, false, 0, 1
	}
	if plainOutput {
		rows := extractStringArray(raw)
		if len(rows) != 1 {
			fmt.Printf("[translate-invalid] id=%s mode=single reason=parse_mismatch\n", one.ID)
			return proposal{}, false, 1, 0
		}
		text := strings.TrimSpace(rows[0])
		if reason := degenerateProposalReason(one.BodyEN, text); reason != "" {
			fmt.Printf("[translate-invalid] id=%s mode=single reason=%s\n", one.ID, reason)
			return proposal{}, false, 1, 0
		}
		return proposal{ID: one.ID, ProposedKO: text}, true, 0, 0
	}
	objs := extractObjects(raw)
	if len(objs) == 0 {
		fmt.Printf("[translate-invalid] id=%s mode=single_object reason=parse_mismatch\n", one.ID)
		return proposal{}, false, 1, 0
	}
	picked := objs[0]
	picked.ID = one.ID
	if reason := degenerateProposalReason(one.BodyEN, picked.ProposedKO); reason != "" {
		fmt.Printf("[translate-invalid] id=%s mode=single_object reason=%s\n", one.ID, reason)
		return proposal{}, false, 1, 0
	}
	return picked, true, 0, 0
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
	return item.GroupKey
}
