"""Targeted review against exact current installed UI names, never old sentences."""
import argparse
from concurrent.futures import ThreadPoolExecutor
import json
import re
import zipfile
from runtime_config import instance_path
from run_pack_translation import ROOT, PROJECT, RULES, sha
from workflow.pkg.input_optimization.batch_execution import call, load, write
from workflow.pkg.input_optimization import admission, integrated_review
from workflow.pkg.localization_formats.minecraft_text import validate, COLOR

INSTANCE = instance_path()
# Aliases are triage triggers only. The reviewer resolves actual occurrences in context.
TERMS = [
 ('apotheosis','block.apotheosis.salvaging_table','Salvaging Table',['분해대']),
 ('ae2','block.ae2.inscriber','Inscriber',['인스크라이버','각인기']),
 ('ars_nouveau','block.ars_nouveau.source_jar','Source Jar',['소스 항아리','소스 단지']),
 ('bhc','item.bhc.heart_amulet','Heart Amulet',['하트 아뮬렛','하트 부적']),
 ('create','block.create.depot','Depot',['디포','데포']),
 ('integrateddynamics','block.integrateddynamics.drying_basin','Drying Basin',['건조 수조','건조 대야']),
 ('integrateddynamics','block.integrateddynamics.logic_programmer','Logic Programmer',['로직 프로그래머']),
 ('irons_spellbooks','block.irons_spellbooks.arcane_anvil','Arcane Anvil',['비전 모루']),
 ('mekanism','block.mekanism.enrichment_chamber','Enrichment Chamber',['농축실','농축 챔버']),
 ('mekanism','block.mekanism.rotary_condensentrator','Rotary Condensentrator',['회전식 응축기']),
 ('modern_industrialization','block.modern_industrialization.heat_exchanger','Heat Exchanger',['열교환기']),
 ('modern_industrialization','block.modern_industrialization.styrene','Styrene',['스타이렌']),
 ('modern_industrialization','block.modern_industrialization.vacuum_freezer','Vacuum Freezer',['진공 냉동기']),
 ('modern_industrialization','block.modern_industrialization.helium_plasma','Helium Plasma',['헬륨 플라스마']),
 ('modern_industrialization','block.modern_industrialization.diethyl_ether','Diethyl Ether',['다이에틸 에터','다이에틸 에테르']),
 ('modern_industrialization','block.modern_industrialization.distillery','Distillery',['증류탑','증류기']),
]


def prepare(out):
    folder=out/'ui-term-alignment'
    if (folder/'input.json').exists():return load(folder/'input.json')
    evidence=[]
    for jar in sorted((INSTANCE/'mods').glob('*.jar')):
        with zipfile.ZipFile(jar) as z:
            names=set(z.namelist())
            for namespace,key,english,aliases in TERMS:
                enpath=f'assets/{namespace}/lang/en_us.json';kopath=f'assets/{namespace}/lang/ko_kr.json'
                if enpath not in names or kopath not in names:continue
                en=json.loads(z.read(enpath));ko=json.loads(z.read(kopath))
                if key not in en or key not in ko:continue
                if en[key]!=english:raise ValueError('Installed English identity mismatch: '+key)
                evidence.append(dict(source_term=english,korean_term=ko[key],aliases=aliases,
                    current_concept_identity=key,status='approved',scope='Exact current installed item concept only',
                    korean_authority='Bundled Korean UI of current installed mod; not a claim of official linguistic authority',
                    evidence=[dict(path=str(jar),sha256=sha(jar),english_entry=enpath,korean_entry=kopath)]))
    if len(evidence)!=len(TERMS):raise ValueError('Missing or ambiguous current installed term evidence')
    plan=load(out/'plan.json');contexts={i['id']:d['context'] for b in plan['batches'] for d in b['documents'] for i in d['items']}
    rows=[]
    for file in sorted((out/'batches').glob('*/records.json')):
        for row in load(file):
            matched=[]
            for term in evidence:
                en=term['source_term']
                pattern=r'\b'+(r'Distiller(?:y|ies)' if en=='Distillery' else re.escape(en)+r's?')+r'\b'
                if en=='Distillery' and 'modern_industrialization:' not in json.dumps(contexts[row['id']]):continue
                if re.search(pattern,COLOR.sub('',row['source']),re.I) and any(a in row['translation'] for a in term['aliases']):matched.append(term)
            if matched:rows.append(dict(record=row,file=str(file),context=contexts[row['id']],terms=matched))
    frozen=dict(terms=evidence,rows=rows)
    write(folder/'input.json',frozen)
    return frozen


def main():
    ap=argparse.ArgumentParser();ap.add_argument('--runner',required=True);ap.add_argument('--codex-bin',required=True)
    ap.add_argument('--prepare-only',action='store_true');args=ap.parse_args()
    out=PROJECT/'output/pack-translation-20261011';folder=out/'ui-term-alignment';frozen=prepare(out)
    print('TARGETED',len(frozen['rows']),'items',len(frozen['terms']),'verified current terms',flush=True)
    if args.prepare_only:return
    groups=[frozen['rows'][n:n+45] for n in range(0,len(frozen['rows']),45)]
    def review(entry):
        n,rows=entry;directory=folder/f'{n:04d}'
        items=[dict(id=x['record']['id'],source=x['record']['source']) for x in rows]
        candidates={x['record']['id']:x['record']['translation'] for x in rows}
        issues={x['record']['id']:['current_ui_term:'+t['current_concept_identity'] for t in x['terms']] for x in rows}
        context={x['record']['id']:dict(quest=x['context'],verified_terms=x['terms']) for x in rows}
        job=integrated_review.job(items,candidates,{i['id']:validate(i['source'],candidates[i['id']]) for i in items},
            context=context,review_issues=issues,instructions=RULES+
            ' Match the exact current installed Korean UI name for the supplied item concept. Correct only necessary term occurrences and grammatical particles, preserving adequate other wording. Alias matches are triage hints, not proof of a wrong occurrence. Distillery and Distillation Tower are DIFFERENT machines: never replace a Tower occurrence with 증류소. Do not translate identifiers. Resolve each supplied issue explicitly using the current item identity and source meaning.')
        response=call(directory,'review',job,args.runner,args.codex_bin,ROOT)
        final,held,changed=integrated_review.accept(response,items,candidates,job['id_map'],job['review_contract'])
        dispositions=integrated_review.dispositions(response,job['review_contract'])
        for i in items:
            k=i['id']
            if set(dispositions.get(k,{}))!=set(issues[k]):held[k]='Missing explicit terminology disposition'
            if validate(i['source'],final[k]):held[k]='Structural validation failed'
        write(directory/'result.json',dict(final=final,held=held,changed=changed,dispositions=dispositions))
        print('REVIEWED',n,len(rows),'changed',len(changed),'held',len(held),flush=True)
        return directory,rows,final,held,changed
    with ThreadPoolExecutor(max_workers=3) as pool:results=list(pool.map(review,enumerate(groups)))
    if any(held for _,_,_,held,_ in results):raise ValueError('Targeted UI review has unresolved items')
    updates={};changes=0
    for directory,rows,final,held,changed in results:
        changes+=len(changed)
        refs=[dict(path=str(p),sha256=sha(p)) for p in (folder/'input.json',directory/'review.job.json',directory/'review/response.json',directory/'review/trace.jsonl',directory/'result.json')]
        for x in rows:updates[x['record']['id']]=(final[x['record']['id']],refs)
    for path in sorted((out/'batches').glob('*/records.json')):
        records=load(path)
        if not any(r['id'] in updates for r in records):continue
        backup=path.parent/'before-ui-term-alignment.records.json'
        if not backup.exists():write(backup,records)
        for r in records:
            if r['id'] not in updates:continue
            target,refs=updates[r['id']];r['translation']=target
            r['review']=admission.proof(r['source'],target,r['review']['evidence']+refs)
            r['failures']=validate(r['source'],target)
        write(path,records)
    registry=load(PROJECT/'context/approved-terminology.json');keys={t['current_concept_identity'] for t in frozen['terms']}
    registry['terms']=[t for t in registry['terms'] if t.get('current_concept_identity') not in keys]+frozen['terms']
    write(PROJECT/'context/approved-terminology.json',registry)
    write(folder/'result.json',dict(reviewed=len(updates),corrected=changes,translation_calls=0,holds=0))


if __name__=='__main__':main()
