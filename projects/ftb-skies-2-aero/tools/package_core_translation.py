"""Verify completed admission and export structure before creating an installable zip."""
from collections import defaultdict
import json
from pathlib import Path
import shutil
import subprocess
import zipfile
from runtime_config import instance_path
from run_pack_translation import PROJECT, SNAPSHOT, LANG, sha, prepare, evidence_valid
from workflow.pkg.input_optimization.batch_execution import load, write
from workflow.pkg.input_optimization import admission
from workflow.pkg.input_optimization.cost_accounting import aggregate
from workflow.pkg.localization_formats.ftb_snbt import Document
from workflow.pkg.localization_formats.minecraft_text import validate, technical_only


def main():
    out=PROJECT/'output/pack-translation-20261011'
    scripts=PROJECT/'output/script-labels-20261011'
    instance=instance_path()
    docs,original,manifest=prepare(instance)
    rows=[i for d in docs for i in d['items']]
    records=load(PROJECT/'output/quest-pilot-20261011-01/records.json')
    for p in sorted((out/'batches').glob('*/records.json')):records.extend(load(p))
    cache={}
    def valid(refs):
        key=json.dumps(refs,sort_keys=True)
        if key not in cache:cache[key]=evidence_valid(refs)
        return cache[key]
    bundle=admission.build({},records,{i['id']:i['source'] for i in rows},valid)
    expected={i['id'] for i in rows if not technical_only(i['source'])}
    if set(bundle['translations']) != expected:
        raise ValueError('Not all core targets admitted: '+str(len(expected-set(bundle['translations']))))
    srows=load(scripts/'source-items.json');srecords=load(scripts/'batch/records.json')
    sbundle=admission.build({},srecords,{i['id']:i['source'] for i in srows},valid)
    if set(sbundle['translations'])!={i['id'] for i in srows}:
        raise ValueError('Not all script targets admitted')
    # Compare every original SNBT leaf, not only changed targets.
    exported=Document((out/'overrides/config/ftbquests/quests/lang/ko_kr.snbt').read_text(encoding='utf-8'))
    source_to_id={tuple(i['path']):i['id'] for i in rows if i['id'].startswith('quest:')}
    if set(original.spans)!=set(exported.spans):raise ValueError('SNBT leaf set changed')
    for path in original.spans:
        a,b=original.value,exported.value
        for key in path:a,b=a[key],b[key]
        identifier=source_to_id.get(path)
        desired=bundle['translations'].get(identifier,a)
        if b!=desired or validate(a,b):raise ValueError('Invalid SNBT leaf: '+str(path))
    files=defaultdict(list)
    for i in rows:
        if not i['id'].startswith('quest:'):files[i['source_file']].append(i)
    for file,items in files.items():
        target=file.replace('en_us.json','ko_kr.json').replace('apotheosis/assets/','apotheosis/lang/')
        src=load(SNAPSHOT/'source'/file);dst=load(out/'overrides'/target)
        if set(src)!=set(dst):raise ValueError('JSON keys changed')
        for i in items:
            if dst[i['key']]!=bundle['translations'][i['id']] or validate(i['source'],dst[i['key']]):
                raise ValueError('JSON translation mismatch')
    script_files=defaultdict(list)
    for row in srows:script_files[row['source_file']].append(row)
    for file,items in script_files.items():
        if sha(instance/file)!=sha(SNAPSHOT/'source'/file):raise ValueError('Live script drift')
        expected_text=(SNAPSHOT/'source'/file).read_text(encoding='utf-8')
        for i in sorted(items,key=lambda i:i['span'][0],reverse=True):
            start,end=i['span']
            if json.loads(expected_text[start:end])!=i['source']:raise ValueError('Script span drift')
            expected_text=expected_text[:start]+json.dumps(sbundle['translations'][i['id']],ensure_ascii=False)+expected_text[end:]
        path=scripts/'overrides'/file
        if path.read_text(encoding='utf-8')!=expected_text:raise ValueError('Unexpected script code change')
        subprocess.run([shutil.which('node'),'--check',str(path)],check=True,capture_output=True)
    snapshots=[];call_count=0
    for directory in (out/'batches',out/'term-alignment',out/'ui-term-alignment',scripts/'batch',PROJECT/'output/quest-pilot-20261011-01'):
        for path in directory.rglob('*.usage.json'):
            u=load(path)
            if not u['complete']:raise ValueError('Uncertain usage')
            snapshots.extend(dict(session_id=k,**v) for k,v in u['sessions'].items())
            call_count+=1
    usage=aggregate(snapshots)
    package=out/'ftb-skies-2-aero-1.13.0-ko-core.zip'
    entries={}
    with zipfile.ZipFile(package,'w',zipfile.ZIP_DEFLATED) as z:
        for root in (out/'overrides',scripts/'overrides'):
            for file in sorted(root.rglob('*')):
                if not file.is_file():continue
                relative=file.relative_to(root).as_posix()
                if relative in entries:raise ValueError('Conflicting packaged file')
                entries[relative]=sha(file);z.write(file,relative)
    with zipfile.ZipFile(package) as z:
        if z.testzip() is not None:raise ValueError('Zip CRC failure')
        if set(z.namelist())!=set(entries):raise ValueError('Zip entries mismatch')
    write(out/'package-manifest.json',dict(pack='FTB Skies 2: Aero',version='1.13.0',
          source_hashes=manifest,files=entries,package_sha256=sha(package),
          install_root=str(instance),live_install_performed=False))
    write(out/'completion.json',dict(status='translation_and_offline_validation_complete',
          admitted=len(expected)+len(srows),quest_admitted=sum(i.startswith('quest:') for i in expected),
          language_admitted=sum(not i.startswith('quest:') for i in expected),script_admitted=len(srows),
          technical_preserved=sum(technical_only(i['source']) for i in rows),
          usage=usage,accounted_calls=call_count,package=str(package),files=len(entries),
          usage_scope='Measured Sol worker calls including pilot, script translation, repair and terminology alignment; controller session usage is separate',
          in_game_validation='not performed',production_ready=False,live_state_changes=0,
          source_warnings=[dict(id='quest:quest.2617A856093F36AB.quest_desc:14',
               issue='Source image directive has a malformed closing delimiter; preserved unchanged by the technical-data policy')]))
    cumulative=admission.build({},records+srecords,{i['id']:i['source'] for i in rows+srows},valid)
    write(out/'cumulative-admission.json',cumulative)
    write(out/'cumulative-source-items.json',dict(items=rows+srows))
    write(PROJECT/'output/admission-current.json',dict(kind='core_translation',
          artifact=str(out/'cumulative-admission.json'),sha256=sha(out/'cumulative-admission.json'),
          source_items=str(out/'cumulative-source-items.json'),admitted_items=len(cumulative['translations']),
          runs=sorted({r['origin'] for r in records+srecords}),production_ready=False))
    write(PROJECT/'output/core-translation-current.json',dict(completion=str(out/'completion.json'),
         admission=str(out/'admission.json'),package=str(package),package_sha256=sha(package)))
    preparation=load(PROJECT/'output/preparation-current.json')
    preparation['qualified_core']=dict(completion=str(out/'completion.json'),
          source_manifest_sha256=sha(SNAPSHOT/'source-manifest.json'),scope='quests, pack language resources, 27 script UI literals',
          in_game_validation=False,broad_mod_and_guide_dispatch_allowed=False)
    write(PROJECT/'output/preparation-current.json',preparation)
    summary=load(out/'completion.json');summary['usage']=summary['usage']['tokens']
    print(json.dumps(summary,ensure_ascii=False))


if __name__=='__main__':main()
