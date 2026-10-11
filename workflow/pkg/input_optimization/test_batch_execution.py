import tempfile
from pathlib import Path
import unittest
from unittest.mock import patch
from .batch_execution import call, write


class ResumeSafety(unittest.TestCase):
    def test_existing_uncertain_call_is_not_reissued(self):
        with tempfile.TemporaryDirectory() as t:
            path=Path(t);(path/'translate').mkdir()
            with patch('subprocess.run') as dispatch:
                with self.assertRaisesRegex(ValueError,'Uncertain/failed'):
                    call(path,'translate',{'prompt':'test'},'runner','codex',path)
                dispatch.assert_not_called()

    def test_changed_frozen_job_cannot_overwrite_or_dispatch(self):
        with tempfile.TemporaryDirectory() as t:
            path=Path(t);write(path/'translate.job.json',{'prompt':'original'})
            with patch('subprocess.run') as dispatch:
                with self.assertRaisesRegex(ValueError,'Frozen job changed'):
                    call(path,'translate',{'prompt':'changed'},'runner','codex',path)
                dispatch.assert_not_called()
            self.assertIn('original',(path/'translate.job.json').read_text())


if __name__=='__main__':unittest.main()
