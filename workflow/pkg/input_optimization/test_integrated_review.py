import json
import unittest
from .integrated_review import job, accept


class IntegratedReviewTests(unittest.TestCase):
    def setUp(self):
        self.items = [{'id': 'long-id-a', 'source': 'Hello'}, {'id': 'long-id-b', 'source': 'World'}]
        self.values = {'long-id-a': '안녕', 'long-id-b': '세계'}
        self.job = job(self.items, self.values, [], {'catalog': {'scene:1': {'summary': 'scene'}}})
        self.answer = {'coverage_sha256': self.job['review_contract']['coverage_sha256'],
                       'changes': [], 'holds': [], 'context_requests': []}

    def accept(self, answer=None, values=None):
        return accept(answer or self.answer, self.items, values or self.values,
                      self.job['id_map'], self.job['review_contract'])

    def patch(self, local='1', value='안녕하세요', quote='Hello'):
        return {'id': local, 'translation': value, 'source_quote': quote,
                'translation_quote': '안녕', 'reason': 'grammar'}

    def test_compact_ids_and_coverage_are_exact(self):
        self.assertEqual(self.job['id_map'], {'1': 'long-id-a', '2': 'long-id-b'})
        self.assertNotIn('long-id-a', self.job['prompt'])
        self.answer['changes'] = [self.patch()]
        values, held, applied = self.accept()
        self.assertEqual(values['long-id-a'], '안녕하세요')
        self.assertEqual(applied, ['long-id-a']); self.assertFalse(held)
        with self.assertRaises(ValueError): self.accept(values={**self.values, 'long-id-b': '변경됨'})
        self.answer['coverage_sha256'] = 'wrong'
        with self.assertRaises(ValueError): self.accept()

    def test_invalid_patch_is_item_local(self):
        self.answer['changes'] = [self.patch(quote='invented')]
        values, held, applied = self.accept()
        self.assertEqual(values, self.values); self.assertEqual(set(held), {'long-id-a'})
        self.assertEqual(applied, [])

    def test_noop_is_ignored_and_explicit_hold_wins(self):
        self.answer['changes'] = [self.patch(value='안녕')]
        self.assertEqual(self.accept(), (self.values, {}, []))
        self.answer['changes'] = [self.patch()]
        self.answer['holds'] = [{'id': '1', 'reason': 'source ambiguous'}]
        self.assertEqual(self.accept(), (self.values, {'long-id-a': 'source ambiguous'}, []))

    def test_unknown_and_duplicate_ids_fail_closed(self):
        self.answer['changes'] = [self.patch(local='long-id-a')]
        with self.assertRaises(ValueError): self.accept()
        self.answer['changes'] = [self.patch(), self.patch()]
        with self.assertRaises(ValueError): self.accept()

    def test_context_requests_are_exact_and_never_pass(self):
        self.answer['context_requests'] = [{'id': '2', 'catalog_ids': ['scene:1'], 'reason': 'speaker unclear'}]
        values, held, applied = self.accept()
        self.assertEqual(set(held), {'long-id-b'}); self.assertFalse(applied)
        self.answer['context_requests'][0]['catalog_ids'] = ['invented']
        with self.assertRaises(ValueError): self.accept()

    def test_context_and_findings_change_coverage(self):
        other = job(self.items, self.values, [{'id': 'long-id-a', 'kind': 'markup'}])
        self.assertNotEqual(other['review_contract']['coverage_sha256'], self.answer['coverage_sha256'])
        with self.assertRaises(ValueError): accept(self.answer, self.items, self.values)

    def test_context_links_use_local_ids_but_catalog_handles_stay_exact(self):
        j = job(self.items, self.values, [], {'node': {'text_id': 'long-id-a'},
                'context_catalog': {'long-id-a': 'exact retrieval handle'}})
        packet = json.loads(j['prompt'].split('\n', 1)[1].split('\nTASK INSTRUCTIONS:\n', 1)[0])
        self.assertEqual(packet['context']['node']['text_id'], '1')
        self.assertIn('long-id-a', packet['context']['context_catalog'])
        self.assertEqual(j['review_contract']['context_ids'], ['long-id-a'])

    def test_catalog_is_not_duplicated_in_schema_and_compression_is_lossless(self):
        context = {'context_catalog': {'evidence:' + str(i): 'Read exact evidence' for i in range(120)},
                   'nodes': [{'speaker': 'same speaker', 'conditions': [], 'text': 'Repeated evidence ' * 20}
                             for _ in range(30)]}
        j = job(self.items, self.values, [], context)
        prop = j['schema']['properties']['context_requests']['items']['properties']['catalog_ids']['items']
        self.assertEqual(prop, {'type': 'string'})
        metadata = j['input_optimization']
        self.assertLess(metadata['prompt_cost'], metadata['alternatives']['plain'])
        # optimize verifies every representation by exact roundtrip before selection.
        self.assertIn(metadata['representation'], ('record-table-v1', 'record-table-v2'))
        answer = {**self.answer, 'coverage_sha256': j['review_contract']['coverage_sha256'],
                  'context_requests': [{'id': '1', 'catalog_ids': ['invented'], 'reason': 'missing'}]}
        with self.assertRaises(ValueError):
            accept(answer, self.items, self.values, j['id_map'], j['review_contract'])

    def test_repeated_placeholder_pairs_remain_literal_outside_codec(self):
        items = [{'id': 'a', 'source': 'Placeholder'}, {'id': 'b', 'source': 'Placeholder'}]
        j = job(items, {'a': 'Placeholder', 'b': 'Placeholder'}, [],
                {'records': [{'kind': 'same', 'value': 'repeated metadata ' * 40} for _ in range(30)]})
        packet = json.loads(j['prompt'].split('\n', 1)[1].split('\nTASK INSTRUCTIONS:\n', 1)[0])
        self.assertEqual(packet['dynamic']['targets'], [
            {'id': '1', 'source': 'Placeholder', 'translation': 'Placeholder'},
            {'id': '2', 'source': 'Placeholder', 'translation': 'Placeholder'}])

    def test_elided_handle_is_requestable_but_unknown_handle_fails(self):
        context = {'node': {'evidence_id': 'scene:1', 'speaker': 'Alice'},
                   'catalog': {'scene:1': 'REDUNDANT_DESCRIPTOR', 'other:2': 'KEEP_DESCRIPTOR'}}
        j = job(self.items, self.values, [], context, reference_fields=('evidence_id',))
        self.assertNotIn('REDUNDANT_DESCRIPTOR', j['prompt'])
        self.assertIn('KEEP_DESCRIPTOR', j['prompt'])
        self.assertEqual(j['review_contract']['context_ids'], ['scene:1', 'other:2'])
        self.assertIn('scene:1', context['catalog'])  # No mutation of source evidence.
        answer = {**self.answer, 'coverage_sha256': j['review_contract']['coverage_sha256'],
                  'context_requests': [{'id': '1', 'catalog_ids': ['scene:1'], 'reason': 'conditions'}]}
        result, held, applied = accept(answer, self.items, self.values, j['id_map'], j['review_contract'])
        self.assertEqual(result, self.values)
        self.assertEqual(set(held), {'long-id-a'})
        self.assertFalse(applied)
        answer['context_requests'][0]['catalog_ids'] = ['invented']
        with self.assertRaises(ValueError):
            accept(answer, self.items, self.values, j['id_map'], j['review_contract'])

    def test_elided_descriptor_changes_coverage_and_default_stays_inline(self):
        context = {'node': {'evidence_id': 'scene:1'}, 'catalog': {'scene:1': 'ORIGINAL_DESCRIPTOR'}}
        a = job(self.items, self.values, [], context, reference_fields=('evidence_id',))
        self.assertIn('ORIGINAL_DESCRIPTOR', job(self.items, self.values, [], context)['prompt'])
        context['catalog']['scene:1'] = 'CHANGED_DESCRIPTOR'
        b = job(self.items, self.values, [], context, reference_fields=('evidence_id',))
        self.assertNotEqual(a['review_contract']['coverage_sha256'], b['review_contract']['coverage_sha256'])

    def test_list_catalog_preserves_opaque_handle_and_semantic_context(self):
        context = {'node': {'text_id': 'long-id-a', 'evidence_id': 'long-id-a', 'speaker': 'Alice'},
                   'context_catalog': [{'id': 'long-id-a', 'description': 'REDUNDANT_DESCRIPTOR'}]}
        j = job(self.items, self.values, [], context, reference_fields=('evidence_id',))
        packet = json.loads(j['prompt'].split('\n', 1)[1].split('\nTASK INSTRUCTIONS:\n', 1)[0])
        self.assertEqual(packet['context']['node'],
                         {'text_id': '1', 'evidence_id': 'long-id-a', 'speaker': 'Alice'})
        self.assertEqual(packet['context']['context_catalog'], [])
        self.assertNotIn('maxItems', j['schema']['properties']['context_requests'])

    def test_catalog_self_reference_cannot_elide_unexposed_handles(self):
        context = {'node': {'unconfigured_field': 'scene:1'},
                   'catalog': {'scene:1': {'evidence_id': 'scene:1', 'description': 'KEEP_DESCRIPTOR'}}}
        j = job(self.items, self.values, [], context, reference_fields=('evidence_id',))
        self.assertIn('KEEP_DESCRIPTOR', j['prompt'])
        self.assertEqual(j['reference_catalog']['elided_handles'], [])
        with self.assertRaises(ValueError):
            job(self.items, self.values, [], context, reference_fields='evidence_id')


if __name__ == '__main__': unittest.main()
