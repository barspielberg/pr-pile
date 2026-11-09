import hashlib
import io
from pathlib import Path
import runpy
import tarfile
import tempfile
import unittest
from unittest.mock import patch

MODULE = runpy.run_path(str(Path(__file__).with_name("install-lychee.py")))
install = MODULE["install"]


class InstallerTests(unittest.TestCase):
    def setUp(self):
        stream = io.BytesIO()
        with tarfile.open(fileobj=stream, mode="w:gz") as archive:
            content = b"fixture executable"
            member = tarfile.TarInfo("lychee")
            member.size = len(content)
            archive.addfile(member, io.BytesIO(content))
        self.archive = stream.getvalue()
        self.assets = patch.dict(install.__globals__["ASSETS"], {("Linux", "x86_64"): ("test", hashlib.sha256(self.archive).hexdigest())})
        self.assets.start()
        self.addCleanup(self.assets.stop)

    @patch("platform.machine", return_value="x86_64")
    @patch("platform.system", return_value="Linux")
    @patch("time.sleep")
    def test_verified_archive_installs_after_transient_failure(self, sleep, system, machine):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with patch("urllib.request.urlopen", side_effect=[OSError("transient"), io.BytesIO(self.archive)]) as download:
                install(root)
            self.assertEqual(download.call_count, 2)
            self.assertEqual((root / "lychee").read_bytes(), b"fixture executable")
            self.assertTrue((root / "lychee").stat().st_mode & 0o111)

    @patch("platform.machine", return_value="x86_64")
    @patch("platform.system", return_value="Linux")
    @patch("time.sleep")
    def test_checksum_mismatch_never_installs_and_retries_are_bounded(self, sleep, system, machine):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with patch("urllib.request.urlopen", side_effect=lambda *args, **kwargs: io.BytesIO(b"corrupt")) as download:
                with self.assertRaisesRegex(RuntimeError, "after 3 attempts.*SHA-256 mismatch"):
                    install(root)
            self.assertEqual(download.call_count, 3)
            self.assertEqual(sleep.call_count, 2)
            self.assertFalse((root / "lychee").exists())
