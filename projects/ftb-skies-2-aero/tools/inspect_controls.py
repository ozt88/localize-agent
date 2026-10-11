import json
import re
from pathlib import Path
from collections import Counter

p = Path(__file__).resolve().parents[1] / 'output/preparation-20261011-03'
rows = json.loads((p / 'translation-candidates.json').read_text(encoding='utf-8'))['items']
strings = [r['source'] for r in rows if r.get('translate')]
bad, keys, count = [], Counter(), 0
def walk(v):
    if isinstance(v, dict):
        keys.update(v.keys())
        for child in v.values(): walk(child)
    elif isinstance(v, list):
        for child in v: walk(child)
for s in strings:
    if s.lstrip().startswith(('["', '{"', '[{"')):
        try:
            v = json.loads(s)
            walk(v)
            count += 1
        except Exception:
            bad.append(s)
print('components', count, 'keys', keys, 'invalid', bad[:5])
print('markers', list(dict.fromkeys(re.findall(r'\{[^{}]*\}', ' '.join(s for s in strings if not s.startswith(('[', '{"'))))))[:60])
print('scriptliterals', json.dumps([r for r in json.loads((p / 'script-guide-review-queue.json').read_text(encoding='utf-8')) if r.get('literal_ui_candidate_lines')], ensure_ascii=False)[:8000])
