from pathlib import Path
import runpy
import unittest

successful = runpy.run_path(str(Path(__file__).with_name("check-ci.py")))["successful"]


class AggregateTests(unittest.TestCase):
    def test_all_succeed(self):
        self.assertTrue(successful({name: {"result": "success"} for name in ["test", "package", "docs"]}))

    def test_no_failed_skipped_cancelled_or_missing_dependency_can_pass(self):
        for name in ["test", "package", "docs"]:
            for result in ["failure", "skipped", "cancelled", "", None]:
                needs = {job: {"result": "success"} for job in ["test", "package", "docs"]}
                needs[name]["result"] = result
                with self.subTest(name=name, result=result):
                    self.assertFalse(successful(needs))
        self.assertFalse(successful({}))
        self.assertFalse(successful({"test": {"result": "success"}, "package": {"result": "success"}}))
        self.assertFalse(successful(None))
