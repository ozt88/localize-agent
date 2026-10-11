import copy
import json
import os
import random
import unittest
from .core import *


class SharedInputTests(unittest.TestCase):
    def test_ranking_cannot_reject_a_budget_valid_representation(self):
        value={'source':'whole'}
        meter=Meter('prefer-padding',lambda s:0 if 'PADDING' in s else 100,'test_only')
        alternative=('padded',value,'PADDING '*500,lambda x:x)
        chosen=optimize(value,'Translate',meter,alternatives=[alternative],ceiling=1000)
        self.assertEqual(chosen['representation'],'plain')
        self.assertTrue(chosen['representation_ceiling']['fits'])
        held=optimize(value,'Translate',meter,alternatives=[alternative],ceiling=1)
        self.assertFalse(held['representation_ceiling']['fits'])
        self.assertEqual(held['representation'],'padded')

    def limits(self,**overrides):
        values=dict(max_input=100000,context_window=120000,output_reserve=20000,max_targets=20,max_units=8)
        values.update(overrides);return Limits(**values)

    def units(self,project='game-a',n=6):
        return [{'project':project,'locale':'ko-KR','policy':'style-v1','unit_id':str(i),
                 'targets':[{'id':str(i),'source':'Hello {name} '+str(i)}],
                 'requires':['scene'],'order':i} for i in range(n)]

    def contexts(self):
        return {'scene':{'data':{'speaker':'Captain','branches':[{'if':'flag','next':'yes'},{'if':False,'next':None}],
                                 'notes':'Distinct branch; do not invent continuity. '*50},'requires':['terms']},
                'terms':{'data':{'provenance':'provisional','value':'Captain','translation':'함장'},'requires':[]}}

    def test_roundtrip_types_literals_placeholders_and_field_order(self):
        value={'$r':[1,2],'codec':'record-table-v1','dialogue':['@s3','{mf|A|B}','<b>{name}</b>'],
               'types':[False,0,0.0,None,True,-1], 'branches':[{'condition':False},{'condition':0}]}
        self.assertEqual(canonical(unpack(pack(value))),canonical(value))
        for invalid in (float('nan'),float('inf')):
            with self.assertRaises(ValueError):pack(invalid)

    def test_generated_nested_roundtrips(self):
        rng=random.Random(7)
        def gen(depth):
            if depth==0:return rng.choice([None,True,False,0,-2,0.0,'@r','{name}','한글\r\n'])
            return rng.choice([lambda:[gen(depth-1) for _ in range(3)],
                               lambda:{str(i):gen(depth-1) for i in range(3)}])()
        for _ in range(40):
            v=gen(3);self.assertEqual(canonical(unpack(pack(v))),canonical(v))

    def test_invalid_and_cyclic_references_rejected(self):
        packet=pack({'a':1})
        packet['shared']=[{'$d':0}];packet['root']={'$d':0}
        with self.assertRaises(ValueError):unpack(packet)
        for n in (True,-1,999):
            packet=pack({'a':1});packet['root']={'$r':[n]}
            with self.assertRaises(ValueError):unpack(packet)

    def test_adapter_candidate_cannot_drop_evidence(self):
        with self.assertRaisesRegex(ValueError,'lost evidence'):
            optimize({'source':'full','conditions':[1,2]},'review',alternatives=[('bad',{},'',lambda x:x)])

    def test_target_identity_stays_literal_outside_compressed_context(self):
        targets=[{'id':'localization-id','source':'Say nothing {name}'}]
        result=optimize({'objects':[{'owner':'engine-guid','body':'Repeated context '*100}]*20},'translate',targets=targets)
        payload=json.loads(result['prompt'].split('\n')[1])
        self.assertEqual(payload['dynamic']['target_contract'],targets)
        self.assertIn('never substitute engine object IDs',result['prompt'])

    def test_identical_source_prefix_across_independent_stages(self):
        context={'source':'Read all branches','branch':['A','B']}
        a=optimize(context,'translate',dynamic={'candidate':{}})
        b=optimize(context,'review',dynamic={'candidate':{'id':'후보'}})
        self.assertIn(canonical(context),os.path.commonprefix([a['prompt'],b['prompt']]))
        self.assertEqual(a['evidence_sha256'],b['evidence_sha256'])
        self.assertNotEqual(a['dynamic_sha256'],b['dynamic_sha256'])

    def test_three_unrelated_engine_shapes_preserved(self):
        shapes=[{'blueprints':[{'guid':'001','conditions':[False],'text':'{name}'}]},
                {'events':[{'code':401,'parameters':['\\V[2]']},{'code':102,'parameters':[['yes','no']]}]},
                {'knots':{'intro':{'lines':['Hello'],'choices':[{'target':'end','condition':'score>2'}]}}}]
        for shape in shapes:
            choice=optimize({'targets':[{'id':'1','source':'Hello'}],'engine_context':shape},'translate')
            payload=json.loads(choice['prompt'].split('\n')[1])
            decoded=unpack(payload) if choice['representation']=='record-table-v1' else payload
            self.assertEqual(decoded['engine_context'],shape)

    def test_shared_context_packed_once_and_results_remain_unapproved(self):
        units=self.units();contexts=self.contexts()
        result=plan(units,contexts,self.limits())
        self.assertEqual(result['held'],[]);self.assertEqual(len(result['ready']),1)
        batch=result['ready'][0]
        self.assertEqual(batch['payload']['contexts'],contexts)
        self.assertEqual(batch['payload']['units'],units)
        singles=sum(plan([u],contexts,self.limits())['ready'][0]['audit']['input_cost'] for u in units)
        self.assertLess(batch['audit']['input_cost'],singles)
        responses={u['unit_id']:{t['id']:'안녕 {name}' for t in u['targets']} for u in units}
        projected=project_results(batch,responses)
        self.assertTrue(all(not x['approved'] for x in projected.values()))
        with self.assertRaises(ValueError):project_results(batch,{})
        batch['payload']['contexts']['scene']['data']['speaker']='mutated'
        with self.assertRaises(ValueError):project_results(batch,responses)

    def test_incompatible_policies_projects_and_locales_never_mix(self):
        units=self.units(n=4)
        units[1]['policy']='other';units[2]['locale']='ja';units[3]['project']='another-game'
        self.assertEqual(len(plan(units,self.contexts(),self.limits())['ready']),4)

    def test_missing_context_and_oversized_units_held_not_truncated(self):
        units=self.units(n=2);units[0]['requires']=['missing']
        result=plan(units,self.contexts(),self.limits(max_input=20))
        self.assertEqual(result['ready'],[]);self.assertEqual(len(result['held']),2)
        self.assertEqual(result['held'][0]['unit'],units[0])
        self.assertEqual(result['held'][1]['unit'],units[1])
        self.assertEqual(result['input_units'],result['output_units'])

    def test_closed_context_cycles_preserved(self):
        contexts=self.contexts();contexts['terms']['requires']=['scene']
        result=plan(self.units(n=1),contexts,self.limits())
        self.assertEqual(result['ready'][0]['payload']['contexts'],contexts)

    def test_sparse_cost_can_be_amortized_without_losing_unit_boundary(self):
        units=self.units();contexts=self.contexts()
        one=plan(units[:1],contexts,self.limits())['ready'][0]['audit']['input_cost']
        together=plan(units,contexts,self.limits())['ready'][0]['audit']['input_cost']/len(units)
        cap=int((one+together)/2)
        result=plan(units,contexts,self.limits(max_input_per_target=cap))
        self.assertEqual(len(result['ready']),1);self.assertEqual(result['held'],[])

    def test_warmup_schema_output_reserve_and_counter_kind_are_visible(self):
        meter=Meter('fixture-exact',lambda s:len(s),'test_only')
        job={'prompt':'a','warmup':'b'*100,'schema':{'type':'object'}}
        report=measure_job(job,1,self.limits(max_input=100),meter)
        self.assertFalse(report['allowed']);self.assertEqual(report['meter_kind'],'test_only')
        self.assertEqual(report['input_cost'],sum(report['cost_by_part'].values()))
        with self.assertRaises(ValueError):self.limits(max_input=110000)

    def test_duplicate_targets_are_not_silently_deduplicated(self):
        units=self.units(n=2);units[1]['targets']=copy.deepcopy(units[0]['targets'])
        with self.assertRaises(ValueError):plan(units,self.contexts(),self.limits())


if __name__=='__main__':unittest.main()
