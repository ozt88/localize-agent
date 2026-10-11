import unittest
from .grouping import partition_co_read


class GroupingTests(unittest.TestCase):
    def test_dp_beats_largest_first_without_splitting_scopes(self):
        costs={(0,):8,(1,):8,(2,):8,(3,):8,(0,1):10,(2,3):10,(0,1,2):19}
        result=partition_co_read(list(range(4)),lambda _:0,
            lambda group:{'allowed':tuple(group) in costs,'cost':costs.get(tuple(group),0)},3)
        self.assertEqual([g['indices'] for g in result['ready']],[[0,1],[2,3]])
        self.assertFalse(result['held'])

    def test_incompatible_families_and_infeasible_scopes(self):
        result=partition_co_read([0,1,2,3],lambda x:x%2,
            lambda group:{'allowed':3 not in group,'cost':10+len(group)})
        self.assertEqual(result['held'][0]['index'],3)
        self.assertEqual(sorted(i for g in result['ready'] for i in g['indices']),[0,1,2])
        self.assertTrue(all(len({i%2 for i in g['indices']})==1 for g in result['ready']))


if __name__=='__main__':unittest.main()
