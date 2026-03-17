package evaluation

import "localize-agent/workflow/pkg/shared"

func runEvalItem(client *evalClient, slotKey string, item *packItem, maxAttempts int, backoffSec float64, maxRetry int) itemOutcome {
	out := itemOutcome{id: item.ID, finalKO: item.ProposedKORestored, finalRisk: item.Risk, finalNotes: item.Notes}
	currentKO := item.ProposedKORestored

	for attempt := 0; attempt <= maxRetry; attempt++ {
		raw, err := shared.CallWithRetry(func() (string, error) {
			return client.sendPrompt(slotKey, kindEval, buildEvalPrompt(map[string]any{
				"id": item.ID, "en": item.EN, "ko": currentKO, "risk": out.finalRisk, "notes": out.finalNotes,
			}, client.evalShapeHint()))
		}, maxAttempts, backoffSec)
		if err != nil || raw == "" {
			out.finalStatus = statusPass
			return out
		}
		evs := extractEvalResults(raw)
		if len(evs) == 0 {
			out.finalStatus = statusPass
			return out
		}
		ev := evs[0]
		ev.ID = item.ID
		out.history = append(out.history, ev)

		switch ev.Verdict {
		case "pass":
			out.finalKO = currentKO
			out.finalStatus = statusPass
			return out
		case "reject":
			out.finalKO = currentKO
			out.finalStatus = statusReject
			return out
		case "revise":
			if attempt == maxRetry {
				out.finalKO = currentKO
				out.finalStatus = statusRevise
				return out
			}
			revRaw, revErr := shared.CallWithRetry(func() (string, error) {
				return client.sendPrompt(slotKey, kindTrans, buildRevisePrompt(item.ID, item.EN, item.CurrentKO, currentKO, ev.Issues, client.transShapeHint()))
			}, maxAttempts, backoffSec)
			if revErr != nil || revRaw == "" {
				out.finalKO = currentKO
				out.finalStatus = statusRevise
				return out
			}
			revised := extractRevised(revRaw)
			if len(revised) == 0 {
				out.finalKO = currentKO
				out.finalStatus = statusRevise
				return out
			}
			currentKO = revised[0].ProposedKO
			out.finalRisk = revised[0].Risk
			out.finalNotes = revised[0].Notes
			out.revised = true
		default:
			out.finalKO = currentKO
			out.finalStatus = statusPass
			return out
		}
	}
	if out.finalStatus == "" {
		out.finalStatus = statusPass
	}
	return out
}
