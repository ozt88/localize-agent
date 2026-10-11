"""Reusable, deterministic admission gate for reviewed translation artifacts.

Adapters verify evidence and syntax. This module never calls models, changes a
database, overwrites an existing translation, or equates admission with release.
"""
from .core import digest

VERSION = 'translation-admission-v1'
SCOPES = {'meaning', 'readability', 'context', 'terminology'}


def unresolved(record, issues, resolutions, evidence_valid):
    """Explicit issue dispositions expire whenever source, candidate or evidence changes."""
    resolved = set()
    for row in resolutions:
        if (row.get('source_sha256') == digest(record['source']) and
                row.get('translation_sha256') == digest(record['translation']) and
                isinstance(row.get('reason'), str) and row['reason'].strip() and
                row.get('evidence') and evidence_valid(row['evidence'])):
            resolved.add(row.get('issue'))
    return [issue for issue in issues if issue not in resolved]


def proof(source, translation, evidence, *, scopes=SCOPES):
    return {'source_sha256': digest(source), 'translation_sha256': digest(translation),
            'scopes': sorted(scopes), 'evidence': evidence}


def assess(record, current_source, evidence_valid):
    reasons = []
    if record['id'] not in current_source:
        return 'blocked', ['unknown_source_id']
    if record['source'] != current_source[record['id']]:
        return 'blocked', ['source_changed']
    if not isinstance(record['translation'], str):
        return 'blocked', ['invalid_translation']
    if record.get('failures'):
        return 'blocked', ['structural_failure']
    if record.get('holds'):
        return 'blocked', ['unresolved_hold']
    p = record.get('review', {})
    if (p.get('source_sha256') != digest(record['source']) or
            p.get('translation_sha256') != digest(record['translation'])):
        reasons.append('final_candidate_review_not_bound')
    if not SCOPES <= set(p.get('scopes', [])):
        reasons.append('review_scope_incomplete')
    if not p.get('evidence') or not evidence_valid(p['evidence']):
        reasons.append('review_evidence_missing_or_changed')
    if record.get('warnings'):
        reasons.append('syntax_warning_needs_disposition')
    if record.get('open_issues'):
        reasons.append('open_quality_issue')
    return ('review_required', reasons) if reasons else ('eligible', [])


def build(base, records, current_source, evidence_valid):
    """Order-independent merge. All competing values are quarantined, never last-write-wins."""
    groups = {}
    for row in records:
        groups.setdefault(row['id'], []).append(row)
    translations, decisions = dict(base), {}
    for key, rows in sorted(groups.items()):
        values = {r['translation'] for r in rows if isinstance(r['translation'], str)}
        if len(values) > 1 or (key in base and values != {base[key]}):
            decisions[key] = {'status': 'conflict', 'reasons': ['competing_translation_values']}
            continue
        checks = [assess(r, current_source, evidence_valid) for r in rows]
        # An unresolved issue from any provenance cannot be hidden by another copy.
        worst = next((c for c in checks if c[0] == 'blocked'), None)
        if worst is None:
            worst = next((c for c in checks if c[0] == 'review_required'), None)
        status, reasons = worst or ('eligible', [])
        if status == 'eligible':
            status = 'already_present' if key in base else 'admitted'
            translations[key] = rows[0]['translation']
        decisions[key] = {'status': status, 'reasons': sorted(set(reasons)),
                          'origins': sorted({r.get('origin', '') for r in rows})}
    counts = {name: sum(r['status'] == name for r in decisions.values()) for name in
              ('admitted', 'already_present', 'review_required', 'blocked', 'conflict')}
    return {'translations': translations, 'decisions': decisions, 'counts': counts,
            'policy_version': VERSION, 'base_sha256': digest(base),
            'records_sha256': digest(records), 'source_sha256': digest(current_source),
            'legacy_carried_forward': len(base), 'legacy_recertified': False,
            'production_ready': False, 'live_state_changes': 0}
