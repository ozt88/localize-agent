import unittest
from .minecraft_text import validate, technical_only


class Contract(unittest.TestCase):
    def test_component_localized_text_only(self):
        source = '["Open ", {"text":"Guide", "clickEvent":{"action":"run_command","value":"/guide open a:b"},"hoverEvent":{"action":"show_text","contents":["Open guide"]}}]'
        target = source.replace('Open ', '열기 ').replace('Guide', '안내서').replace('Open guide', '안내서 열기')
        self.assertEqual(validate(source, target), [])
        self.assertEqual(validate(source, target.replace('/guide', '/kill')), ['component_structure_changed'])
        self.assertTrue(validate(source, target.replace('run_command', 'open_url')))

    def test_markers_format_and_resource(self):
        self.assertTrue(technical_only('{image:minecraft:item/emerald width:20}'))
        self.assertFalse(technical_only('Look {image:minecraft:item/emerald width:20}'))
        self.assertTrue(validate('Need %s at %d', '필요 %d 위치 %s'))
        self.assertTrue(validate('&aName&r {@pagebreak}', '&a이름 {@pagebreak}'))
        self.assertTrue(validate('minecraft:stone', 'minecraft:dirt'))
        self.assertEqual(validate('50% chance', '확률 50%'), [])

    def test_keybind_and_shape_are_immutable(self):
        self.assertTrue(validate('{"keybind":"key.jump"}', '{"keybind":"점프"}'))
        self.assertTrue(validate('["A","B"]', '["가"]'))

    def test_component_whitespace_does_not_change_type(self):
        self.assertEqual(validate('[ "Open ", { "text": "Guide" } ]',
                                  '["열기 ",{"text":"안내서"}]'), [])
        self.assertTrue(validate('[ "Open ", { "text": "Guide" } ]',
                                 '["열기 ",{"text":"안내서"},"추가"]'))
        self.assertEqual(validate('{ "text": "Guide" }', '{"text":"안내서"}'), [])

    def test_sentence_punctuation_is_not_a_resource_or_coordinate(self):
        self.assertEqual(validate('between y:25 and y:50.', 'y:25에서 y:50 사이예요.'), [])
        self.assertEqual(validate('Use minecraft:stone.', 'minecraft:stone을 쓰세요.'), [])
        self.assertTrue(validate('Use minecraft:stone.', 'minecraft:dirt을 쓰세요.'))
        self.assertTrue(validate('between y:25 and y:50.', 'y:25에서 y:51 사이예요.'))


if __name__ == '__main__': unittest.main()
