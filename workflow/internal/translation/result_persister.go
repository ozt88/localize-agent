package translation

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/pkg/shared"
)

type persistResult struct {
	pack           []map[string]any
	skippedInvalid int
	abortWorker    bool
}

func persistResults(
	rt translationRuntime,
	slotKey string,
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
		if p.Notes == "" {
			p.Notes = ""
		}
		restored, restoreErr := restoreWithRecovery(rt, slotKey, id, p, meta)
		if restoreErr != nil {
			if rt.cfg.SkipInvalid {
				out.skippedInvalid++
				continue
			}
			fmt.Fprintf(os.Stderr, "restore error for %s: %v\n", id, restoreErr)
			out.abortWorker = true
			return out
		}
		if err := validateRestoredOutput(meta, restored); err != nil {
			if rt.cfg.SkipInvalid {
				out.skippedInvalid++
				continue
			}
			fmt.Fprintf(os.Stderr, "postprocess validation error for %s: %v\n", id, err)
			out.abortWorker = true
			return out
		}

		currentKO := checkpointCurrentKO(meta)
		base := meta.curObj
		base["Text"] = restored
		packObj := map[string]any{
			"id":                   id,
			"en":                   meta.enText,
			"source_raw":           meta.sourceRaw,
			"current_ko":           currentKO,
			"fresh_ko":             restored,
			"context_en":           meta.contextEN,
			"prev_en":              meta.prevEN,
			"next_en":              meta.nextEN,
			"prev_ko":              meta.prevKO,
			"next_ko":              meta.nextKO,
			"text_role":            meta.textRole,
			"speaker_hint":         meta.speakerHint,
			"retry_reason":         meta.retryReason,
			"translation_policy":   meta.translationPolicy,
			"source_type":          meta.sourceType,
			"source_file":          meta.sourceFile,
			"resource_key":         meta.resourceKey,
			"meta_path_label":      meta.metaPathLabel,
			"scene_hint":           meta.sceneHint,
			"segment_id":           meta.segmentID,
			"choice_block_id":      meta.choiceBlockID,
			"prev_line_id":         meta.prevLineID,
			"next_line_id":         meta.nextLineID,
			"choice_prefix":        meta.choicePrefix,
			"stat_check":           meta.statCheck,
			"choice_mode":          meta.choiceMode,
			"is_stat_check":        meta.isStatCheck,
			"translation_lane":     meta.translationLane,
			"proposed_ko_restored": restored,
			"risk":                 p.Risk,
			"notes":                p.Notes,
			"pipeline_version":     rt.cfg.PipelineVersion,
		}
		if meta.segmentPos != nil {
			packObj["segment_pos"] = *meta.segmentPos
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

	for id, meta := range metas {
		if !meta.passthrough {
			continue
		}
		currentKO := checkpointCurrentKO(meta)
		base := meta.curObj
		base["Text"] = meta.sourceRaw
		packObj := map[string]any{
			"id":                   id,
			"en":                   meta.enText,
			"source_raw":           meta.sourceRaw,
			"current_ko":           currentKO,
			"fresh_ko":             meta.sourceRaw,
			"prev_en":              meta.prevEN,
			"next_en":              meta.nextEN,
			"prev_ko":              meta.prevKO,
			"next_ko":              meta.nextKO,
			"text_role":            meta.textRole,
			"speaker_hint":         meta.speakerHint,
			"retry_reason":         meta.retryReason,
			"translation_policy":   meta.translationPolicy,
			"source_type":          meta.sourceType,
			"source_file":          meta.sourceFile,
			"resource_key":         meta.resourceKey,
			"meta_path_label":      meta.metaPathLabel,
			"scene_hint":           meta.sceneHint,
			"segment_id":           meta.segmentID,
			"choice_block_id":      meta.choiceBlockID,
			"prev_line_id":         meta.prevLineID,
			"next_line_id":         meta.nextLineID,
			"stat_check":           meta.statCheck,
			"choice_mode":          meta.choiceMode,
			"is_stat_check":        meta.isStatCheck,
			"proposed_ko_restored": meta.sourceRaw,
			"risk":                 "low",
			"notes":                "passthrough control token",
			"pipeline_version":     rt.cfg.PipelineVersion,
		}
		if meta.segmentPos != nil {
			packObj["segment_pos"] = *meta.segmentPos
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

func checkpointCurrentKO(meta itemMeta) string {
	if meta.curObj != nil {
		if text, _ := meta.curObj["Text"].(string); strings.TrimSpace(text) != "" {
			return text
		}
	}
	return meta.curText
}

func restoreWithRecovery(rt translationRuntime, slotKey, id string, p proposal, meta itemMeta) (string, error) {
	restored, restoreErr := restorePreparedText(p.ProposedKO, meta)
	if restoreErr == nil || rt.cfg.PlaceholderRecoveryAttempts <= 0 {
		return restored, restoreErr
	}

	exp := make([]string, 0, len(meta.mapTags))
	for _, m := range meta.mapTags {
		exp = append(exp, m.placeholder)
	}
	shape := rt.skill.shapeHint()
	client := rt.clientForLane(meta.translationLane)
	sessionKey := client.sessionKey(slotKey)
	rraw, rerr := shared.CallWithRetry(func() (string, error) {
		return client.sendPrompt(sessionKey, buildRecoveryPrompt(id, maskNoErr(meta.enText), maskNoErr(meta.curText), p.ProposedKO, exp, shape))
	}, rt.cfg.PlaceholderRecoveryAttempts, rt.cfg.BackoffSec)
	if rerr != nil {
		return restored, restoreErr
	}
	if robj := extractObjects(rraw); len(robj) > 0 {
		return restorePreparedText(robj[0].ProposedKO, meta)
	}
	return restored, restoreErr
}
