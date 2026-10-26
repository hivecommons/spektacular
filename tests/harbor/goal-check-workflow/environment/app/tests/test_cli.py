import io
import os
import tempfile
import unittest
from contextlib import redirect_stdout
from pathlib import Path

from todo.cli import main


class CliTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        os.environ["TODO_FILE"] = str(Path(self.dir.name) / "todos.json")

    def tearDown(self):
        del os.environ["TODO_FILE"]
        self.dir.cleanup()

    def run_cli(self, *args):
        out = io.StringIO()
        with redirect_stdout(out):
            code = main(list(args))
        return code, out.getvalue()

    def test_add_then_list(self):
        self.assertEqual(self.run_cli("add", "buy milk"), (0, "Added 1\n"))
        self.assertEqual(self.run_cli("list"), (0, "1. buy milk\n"))
