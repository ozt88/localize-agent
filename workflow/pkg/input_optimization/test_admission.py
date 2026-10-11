import unittest
from copy import deepcopy
from .admission import build, proof, unresolved


class AdmissionTests(unittest.TestCase):
    def setUp(self):
        self.row = {'id': 'a', 'source': 'Hello', 'translation': '안녕',
                    'review': proof('Hello', '안녕', [{'path': 'review', 'sha256': 'hash'}]),
                    'failures': [], 'warnings': [], 'holds': [], 'open_issues': []}

    def run_gate(self, rows=None, source=None, base=None, evidence_valid=lambda refs: True):
        return build(base or {}, rows or [self.row], source or {'a': 'Hello'}, evidence_valid)

    def test_admits_without_changing_base_and_is_idempotent(self):
        base = {'old': '기존'}
        first = self.run_gate(base=base)
        self.assertEqual(base, {'old': '기존'})
        self.assertEqual(first['counts']['admitted'], 1)
        second = self.run_gate(base=first['translations'])
        self.assertEqual(second['counts']['already_present'], 1)
        self.assertEqual(second['translations'], first['translations'])
        self.assertFalse(first['production_ready'])
        self.assertFalse(first['legacy_recertified'])

    def test_source_drift_and_unknown_id_are_blocked(self):
        for source in ({'a': 'Changed'}, {'b': 'Hello'}):
            self.assertEqual(self.run_gate(source=source)['counts']['blocked'], 1)

    def test_post_review_edit_cannot_reuse_approval(self):
        self.row['translation'] = '변경'
        report = self.run_gate()
        self.assertEqual(report['counts']['review_required'], 1)
        self.assertNotIn('a', report['translations'])

    def test_missing_context_or_changed_evidence_requires_review(self):
        self.row['review']['scopes'] = ['meaning', 'readability']
        self.assertEqual(self.run_gate()['counts']['review_required'], 1)
        self.row['review'] = proof('Hello', '안녕', [{'path': 'review', 'sha256': 'hash'}])
        self.assertEqual(self.run_gate(evidence_valid=lambda refs: False)['counts']['review_required'], 1)

    def test_unresolved_holds_and_code_failures_are_not_approved(self):
        for field in ('holds', 'failures'):
            row = deepcopy(self.row); row[field] = ['unresolved']
            self.assertEqual(self.run_gate([row])['counts']['blocked'], 1)

    def test_warnings_and_known_issues_require_disposition(self):
        for field in ('warnings', 'open_issues'):
            row = deepcopy(self.row); row[field] = ['unresolved']
            self.assertEqual(self.run_gate([row])['counts']['review_required'], 1)

    def test_conflicting_variants_never_overwrite_or_depend_on_order(self):
        other = deepcopy(self.row); other['translation'] = '다른 번역'
        for rows in ([self.row, other], [other, self.row]):
            report = self.run_gate(rows)
            self.assertEqual(report['counts']['conflict'], 1)
            self.assertEqual(report['translations'], {})
        report = self.run_gate(base={'a': '기존 번역'})
        self.assertEqual(report['translations']['a'], '기존 번역')
        self.assertEqual(report['counts']['conflict'], 1)

    def test_duplicate_cannot_hide_hold(self):
        other = deepcopy(self.row); other['holds'] = ['source uncertain']
        for rows in ([self.row, other], [other, self.row]):
            self.assertEqual(self.run_gate(rows)['counts']['blocked'], 1)

    def test_issue_disposition_is_candidate_and_evidence_bound(self):
        resolution = {**self.row['review'], 'issue': 'ambiguity', 'reason': 'Exact source context resolves it'}
        self.assertEqual(unresolved(self.row, ['ambiguity'], [resolution], lambda refs: True), [])
        self.assertEqual(unresolved(self.row, ['ambiguity'], [resolution], lambda refs: False), ['ambiguity'])
        self.row['translation'] = '변경됨'
        self.assertEqual(unresolved(self.row, ['ambiguity'], [resolution], lambda refs: True), ['ambiguity'])


if __name__ == '__main__': unittest.main()
