import copy
import unittest
from .bundles import pool_scopes,unpool_scopes,compile_bundle,split_output,plan_overlap
from .core import Limits


class BundleTests(unittest.TestCase):
    def scopes(self):
        record={'id':'shared','condition':False,'branches':['{nc|A|B}','<b>literal</b>'],'comment':'context '*100}
        return [{'unit_id':str(i),'compatibility':['game','ko','policy'],
                 'targets':[{'id':f't{i}','source':'hello'}],
                 'data':{'records':[record,record], 'speaker':str(i),'source':'hello'},
                 'record_paths':[['records']]} for i in range(3)]

    def test_order_occurrences_and_scope_boundaries_restore(self):
        scopes=self.scopes();packet=pool_scopes(scopes)
        self.assertEqual(len(packet['records']),1)
        self.assertEqual(unpool_scopes(packet),{s['unit_id']:s['data'] for s in scopes})
        self.assertEqual(len(packet['scopes'][0]['bindings'][0]['indices']),2)
        bad=copy.deepcopy(packet);bad['scopes'][0]['bindings'][0]['indices']=[True]
        with self.assertRaises(ValueError):unpool_scopes(bad)

    def test_same_guid_different_body_is_not_shared(self):
        scopes=self.scopes();scopes[1]['data']['records'][0]={'id':'shared','condition':True}
        self.assertEqual(len(pool_scopes(scopes)['records']),2)

    def test_output_must_match_each_scope_and_stays_unapproved(self):
        bundle=compile_bundle(self.scopes(),Limits(50000,70000,10000))
        answer={'units':[{'unit_id':str(i),'translations':[{'id':f't{i}','text':'안녕'}]} for i in range(3)]}
        self.assertTrue(all(not r['approved'] for r in split_output(bundle,answer).values()))
        answer['units'][0]['translations'][0]['id']='t1'
        with self.assertRaises(ValueError):split_output(bundle,answer)

    def test_conflicting_policies_and_unit_budget(self):
        scopes=self.scopes();scopes[1]['compatibility']=['game','ko','different']
        with self.assertRaises(ValueError):compile_bundle(scopes,Limits(50000,70000,10000))
        result=compile_bundle(self.scopes(),Limits(50000,70000,10000,max_units=2))
        self.assertFalse(result['audit']['allowed'])

    def test_overlap_planning_conservation_cost_and_compatibility(self):
        scopes=self.scopes();scopes[2]['compatibility']=['other-game','ko','policy']
        result=plan_overlap(scopes,lambda group:{'allowed':True,'cost':100+len(group)*10},max_units=3)
        self.assertEqual(sorted(len(g['indices']) for g in result['ready']),[1,2])
        self.assertEqual(result['output_scopes'],3)
        held=plan_overlap(self.scopes(),lambda group:{'allowed':False,'cost':100})
        self.assertEqual(sum(len(g['indices']) for g in held['held']),3)

    def test_bad_paths_and_bad_evaluations_fail_closed(self):
        scopes=self.scopes();scopes[0]['record_paths']=[['records'],['records']]
        with self.assertRaises(ValueError):pool_scopes(scopes)
        with self.assertRaises(ValueError):plan_overlap(self.scopes(),lambda group:{'allowed':True,'cost':-1})


if __name__=='__main__':unittest.main()
