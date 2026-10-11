import copy
import unittest
import json
import subprocess
import sys
from pathlib import Path
from .evidence_selection import select,restore,expand


class EvidenceSelectionTests(unittest.TestCase):
    def test_exact_restoration_and_expansion(self):
        source={'items':[{'source':'{nc|A|B}','condition':False,'runtime':{'x':[0,False,None]}}]}
        path=['items',0,'runtime']
        view,audit=select(source,[{'path':path,'reason':'experimental runtime view'}],[['items',0,'condition']])
        self.assertEqual(restore(view,audit),source)
        self.assertEqual(expand(view,audit,[path])[0],source)
        self.assertFalse(audit['semantic_equivalence_verified'])
        self.assertIn('runtime',source['items'][0])

    def test_required_overlap_and_array_removal_rejected(self):
        source={'a':{'b':1},'items':[1]}
        for path,required in [(['a'],[['a','b']]),(['a','b'],[['a']]),(['items',0],[])]:
            with self.assertRaises(ValueError):select(source,[{'path':path,'reason':'test'}],required)
        with self.assertRaises(ValueError):select(source,[{'path':['a'],'reason':'x'},{'path':['a','b'],'reason':'x'}])

    def test_tampering_and_unknown_expansion_rejected(self):
        view,audit=select({'a':1,'b':False},[{'path':['a'],'reason':'test'}],unresolved=['unknown link'])
        bad=copy.deepcopy(audit);bad['entries'][0]['value']=2
        with self.assertRaises(ValueError):restore(view,bad)
        with self.assertRaises(ValueError):restore({'b':True},audit)
        with self.assertRaises(ValueError):expand(view,audit,[['z']])
        self.assertEqual(audit['unresolved'],['unknown link'])

    def test_ordered_list_subset_is_explicit_and_restorable(self):
        source={'edges':[{'id':'a'},{'id':'b'},{'id':'a'}]}
        view,audit=select(source,[{'path':['edges'],'reason':'explicit edge selection','keep_indices':[0,2]}])
        self.assertEqual(len(view['edges']),2)
        self.assertEqual(restore(view,audit),source)
        self.assertEqual(expand(view,audit,[['edges']])[0],source)
        for indices in ([2,0],[0,0],[True],[3]):
            with self.assertRaises(ValueError):select(source,[{'path':['edges'],'reason':'test','keep_indices':indices}])

    def test_public_cli_roundtrip_without_game_or_budget_schema(self):
        root=Path(__file__).resolve().parents[3]
        def call(operation,value):
            result=subprocess.run([sys.executable,'-m','workflow.pkg.input_optimization',operation],
                input=json.dumps(value),capture_output=True,text=True,cwd=root,check=True)
            return json.loads(result.stdout)
        source={'dialogue':['A','B'],'editor_only':{'panel':False}}
        selected=call('select-evidence',{'source':source,'deferrals':[{'path':['editor_only'],'reason':'editor layout'}]})
        self.assertEqual(call('restore-evidence',selected),source)
        self.assertFalse(selected['approved'])
