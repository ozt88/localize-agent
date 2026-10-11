"""Translate explicitly identified UI string spans, preserving all JavaScript code."""
import argparse
from collections import defaultdict
import json
from pathlib import Path
import re
from run_pack_translation import ROOT, PROJECT, SNAPSHOT, RULES, sha, evidence_valid
from workflow.pkg.input_optimization.batch_execution import execute, write, load
from workflow.pkg.input_optimization import admission
from workflow.pkg.localization_formats.minecraft_text import validate

STRING=r'"(?:\\.|[^"\\])*"'


def prepare(instance):
    rows=[]
    for path in sorted((SNAPSHOT/'source/kubejs').rglob('*.js')):
        relative=path.relative_to(SNAPSHOT/'source').as_posix()
        if sha(path)!=sha(instance/relative):raise ValueError('Script drift')
        if path.name in ('_template.js','spells.js'):continue  # template and disabled debug only
        source=path.read_text(encoding='utf-8')
        spans={}
        for match in re.finditer(r'\b(?:displayName|description)\s*:\s*('+STRING+')',source):
            spans[match.start(1)]=match.end(1)
        # Confirmed live UI literals; keep command identifiers and dynamic values intact.
        explicit={'commands.js':['Total Events Triggered: ','Total ',' Events Failed: ',' Events Succeeded: '],
                  'echo_attribute.js':['echo_attribute: invalid attribute']}.get(path.name,[])
        for value in explicit:
            literal=json.dumps(value)
            for match in re.finditer(re.escape(literal),source):
                spans[match.start()]=match.end()
        for start,end in sorted(spans.items()):
            value=json.loads(source[start:end])
            if not value.strip():continue
            rows.append(dict(id=relative+':'+str(start),key=relative+':'+str(start),source=value,
                             source_file=relative,span=[start,end],
                             context=source[max(0,start-160):min(len(source),end+200)]))
    return rows


def main():
    ap=argparse.ArgumentParser();ap.add_argument('--instance',type=Path,required=True)
    ap.add_argument('--out',type=Path,required=True);ap.add_argument('--runner',type=Path,required=True)
    ap.add_argument('--codex-bin',type=Path,required=True);ap.add_argument('--prepare-only',action='store_true')
    args=ap.parse_args();out=args.out.resolve();out.mkdir(parents=True,exist_ok=True)
    rows=prepare(args.instance);write(out/'source-items.json',rows)
    print('Script UI literals',len(rows),flush=True)
    if args.prepare_only:return
    context={'usage':[dict(id=r['id'],code=r['context']) for r in rows],
             'contract':'Only quoted UI literal spans are edited. Preserve leading/trailing spaces in concatenated fragments. Technical identifiers remain unchanged.'}
    records=execute(out/'batch',rows,context,RULES,validate,args.runner,args.codex_bin,ROOT)
    bundle=admission.build({},records,{i['id']:i['source'] for i in rows},evidence_valid)
    write(out/'admission.json',bundle)
    files=defaultdict(list)
    for r in rows:
        if r['id'] in bundle['translations']:files[r['source_file']].append(r)
    for file,items in files.items():
        source=(SNAPSHOT/'source'/file).read_text(encoding='utf-8');rendered=source
        for r in sorted(items,key=lambda r:r['span'][0],reverse=True):
            start,end=r['span'];value=bundle['translations'][r['id']]
            # Quotes/escapes are serialized by JSON, a valid JS double-quoted string subset.
            rendered=rendered[:start]+json.dumps(value,ensure_ascii=False)+rendered[end:]
        dest=out/'overrides'/file;dest.parent.mkdir(parents=True,exist_ok=True);dest.write_text(rendered,encoding='utf-8')
    write(out/'result.json',dict(items=len(rows),counts=bundle['counts'],files=len(files),
                               unchanged_code=True,live_state_changes=0,production_ready=False))


if __name__=='__main__':main()
