"""Offline triage hints, not automatic semantic failures or blanket model audits."""
from collections import Counter
import re
from run_pack_translation import PROJECT
from workflow.pkg.input_optimization.batch_execution import load, write
from workflow.pkg.localization_formats.minecraft_text import component, text_paths, value_at, COLOR, MARKER, RESOURCE, URL


def visible(text):
    c=component(text)
    if c is not None:text=' '.join(value_at(c,p) for p in text_paths(c))
    for pattern in (COLOR,MARKER,URL,RESOURCE):text=pattern.sub('',text)
    return text


def main():
    out=PROJECT/'output/pack-translation-20261011';hints=[]
    for path in (out/'batches').glob('*/records.json'):
        for row in load(path):
            a,b=visible(row['source']),visible(row['translation'])
            reasons=[]
            nums=lambda s:Counter(x.replace(',','') for x in re.findall(r'\d[\d,]*(?:\.\d+)?',s))
            if nums(a) and nums(a)!=nums(b):reasons.append('numeric_form_difference_needs_interpretation')
            if len(re.findall(r'[A-Za-z]+',a))>=6 and not re.search(r'[가-힣]',b):
                reasons.append('long_text_without_hangul')
            if row['source'].count('\n')!=row['translation'].count('\n'):
                reasons.append('explicit_newline_count_changed')
            if reasons:hints.append(dict(id=row['id'],batch=path.parent.name,source=a,translation=b,hints=reasons))
    write(out/'visible-text-triage.json',hints)
    print('triage',len(hints))
    for h in hints:print(h)


if __name__=='__main__':main()
