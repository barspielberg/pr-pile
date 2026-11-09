import base64
import hashlib
import tempfile
from pathlib import Path
import runpy
import unittest


MODULE = runpy.run_path(str(Path(__file__).with_name("homebrew-formula.py")))
formula = MODULE["formula"]
update_payload = MODULE["update_payload"]
CHECKSUM = "a" * 64
PLATFORMS = (("darwin", "arm64"), ("darwin", "amd64"), ("linux", "arm64"), ("linux", "amd64"))


def manifest(tag):
    return "".join(f"{format(index + 10, 'x') * 64}  pile_{tag[1:]}_{system}_{arch}.tar.gz\n" for index, (system, arch) in enumerate(PLATFORMS))


def existing_formula(tag):
    # GitHub wraps base64 content across lines in the Contents API response.
    content = base64.b64encode(formula(tag, manifest(tag)).encode()).decode()
    return {"encoding": "base64", "content": content[:60] + "\n" + content[60:], "sha": "existing-sha"}


class HomebrewFormulaTests(unittest.TestCase):
    def test_each_platform_uses_its_archive_and_actual_checksum(self):
        rendered = formula("v0.1.0", manifest("v0.1.0"))
        for index, (system, arch) in enumerate(PLATFORMS):
            checksum = format(index + 10, "x") * 64
            brew_os = "macos" if system == "darwin" else "linux"
            brew_arch = "arm" if arch == "arm64" else "intel"
            block = rendered.split(f"  on_{brew_os} do\n", 1)[1].split("\n  end\n", 1)[0]
            self.assertIn(f'    on_{brew_arch} do\n      url "https://github.com/barspielberg/pr-pile/releases/download/v0.1.0/pile_0.1.0_{system}_{arch}.tar.gz"\n      sha256 "{checksum}"', block)
        self.assertIn('version "0.1.0"', rendered)
        self.assertNotIn('depends_on "go"', rendered)
        self.assertNotIn('"build"', rendered)
        self.assertIn('bin.install "pile"', rendered)
        self.assertIn('built ', rendered)

    def test_missing_duplicate_wrong_version_and_unsafe_manifest_entries(self):
        good = manifest("v0.1.0")
        for bad in [
            "\n".join(good.splitlines()[:-1]), good + good.splitlines()[0],
            good.replace("pile_0.1.0", "pile_0.2.0"),
            good.replace("pile_", "../pile_", 1), good + "\n",
            good.replace("a" * 64, "z" * 64),
            good.replace("a" * 64, "A" * 64),
        ]:
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                formula("v0.1.0", bad)

    def test_local_rc_candidate_preserves_version_and_verifies_every_download(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            # Exercise macOS-style temp-directory aliases on Linux CI too.
            alias = root / "downloads-link"
            alias.symlink_to(root, target_is_directory=True)
            lines = []
            for system, arch in PLATFORMS:
                path = root / f"pile_0.1.0-rc.1_{system}_{arch}.tar.gz"
                path.write_bytes(f"fixture {system} {arch}".encode())
                lines.append(f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}")
            sums = "\n".join(lines)
            rendered = formula("v0.1.0-rc.1", sums, alias)
            self.assertIn('class PileCandidate', rendered)
            self.assertIn('version "0.1.0-rc.1"', rendered)
            self.assertIn('bin.install "pile" => "pile-candidate"', rendered)
            self.assertNotIn(MODULE["RELEASE_URL"] + "v", rendered)
            for system, arch in PLATFORMS:
                self.assertIn((root / f"pile_0.1.0-rc.1_{system}_{arch}.tar.gz").as_uri(), rendered)
            with self.assertRaises(ValueError):
                update_payload("v0.1.0", rendered, {})
            path.write_bytes(b"tampered archive")
            with self.assertRaisesRegex(ValueError, "checksum mismatch"):
                formula("v0.1.0-rc.1", sums, root)
            path.unlink()
            with self.assertRaises(OSError):
                formula("v0.1.0-rc.1", sums, root)

    def test_previous_source_formula_can_upgrade_but_not_downgrade(self):
        legacy = '  url "https://github.com/barspielberg/pr-pile/archive/refs/tags/v0.2.0.tar.gz"'
        state = {"encoding": "base64", "content": base64.b64encode(legacy.encode()).decode(), "sha": "legacy-sha"}
        self.assertEqual(update_payload("v0.3.0", formula("v0.3.0", manifest("v0.3.0")), state)["sha"], "legacy-sha")
        with self.assertRaisesRegex(ValueError, "refusing to downgrade"):
            update_payload("v0.1.0", formula("v0.1.0", manifest("v0.1.0")), state)

    def test_stable_draft_candidate_is_local_only_too(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            lines = []
            for system, arch in PLATFORMS:
                path = root / f"pile_0.1.0_{system}_{arch}.tar.gz"
                path.write_bytes(b"stable draft fixture")
                lines.append(f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}")
            rendered = formula("v0.1.0", "\n".join(lines), root)
            self.assertIn('class PileCandidate', rendered)
            self.assertIn('version "0.1.0"', rendered)
            with self.assertRaisesRegex(ValueError, "URLs do not match"):
                update_payload("v0.1.0", rendered, {})

    def test_first_formula_creation(self):
        rendered = formula("v0.1.0", manifest("v0.1.0"))
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
                payload = update_payload(new, formula(new, manifest(new)), existing_formula(old))
                self.assertEqual(payload["sha"], "existing-sha")
                self.assertIn("/" + new + "/pile_" + new[1:] + "_", base64.b64decode(payload["content"]).decode())

    def test_older_versions_are_rejected(self):
        # The same publication guard handles both release events and manual retries.
        for old, new in [("v0.3.0", "v0.2.1"), ("v0.10.0", "v0.9.9"), ("v1.0.0", "v0.99.99")]:
            with self.subTest(old=old, new=new):
                with self.assertRaisesRegex(ValueError, "refusing to downgrade"):
                    update_payload(new, formula(new, manifest(new)), existing_formula(old))

    def test_invalid_release_inputs(self):
        for tag in [None, "", "0.1.0", "v01.2.3", "v0.1.0-rc.1", "v1.2.3+build", "v١.2.3", "v1.2.3\n", 'v1.2.3"; system "bad"']:
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                formula(tag, manifest("v0.1.0"))
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
            formula("v0.1.0", manifest("v0.1.0")).replace("/v0.1.0/", "/v0.1.0-rc.1/"),
            formula("v0.1.0", manifest("v0.1.0")) * 2,
        ]:
            states.append(dict(valid, content=base64.b64encode(text.encode()).decode()))
        for state in states:
            with self.subTest(state=state), self.assertRaises(ValueError):
                update_payload("v0.2.0", formula("v0.2.0", manifest("v0.2.0")), state)

    def test_payload_cannot_publish_a_different_version(self):
        with self.assertRaisesRegex(ValueError, "does not match"):
            update_payload("v0.2.0", formula("v0.1.0", manifest("v0.1.0")), {})


if __name__ == "__main__":
    unittest.main()
