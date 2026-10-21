import io
import os
import tempfile
import unittest
from contextlib import redirect_stdout
from pathlib import Path

from todo.cli import main


class CountTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        os.environ["TODO_FILE"] = str(Path(self.dir.name) / "todos.json")

    def tearDown(self):
        del os.environ["TODO_FILE"]
        self.dir.cleanup()

    def count(self):
        out = io.StringIO()
        with redirect_stdout(out):
            code = main(["count"])
        return code, out.getvalue()

    def test_empty_list_counts_zero(self):
        self.assertEqual(self.count(), (0, "0\n"))

    def test_counts_every_item(self):
        with redirect_stdout(io.StringIO()):
            main(["add", "buy milk"])
            main(["add", "walk dog"])
        self.assertEqual(self.count(), (0, "2\n"))
