import base64
from pathlib import Path
import runpy
import unittest


MODULE = runpy.run_path(str(Path(__file__).with_name("homebrew-formula.py")))
formula = MODULE["formula"]
update_payload = MODULE["update_payload"]
CHECKSUM = "a" * 64


def existing_formula(tag):
    # GitHub wraps base64 content across lines in the Contents API response.
    content = base64.b64encode(formula(tag, CHECKSUM).encode()).decode()
    return {"encoding": "base64", "content": content[:60] + "\n" + content[60:], "sha": "existing-sha"}


class HomebrewFormulaTests(unittest.TestCase):
    def test_first_formula_creation(self):
        rendered = formula("v0.1.0", CHECKSUM)
        payload = update_payload("v0.1.0", rendered, {})
        self.assertNotIn("sha", payload)
        self.assertEqual(base64.b64decode(payload["content"]).decode(), rendered)
        self.assertIn('depends_on "gh"', rendered)
        self.assertIn('sha256 "' + CHECKSUM + '"', rendered)

    def test_same_version_retry_and_forward_updates(self):
        for old, new in [
            ("v0.1.0", "v0.1.0"),
            ("v0.1.0", "v0.2.0"),
            ("v0.9.9", "v0.10.0"),
            ("v0.99.99", "v1.0.0"),
        ]:
            with self.subTest(old=old, new=new):
                payload = update_payload(new, formula(new, CHECKSUM), existing_formula(old))
                self.assertEqual(payload["sha"], "existing-sha")
                self.assertIn(new + ".tar.gz", base64.b64decode(payload["content"]).decode())

    def test_older_versions_are_rejected(self):
        # The same publication guard handles both release events and manual retries.
        for old, new in [("v0.3.0", "v0.2.1"), ("v0.10.0", "v0.9.9"), ("v1.0.0", "v0.99.99")]:
            with self.subTest(old=old, new=new):
                with self.assertRaisesRegex(ValueError, "refusing to downgrade"):
                    update_payload(new, formula(new, CHECKSUM), existing_formula(old))

    def test_invalid_release_inputs(self):
        for tag in [None, "", "0.1.0", "v01.2.3", "v0.1.0-rc.1", "v1.2.3+build", "v١.2.3", "v1.2.3\n", 'v1.2.3"; system "bad"']:
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                formula(tag, CHECKSUM)
        for checksum in [None, "", "a" * 63, "A" * 64, "g" * 64, "a" * 64 + "\n"]:
            with self.subTest(checksum=checksum), self.assertRaises(ValueError):
                formula("v0.1.0", checksum)

    def test_invalid_existing_state_fails_closed(self):
        valid = existing_formula("v0.1.0")
        states = [None, [], {"message": "Forbidden"}, {"sha": "sha"}]
        for field, value in [("sha", ""), ("encoding", "none"), ("content", "invalid base64!")]:
            states.append(dict(valid, **{field: value}))
        for text in [
            "not a formula",
            '  url "https://example.com/v0.1.0.tar.gz"',
            formula("v0.1.0", CHECKSUM).replace("v0.1.0.tar.gz", "v0.1.0-rc.1.tar.gz"),
            formula("v0.1.0", CHECKSUM) * 2,
        ]:
            states.append(dict(valid, content=base64.b64encode(text.encode()).decode()))
        for state in states:
            with self.subTest(state=state), self.assertRaises(ValueError):
                update_payload("v0.2.0", formula("v0.2.0", CHECKSUM), state)

    def test_payload_cannot_publish_a_different_version(self):
        with self.assertRaisesRegex(ValueError, "does not match"):
            update_payload("v0.2.0", formula("v0.1.0", CHECKSUM), {})


if __name__ == "__main__":
    unittest.main()
