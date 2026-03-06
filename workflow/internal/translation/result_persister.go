package translation

import (
	"fmt"
	"os"
	"sync"

	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/internal/shared"
)

type persistResult struct {
	pack           []map[string]any
	skippedInvalid int
	abortWorker    bool
}

func persistResults(
	rt translationRuntime,
	sessionKey string,
	proposals map[string]proposal,
	metas map[string]itemMeta,
	done map[string]map[string]any,
	pack []map[string]any,
	doneMu *sync.Mutex,
	cpWriter *checkpointBatchWriter,
) persistResult {
	out := persistResult{pack: pack}

	for id, p := range proposals {
		meta, ok := metas[id]
		if !ok {
			out.skippedInvalid++
			continue
		}
		if p.Risk == "" {
			p.Risk = "low"
		}

		restored, restoreErr := restoreWithRecovery(rt, sessionKey, id, p, meta)
		if restoreErr != nil {
			if rt.cfg.SkipInvalid {
				out.skippedInvalid++
				continue
			}
			fmt.Fprintf(os.Stderr, "restore error for %s: %v\n", id, restoreErr)
			out.abortWorker = true
			return out
		}

		base := meta.curObj
		base["Text"] = restored
		packObj := map[string]any{
			"id":                   id,
			"en":                   meta.enText,
			"current_ko":           meta.curText,
			"proposed_ko_restored": restored,
			"risk":                 p.Risk,
			"notes":                p.Notes,
		}
		doneMu.Lock()
		done[id] = base
		out.pack = append(out.pack, packObj)
		doneMu.Unlock()

		if rt.checkpoint.IsEnabled() {
			sourceHash := fmt.Sprintf("%x", len(meta.enText))
			item := contracts.TranslationCheckpointItem{
				EntryID:    id,
				Status:     "done",
				SourceHash: sourceHash,
				Attempts:   0,
				LastError:  "",
				LatencyMs:  0,
				KOObj:      base,
				PackObj:    packObj,
			}
			if cpWriter != nil {
				if err := cpWriter.Enqueue(item); err != nil {
					fmt.Fprintf(os.Stderr, "checkpoint enqueue error for %s: %v\n", id, err)
					out.abortWorker = true
					return out
				}
			} else {
				if err := rt.checkpoint.UpsertItem(id, "done", sourceHash, 0, "", 0, base, packObj); err != nil {
					fmt.Fprintf(os.Stderr, "checkpoint write error for %s: %v\n", id, err)
					out.abortWorker = true
					return out
				}
			}
		}
	}

	return out
}

func restoreWithRecovery(rt translationRuntime, sessionKey, id string, p proposal, meta itemMeta) (string, error) {
	restored, restoreErr := restoreTags(p.ProposedKO, meta.mapTags)
	if restoreErr == nil || rt.cfg.PlaceholderRecoveryAttempts <= 0 {
		return restored, restoreErr
	}

	exp := make([]string, 0, len(meta.mapTags))
	for _, m := range meta.mapTags {
		exp = append(exp, m.placeholder)
	}
	shape := rt.skill.shapeHint()
	rraw, rerr := shared.CallWithRetry(func() (string, error) {
		return rt.client.sendPrompt(sessionKey, buildRecoveryPrompt(id, maskNoErr(meta.enText), maskNoErr(meta.curText), p.ProposedKO, exp, shape))
	}, rt.cfg.PlaceholderRecoveryAttempts, rt.cfg.BackoffSec)
	if rerr != nil {
		return restored, restoreErr
	}
	if robj := extractObjects(rraw); len(robj) > 0 {
		return restoreTags(robj[0].ProposedKO, meta.mapTags)
	}
	return restored, restoreErr
}
