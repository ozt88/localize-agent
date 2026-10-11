import copy
import random
import unittest
from .core import canonical
from .scopes import difference, apply_difference, bind_scopes, restore_scopes, split_candidate


class ScopeTests(unittest.TestCase):
    def test_array_reordering_type_changes_and_literal_paths(self):
        a={'/x~':[{'id':1,'text':'{name}'},{'id':2,'text':'B'},{'id':3,'text':'C'}],'flag':False}
        b={'/x~':[{'id':3,'text':'C'},{'id':1,'text':'{name}'},{'id':4,'text':'D'}],'flag':0}
        self.assertEqual(canonical(apply_difference(a,difference(a,b))),canonical(b))

    def test_generated_nested_array_mutations(self):
        rng=random.Random(42)
        for _ in range(80):
            a=[{'id':i,'body':[i,'한글 {x}',False]} for i in range(rng.randrange(12))]
            b=copy.deepcopy(a);rng.shuffle(b)
            if b:b[rng.randrange(len(b))]['body']=[None,0.0,True]
            b.insert(rng.randrange(len(b)+1),{'new':[3,4]})
            self.assertEqual(canonical(apply_difference(a,difference(a,b))),canonical(b))

    def test_scope_binding_tamper_and_unapproved_projection(self):
        base={'scene':['A','B'],'targets':['a','b']}
        originals=[{'scope':k,'targets':[k],'request':{**base,'targets':[k]}} for k in ('a','b')]
        manifest=bind_scopes(base,originals,'scene:one')
        self.assertEqual(restore_scopes(base,manifest),{x['scope']:x['request'] for x in originals})
        self.assertFalse(split_candidate(base,manifest,{'a':'가','b':'나'})['a']['approved'])
        with self.assertRaises(ValueError):split_candidate(base,manifest,{'a':'가'})
        changed=copy.deepcopy(base);changed['scene'].append('C')
        with self.assertRaises(ValueError):restore_scopes(changed,manifest)
        manifest['target_owners']['a']='b'
        with self.assertRaises(ValueError):restore_scopes(base,manifest)


if __name__=='__main__':unittest.main()
