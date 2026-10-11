"""Review only unresolved final candidates. Never retranslates a completed batch."""
import argparse
from pathlib import Path
from run_pack_translation import ROOT, PROJECT, RULES, sha
from workflow.pkg.input_optimization.batch_execution import call, load, write
from workflow.pkg.input_optimization import admission, integrated_review
from workflow.pkg.localization_formats.minecraft_text import validate


def main():
    ap=argparse.ArgumentParser();ap.add_argument('--runner',type=Path,required=True)
    ap.add_argument('--codex-bin',type=Path,required=True);args=ap.parse_args()
    out=PROJECT/'output/pack-translation-20261011'
    for path in sorted((out/'batches').glob('*/records.json')):
        rows=load(path);targets=[r for r in rows if validate(r['source'],r['translation']) or r['holds']]
        if not targets:continue
        folder=path.parent;frozen=load(folder/'input.json')
        items=[i for i in frozen['items'] if i['id'] in {r['id'] for r in targets}]
        candidates={r['id']:r['translation'] for r in targets}
        findings={r['id']:validate(r['source'],r['translation'])+r['holds'] for r in targets}
        context={'original_context':frozen['context'],
                 'related_source_items':[dict(id=i['id'],source=i['source']) for i in frozen['items']
                                         if any(i['key'].split('.')[1]==r['id'].split('.')[1] for r in targets)]}
        instructions=RULES+' A local validated parser found the listed failures. JSON array length and node types must match exactly: do not append a new plain-text element to make Korean grammar easier. Rephrase within existing text nodes without altering click/style attachment. Review only these unresolved targets; never repeat already approved items.'
        job=integrated_review.job(items,candidates,findings,context=context,instructions=instructions)
        name='targeted-resolution'
        response=call(folder,name,job,args.runner,args.codex_bin,ROOT)
        final,held,applied=integrated_review.accept(response,items,candidates,job['id_map'],job['review_contract'])
        write(folder/'before-targeted-resolution.records.json',rows)
        for r in rows:
            if r['id'] not in final:continue
            r['translation']=final[r['id']];r['failures']=validate(r['source'],r['translation'])
            r['holds']=[held[r['id']]] if r['id'] in held else []
            refs=r['review']['evidence']+[dict(path=str(folder/n),sha256=sha(folder/n)) for n in
                    (name+'.job.json',name+'/response.json',name+'/trace.jsonl')]
            r['review']=admission.proof(r['source'],r['translation'],refs)
        write(path,rows)
        result=load(folder/'result.json')
        result['failures']={r['id']:r['failures'] for r in rows if r['failures']}
        result['holds']={r['id']:r['holds'] for r in rows if r['holds']}
        result['corrections']=sorted(set(result['corrections']+applied))
        result['stages'].append(name)
        write(folder/'result.json',result)
        print(folder.name,'targets',len(targets),'remaining',sum(bool(r['holds'] or r['failures']) for r in rows),flush=True)


if __name__=='__main__':main()
