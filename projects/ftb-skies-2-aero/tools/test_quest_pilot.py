import unittest
from run_quest_pilot import failures


class ProtectedText(unittest.TestCase):
    def test_command_arguments_cannot_change(self):
        source = '&a/ftbteams party invite <username>&f'
        self.assertEqual([], failures(source, source + '를 입력하세요.'))
        self.assertIn('command_changed', failures(source, source.replace('invite', 'leave')))
        self.assertTrue(failures(source, source.replace('<username>', '<사용자>')))

    def test_added_command_and_changed_colors_block(self):
        self.assertIn('command_name_changed', failures('Hello', '안녕 /kill'))
        self.assertIn('color_control_changed', failures('&a/back&r', '&r/back&a'))
        self.assertTrue(failures('Hello', '{click:run_command}'))

    def test_natural_word_order_can_move_color_spans(self):
        self.assertEqual([], failures('&fWelcome to &6FTB Skies 2: Aero&f!',
                                      '&f&6FTB Skies 2: Aero&f에 오신 것을 환영해요!'))


if __name__ == '__main__':
    unittest.main()
