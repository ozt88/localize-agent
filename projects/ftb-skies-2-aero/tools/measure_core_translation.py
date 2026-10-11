"""Offline first-pass/final quality and per-stage token measurement."""
from collections import Counter
import json
from pathlib import Path
from run_pack_translation import PROJECT
from workflow.pkg.input_optimization.batch_execution import load, write
from workflow.pkg.input_optimization.cost_accounting import aggregate
from workflow.pkg.localization_formats.minecraft_text import validate


def main():
    out=PROJECT/'output/pack-translation-20261011'
    issue_counts=Counter();first_failed=corrected=held=final_failed=items=0
    per_stage={};timings=[]
    for folder in sorted((out/'batches').glob('*')):
        if not (folder/'result.json').exists():continue
        result=load(folder/'result.json');frozen=load(folder/'input.json');answer=load(folder/'translate/response.json')
        for n,row in enumerate(frozen['items'],1):
            issues=validate(row['source'],answer[str(n)])
            first_failed+=bool(issues);issue_counts.update(issues);items+=1
        corrected+=len(result['corrections']);held+=len(result['holds']);final_failed+=len(result['failures'])
        for name in result['stages']:
            usage=load(folder/(name+'.usage.json'))
            stage='repair' if name.startswith('repair-') else name
            per_stage.setdefault(stage,[]).extend(dict(session_id=k,**v) for k,v in usage['sessions'].items())
            path=folder/name/'run.json';run=load(path)
            timings.append((path.stat().st_mtime-run['seconds'],path.stat().st_mtime,run['seconds']))
    duration=max(t[1] for t in timings)-min(t[0] for t in timings) if timings else 0
    measured=dict(completed_new_items=items,first_pass_structural_failures=first_failed,
                  initial_findings=dict(issue_counts),review_corrected_items=corrected,
                  final_structural_failures=final_failed,held_items=held,
                  stage_usage={k:aggregate(v) for k,v in per_stage.items()},
                  completed_model_seconds=sum(t[2] for t in timings),settled_wall_seconds=duration,
                  observed_parallel_factor=sum(t[2] for t in timings)/duration if duration else None,
                  semantic_quality_measure='independent model review; not a human/in-game quality score')
    term_result=out/'term-alignment/result.json'
    if term_result.exists():
        measured['targeted_term_corrections']=load(term_result)['corrected']
        measured['term_alignment_usage']=load(out/'term-alignment/review.usage.json')
    ui_result=out/'ui-term-alignment/result.json'
    if ui_result.exists():
        measured['current_ui_term_review']=load(ui_result)
        measured['current_ui_term_usage']=aggregate([
            dict(session_id=k,**v) for p in (out/'ui-term-alignment').glob('*/*.usage.json')
            for k,v in load(p)['sessions'].items()])
    write(out/'quality-metrics.json',measured)
    print(json.dumps({k:(v['tokens'] if isinstance(v,dict) and 'tokens' in v else v)
                      for k,v in measured.items() if k!='stage_usage'},ensure_ascii=False))


if __name__=='__main__':main()
