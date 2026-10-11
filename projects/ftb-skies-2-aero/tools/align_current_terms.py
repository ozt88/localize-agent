"""Correct proven same-concept term inconsistencies without retranslating batches."""
import argparse
from pathlib import Path
import zipfile
from runtime_config import instance_path
from run_pack_translation import ROOT, PROJECT, RULES, sha
from workflow.pkg.input_optimization.batch_execution import call, load, write
from workflow.pkg.input_optimization import admission, integrated_review
from workflow.pkg.localization_formats.minecraft_text import validate


def main():
    ap=argparse.ArgumentParser();ap.add_argument('--runner',type=Path,required=True)
    ap.add_argument('--codex-bin',type=Path,required=True);ap.add_argument('--prepare-only',action='store_true')
    args=ap.parse_args();out=PROJECT/'output/pack-translation-20261011';folder=out/'term-alignment'
    jar=instance_path()/'mods/fargostalismans-1.21.1-2.3.1.jar'
    with zipfile.ZipFile(jar) as z:
        import json
        en=json.loads(z.read('assets/fargostalismans/lang/en_us.json'))
        assert any(k.endswith('_talisman') and 'Talisman' in v for k,v in en.items())
    decision=dict(source_term='Talisman',korean_term='부적',current_concept_identity='item.fargostalismans.*_talisman',
                  evidence=[dict(path=str(jar),sha256=sha(jar),entry='assets/fargostalismans/lang/en_us.json')],
                  source_version='fargostalismans 2.3.1 / FTB Skies 2 Aero 1.13.0',status='approved',
                  scope='Fargo talisman item names and descriptions only; mod title unchanged',
                  korean_authority='Project editorial choice, not claimed official Korean; 85 existing consistent instances preferred over 65 variants')
    registry=load(PROJECT/'context/approved-terminology.json')
    registry['terms']=[t for t in registry['terms'] if t.get('current_concept_identity')!=decision['current_concept_identity']]+[decision]
    write(PROJECT/'context/approved-terminology.json',registry)
    files=[out/'batches'/n/'records.json' for n in ('0057','0058','0059')]
    targets=[r for f in files for r in load(f) if '탈리스만' in r['translation']]
    if (folder/'items.json').exists():targets=load(folder/'items.json')
    else:write(folder/'items.json',targets)
    proposed={r['id']:r['translation'].replace('탈리스만','부적') for r in targets}
    write(folder/'decision.json',decision)
    issue='inconsistent_current_term:Talisman'
    if args.prepare_only:
        for file in files:
            rows=load(file)
            for r in rows:
                if r['id'] in proposed:r['open_issues']=sorted(set(r.get('open_issues',[])+[issue]))
            write(file,rows)
        print('Targeted term correction prepared',len(targets));return
    items=[dict(id=r['id'],source=r['source']) for r in targets]
    # Review final proposed text once. Previous translations/evidence remain recorded.
    context={'terminology_decision':decision,'documents':[]}
    plan=load(out/'plan.json')
    for batch in plan['batches']:
        for d in batch['documents']:
            if any(i['id'] in proposed for i in d['items']):context['documents'].append(d['context'])
    job=integrated_review.job(items,proposed,{r['id']:validate(r['source'],proposed[r['id']]) for r in targets},
          context=context,instructions=RULES+' For this verified current Fargo item class, Talisman is 부적. Check the proposed terminology correction and source fidelity; retain adequate other wording.')
    response=call(folder,'review',job,args.runner,args.codex_bin,ROOT)
    final,held,applied=integrated_review.accept(response,items,proposed,job['id_map'],job['review_contract'])
    if held or any(validate(i['source'],final[i['id']]) for i in items):
        write(folder/'unresolved.json',dict(held=held,final=final));raise ValueError('Term correction needs resolution')
    refs=[dict(path=str(folder/n),sha256=sha(folder/n)) for n in ('items.json','decision.json','review.job.json','review/response.json','review/trace.jsonl')]
    for file in files:
        rows=load(file)
        backup=file.parent/'before-term-alignment.records.json'
        if not backup.exists():write(backup,rows)
        for r in rows:
            if r['id'] not in final:continue
            r['translation']=final[r['id']]
            r['review']=admission.proof(r['source'],r['translation'],r['review']['evidence']+refs)
            r['open_issues']=[x for x in r.get('open_issues',[]) if x!=issue]
            r['failures']=validate(r['source'],r['translation'])
        write(file,rows)
    write(folder/'result.json',dict(corrected=len(final),additional_review_changes=applied,holds=held,translation_calls=0))
    print('Term correction complete',len(final))


if __name__=='__main__':main()
