import unittest
from .ftb_snbt import Document

class SnbtTest(unittest.TestCase):
    def test_lossless_selected_text_edit(self):
        source = '{\n id: "AB123"\n text: ["Hello" "" "{@pagebreak}"]\n n: 3L\n}'
        doc = Document(source)
        self.assertEqual(doc.replace({}), source)
        result = doc.replace({('text', 0): '안녕 "모험가"'})
        self.assertEqual(Document(result).value['text'], ['안녕 "모험가"', '', '{@pagebreak}'])
        self.assertIn('id: "AB123"', result)
        self.assertIn('n: 3L', result)

    def test_typed_arrays_stay_opaque_and_unchanged(self):
        source = '{uuid: [I; 1, -2, 3, 4] text: "Hi"}'
        doc = Document(source)
        self.assertEqual(doc.value['uuid']['values'], ['1','-2','3','4'])
        self.assertIn('[I; 1, -2, 3, 4]', doc.replace({('text',): '안녕'}))

    def test_rejects_duplicate_and_unsupported_content(self):
        for source in ['{a: "x" a: "y"}', '{a: "x"', '{a: "x"} extra']:
            with self.subTest(source=source), self.assertRaises(ValueError): Document(source)
        with self.assertRaises(ValueError): Document('{a: 1}').replace({('a',): '잘못'})

if __name__ == '__main__': unittest.main()
