import json
from pathlib import Path
import tempfile
import unittest
from .controller_usage import measure


class ControllerAccounting(unittest.TestCase):
    def test_inherited_usage_is_not_charged_to_new_request(self):
        def usage(i,c,o):return {'type':'event_msg','payload':{'type':'token_count','info':{'total_token_usage':{
            'input_tokens':i,'cached_input_tokens':c,'output_tokens':o,'total_tokens':i+o}}}}
        events=[usage(1000000,900000,1000),{'type':'response_item','payload':{
            'type':'message','role':'user','content':[{'text':'Start scoped work'}]}},usage(1000300,900200,1050)]
        with tempfile.TemporaryDirectory() as t:
            p=Path(t)/'rollout.jsonl';p.write_text('\n'.join(json.dumps(x) for x in events))
            result=measure(p,'Start scoped work')['tokens']
            self.assertEqual(result['total_tokens'],350)
            self.assertEqual(result['uncached_input_tokens'],100)

    def test_no_boundary_is_not_silently_counted_as_zero(self):
        with tempfile.TemporaryDirectory() as t:
            p=Path(t)/'rollout.jsonl';p.write_text('')
            with self.assertRaises(ValueError):measure(p,'missing')


if __name__=='__main__':unittest.main()
