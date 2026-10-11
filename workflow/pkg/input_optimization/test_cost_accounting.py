import unittest
from .cost_accounting import aggregate


class CostAccountingTests(unittest.TestCase):
    def test_interrupted_usage_is_retained_without_claiming_finality(self):
        result=aggregate([self.snapshot('a',100)],['a'],uncertain_sessions=['a'])
        self.assertEqual(result['tokens']['total'],110)
        self.assertFalse(result['complete'])
        self.assertEqual(result['uncertain_sessions'],['a'])
    def snapshot(self,session,n):
        return {'session_id':session,'input':n,'cached_input':n//2,'output':10,'reasoning_output':4}

    def test_cumulative_snapshots_are_not_summed(self):
        result=aggregate([self.snapshot('a',100),self.snapshot('a',120),self.snapshot('b',50)],['a','b'])
        self.assertEqual(result['tokens']['total'],190)
        self.assertEqual(result['tokens']['uncached_input'],85)
        self.assertTrue(result['complete'])

    def test_missing_and_regressing_usage_not_treated_as_zero(self):
        result=aggregate([self.snapshot('a',100),self.snapshot('a',80)],['a','b'])
        self.assertFalse(result['complete']);self.assertEqual(result['missing_sessions'],['b'])
        self.assertEqual(result['regressed_sessions'],['a'])
        with self.assertRaises(ValueError):aggregate([{**self.snapshot('a',10),'cached_input':11}])
