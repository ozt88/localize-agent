"""Read-only FTB discovery and provisional document preparation; zero model calls."""
import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path
import re
import sys
import zipfile

ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT))
from workflow.pkg.localization_formats.ftb_snbt import Document

def sha(data): return hashlib.sha256(data).hexdigest()
def write(path, data):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding='utf-8')
def obj(data, duplicates=None):
    def pairs(values):
        result = {}
        for key, value in values:
            if key in result:
                if duplicates is None: raise ValueError('Duplicate JSON key ' + key)
                duplicates.append(key)
            result[key] = value
        for key in duplicates or []: result.pop(key, None)
        return result
    return json.loads(data.decode('utf-8-sig'), object_pairs_hook=pairs)

def prepare(instance, out):
    if out.exists(): raise ValueError('Choose a new immutable inventory output directory')
    manifest = obj((instance / 'manifest.json').read_bytes())
    fingerprints, errors, rows, chapters = {}, [], [], []
    def read(path):
        data = path.read_bytes(); rel = path.relative_to(instance).as_posix()
        fingerprints[rel] = sha(data)
        target = out / 'source' / rel; target.parent.mkdir(parents=True, exist_ok=True); target.write_bytes(data)
        return data
    quest_root = instance / 'config/ftbquests/quests'
    owners = {}
    for path in sorted((quest_root / 'chapters').glob('*.snbt')):
        try:
            doc = Document(read(path).decode('utf-8-sig')); value = doc.value
            chapter = {'id': value['id'], 'file': path.name, 'quests': value.get('quests', [])}
            chapters.append(chapter)
            def visit(value, quest=None):
                if isinstance(value, dict):
                    if value in chapter['quests']: quest = value.get('id')
                    if 'id' in value: owners.setdefault(value['id'], []).append({'chapter': chapter['id'], 'quest': quest})
                    for child in value.values(): visit(child, quest)
                elif isinstance(value, list):
                    for child in value: visit(child, quest)
            visit(value)
        except (ValueError, KeyError) as exc: errors.append({'file': str(path), 'error': str(exc)})
    lang = quest_root / 'lang/en_us.snbt'
    document = Document(read(lang).decode('utf-8-sig'))
    korean_path = lang.with_name('ko_kr.snbt')
    korean = Document(read(korean_path).decode('utf-8-sig')).value if korean_path.exists() else {}
    for key, value in document.value.items():
        values = value if isinstance(value, list) else [value]
        if not all(isinstance(v, str) for v in values): raise ValueError('Non-text language field: ' + key)
        ident = key.split('.')[1] if '.' in key else ''
        for n, text in enumerate(values):
            path = (key, n) if isinstance(value, list) else (key,)
            if path not in document.spans: raise ValueError('Unquoted language field')
            rows.append({'id': 'quest:' + key + (':' + str(n) if isinstance(value, list) else ''),
                'key': key, 'path': list(path), 'source': text, 'context_owners': owners.get(ident, []),
                'translate': bool(re.search(r'[A-Za-z]', re.sub(r'\{@[^}]*\}', '', text))),
                'source_file': lang.relative_to(instance).as_posix()})
    loose = []
    for base in ['kubejs', 'resourcepacks', 'configureddefaults', 'defaultconfigs', 'datapacks', 'config/collapsiblegroups']:
        for path in sorted((instance / base).rglob('en_us.json')):
            try:
                english = obj(read(path)); kp = path.with_name('ko_kr.json'); ko = obj(read(kp)) if kp.exists() else {}
                loose.append({'file': path.relative_to(instance).as_posix(), 'english_keys': len(english),
                    'korean_keys_present': len(set(english) & set(ko)),
                    'nonstandard_path': path.parent.name != 'lang'})
                for key, text in english.items():
                    if not isinstance(text, str): continue
                    rows.append({'id': path.relative_to(instance).as_posix() + ':' + key,
                        'key': key, 'source': text,
                        'source_file': path.relative_to(instance).as_posix(), 'translate': bool(text.strip()),
                        'readiness': 'resource_path_review' if path.parent.name != 'lang' else 'needs_context_and_review'})
            except (ValueError, OSError) as exc: errors.append({'file': str(path), 'error': str(exc)})
    jars, references = [], []
    archives = list((instance / 'mods').glob('*.jar'))
    archives += list((instance / 'resourcepacks').rglob('*.zip')) + list((instance / 'kubejs/assets').glob('*.zip'))
    for archive in sorted(archives):
        try:
            with zipfile.ZipFile(archive) as z:
                names = set(z.namelist())
                for name in sorted(names):
                    if not re.fullmatch(r'assets/[^/]+/lang/en_us.json', name): continue
                    duplicates_en, duplicates_ko = [], []
                    enbytes = z.read(name); en = obj(enbytes, duplicates_en); kn = name[:-10] + 'ko_kr.json'
                    kbytes = z.read(kn) if kn in names else None; ko = obj(kbytes, duplicates_ko) if kbytes else {}
                    jars.append({'archive': archive.relative_to(instance).as_posix(), 'entry': name,
                        'english_sha256': sha(enbytes), 'korean_sha256': sha(kbytes) if kbytes else None,
                        'english_keys': len(en), 'korean_file_present': kbytes is not None,
                        'ambiguous_duplicate_keys_excluded': {'en_us': sorted(set(duplicates_en)), 'ko_kr': sorted(set(duplicates_ko))},
                        'covered_keys': len(set(en) & set(ko)), 'missing_keys': sorted(set(en)-set(ko)),
                        'unchanged_values': sum(en[k] == ko[k] for k in set(en)&set(ko))})
                    for key in set(en)&set(ko):
                        if isinstance(en[key], str) and isinstance(ko[key], str):
                            references.append({'key': key, 'source': en[key], 'korean': ko[key],
                                'archive': archive.name, 'entry': kn, 'status': 'existing_mod_reference_not_certified'})
        except (ValueError, OSError, zipfile.BadZipFile) as exc: errors.append({'file': str(archive), 'error': str(exc)})
    extra = []
    for path in sorted((instance / 'kubejs').rglob('*')):
        if path.suffix not in ('.js', '.md', '.mdx'): continue
        data = read(path); text = data.decode('utf-8-sig')
        extra.append({'file': path.relative_to(instance).as_posix(), 'bytes': len(data),
            'kind': 'guide' if path.suffix in ('.md', '.mdx') else 'script',
            'literal_ui_candidate_lines': [n+1 for n,line in enumerate(text.splitlines())
                if re.search(r'\.tell\(|\.displayName\(|\.addTooltip\(|Text\.of\(', line)],
            'status': 'manual_or_AST_review_required_not_automatic_translation'})
    changed = [name for name,h in fingerprints.items() if not (instance/name).exists() or sha((instance/name).read_bytes()) != h]
    report = {'pack': {k: manifest[k] for k in ['name','version','minecraft']},
        'instance': str(instance), 'installation_status': 'user_reported_93_percent; completion_not_certified',
        'launcher_metadata_present': (instance/'minecraftinstance.json').exists(),
        'snapshot_changed_during_scan': changed, 'provisional': True, 'paid_dispatch_allowed': False,
        'quest_chapters': len(chapters), 'quest_language_fields': len(document.value),
        'quest_text_leaves': sum(r['id'].startswith('quest:') for r in rows),
        'quest_translatable_leaves': sum(r['id'].startswith('quest:') and r['translate'] for r in rows),
        'quest_locales': [p.stem for p in (quest_root/'lang').glob('*.snbt')],
        'loose_language_files': loose, 'archives_scanned': len(archives),
        'archive_language_sets': len(jars), 'archive_sets_with_korean': sum(r['korean_file_present'] for r in jars),
        'archive_english_keys_sum_not_unique': sum(r['english_keys'] for r in jars),
        'archive_covered_keys_sum_not_unique': sum(r['covered_keys'] for r in jars),
        'errors': errors, 'model_calls': 0, 'live_state_changes': 0,
        'remaining_gates': ['installation completion and source revalidation', 'resource priority and duplicate key resolution',
          'format/control syntax validation', 'script and guide coverage', 'context-complete review packet and admission adapter', 'in-game validation']}
    write(out/'inventory.json', report); write(out/'source-manifest.json', fingerprints)
    write(out/'translation-candidates.json', {'items': rows})
    write(out/'quest-context.json', {'chapters': chapters})
    write(out/'archive-language-coverage.json', jars); write(out/'terminology-references.json', references)
    write(out/'script-guide-review-queue.json', extra)
    grouped = {}
    for row in rows:
        if not row['id'].startswith('quest:') or not row['translate']: continue
        owner = row['context_owners'][0] if len(row['context_owners']) == 1 else None
        group = (owner['chapter'] + ':' + (owner['quest'] or row['key'].split('.')[1])) if owner else row['key'].rsplit('.',1)[0]
        entry = grouped.setdefault(group, {'id': group, 'items': [], 'owner': owner,
            'readiness': 'provisional_requires_source_and_control_validation', 'production_ready': False})
        entry['items'].append(row)
    for entry in grouped.values():
        owner = entry['owner']; entry['context'] = None
        if owner:
            chapter = next(c for c in chapters if c['id'] == owner['chapter'])
            entry['context'] = {'chapter_id': chapter['id'],
                'chapter_title': document.value.get('chapter.' + chapter['id'] + '.title'),
                'quest': next((q for q in chapter['quests'] if q.get('id') == owner['quest']), None)}
    write(out/'prepared-documents.json', {'documents': list(grouped.values()), 'model_calls': 0,
        'admission_required': True, 'paid_dispatch_allowed': False,
        'historical_sentence_reuse_allowed': False,
        'reference_policy': 'Current English and current-pack context; approved term-level glossary only'})
    print(json.dumps(report, ensure_ascii=False, indent=2))

if __name__ == '__main__':
    ap=argparse.ArgumentParser(); ap.add_argument('--instance', type=Path, required=True); ap.add_argument('--out',type=Path,required=True)
    args=ap.parse_args(); prepare(args.instance.resolve(), args.out.resolve())
