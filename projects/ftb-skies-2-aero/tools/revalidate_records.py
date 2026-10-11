"""Replay local validation only, retaining immutable semantic review evidence."""
import time
from run_pack_translation import PROJECT, sha
from workflow.pkg.input_optimization.batch_execution import load, write
from workflow.pkg.input_optimization.core import digest
from workflow.pkg.localization_formats.minecraft_text import validate


def main():
    out=PROJECT/'output/pack-translation-20261011';changes=[]
    for file in (out/'batches').glob('*/records.json'):
        rows=load(file);changed=False
        for row in rows:
            findings=validate(row['source'],row['translation'])
            if findings==row['failures']:continue
            changes.append(dict(id=row['id'],source_sha256=digest(row['source']),
                target_sha256=digest(row['translation']),before=row['failures'],after=findings))
            row['failures']=findings;changed=True
        if changed:
            write(file,rows);result=load(file.parent/'result.json')
            result['failures']={r['id']:r['failures'] for r in rows if r['failures']}
            write(file.parent/'result.json',result)
    if changes:
        write(out/'validation-replays'/f'{time.time_ns()}.json',dict(changes=changes,model_calls=0,
            validator_sha256=sha(PROJECT.parents[1]/'workflow/pkg/localization_formats/minecraft_text.py')))
    print('Local findings updated',len(changes),'model calls 0')


if __name__=='__main__':main()
