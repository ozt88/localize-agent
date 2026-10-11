import copy
import random
import unittest
from .core import canonical, pack, optimize
from .compact_codec import compact, unpack_compact


class CompactCodecTests(unittest.TestCase):
    def test_nested_literals_and_control_strings_roundtrip(self):
        value={'@s0':'@d12','literal':['@','@s3','@nonsense','{nc|A|B}',[-1,False,None]],
               '$r':[False,0,0.0,-1], 'array':[-2,[-1]], 'branch':{'condition':False,'next':'end'}}
        packet=compact(pack(value))
        self.assertEqual(canonical(unpack_compact(packet)),canonical(value))

    def test_generated_roundtrips(self):
        rng=random.Random(13)
        def gen(n):
            if not n:return rng.choice([None,False,True,-4,0,0.0,'@s0','한글','{x}'])
            return rng.choice([lambda:[gen(n-1) for _ in range(4)],lambda:{str(i):gen(n-1) for i in range(3)}])()
        for _ in range(50):
            v=gen(3);self.assertEqual(canonical(unpack_compact(compact(pack(v)))),canonical(v))

    def test_malformed_cycle_and_reference_rejected(self):
        packet=compact(pack({'x':'hello'}));packet['shared']=['@d0'];packet['root']='@d0'
        with self.assertRaises(ValueError):unpack_compact(packet)
        for root in ('@s-1','@s999','@sTrue',{'array':[True]},[-999],{'literal':'plain'}):
            packet=compact(pack({'x':'hello'}));packet['root']=root
            with self.assertRaises(ValueError):unpack_compact(packet)

    def test_codec_is_optional_and_target_contract_remains_literal(self):
        v={'items':[{'id':str(i),'value':'same text '*30,'conditions':[False,i]} for i in range(40)]}
        result=optimize(v,'Translate',targets=[{'id':'real-id','source':'{name}'}])
        self.assertIn('record-table-v2',result['alternatives'])
        self.assertLessEqual(result['prompt_cost'],result['alternatives']['record-table-v1'])
        self.assertIn('"id":"real-id"',result['prompt'])


if __name__=='__main__':unittest.main()
