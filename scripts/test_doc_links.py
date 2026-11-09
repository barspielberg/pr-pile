import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import unittest

CHECKER = Path(__file__).with_name("check-doc-links.py").resolve()


class DocLinkTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.lychee = os.environ.get("LYCHEE")
        if not cls.lychee:
            if os.environ.get("PILE_REQUIRE_LYCHEE") == "1":
                raise RuntimeError("LYCHEE must name the installed pinned executable")
            raise unittest.SkipTest("set LYCHEE to run real link-checking fixtures")

    def check_fixture(self, href, expected):
        with tempfile.TemporaryDirectory() as directory:
            site = Path(directory)
            (site / "guide").mkdir()
            (site / "guide/index.html").write_text('<h1 id="target">Target</h1>')
            (site / "plain.html").write_text('<h1 id="target">Target</h1>')
            (site / "empty").mkdir()
            (site / "index.html").write_text(f'<h1 id="home">Home</h1><a href="{href}">fixture</a><a href="https://external.invalid/">offline</a>')
            result = subprocess.run([sys.executable, str(CHECKER), str(site), "--lychee", self.lychee], capture_output=True, text=True, timeout=30)
            self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
            return result

    def test_valid_relative_root_self_hosted_and_fragment_links(self):
        for href in ["#home", "guide/#target", "/pr-pile/guide/#target", "/pr-pile/plain#target", "https://barspielberg.github.io/pr-pile/guide/#target"]:
            with self.subTest(href=href):
                self.check_fixture(href, 0)

    def test_missing_pages_fail(self):
        for href in ["/pr-pile/missing/", "/pr-pile/empty/", "/guide/", "https://barspielberg.github.io/pr-pile/missing/"]:
            with self.subTest(href=href):
                self.check_fixture(href, 1)

    def test_missing_anchors_fail(self):
        for href in ["#missing", "guide/#missing", "/pr-pile/guide/#missing", "https://barspielberg.github.io/pr-pile/guide/#missing"]:
            with self.subTest(href=href):
                self.check_fixture(href, 1)

    def test_installation_failure_is_not_reported_as_a_broken_link(self):
        original = self.lychee
        try:
            self.lychee = "/nonexistent/lychee"
            result = self.check_fixture("#home", 2)
            self.assertIn("INSTALL ERROR", result.stderr)
            self.assertNotIn("LINK CHECK FAILED", result.stderr)
        finally:
            self.lychee = original

    def test_external_report_retries_without_using_internal_file_checks(self):
        counts = {"requests": 0}

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                counts["requests"] += 1
                self.send_response(500 if counts["requests"] == 1 else 200)
                self.end_headers()
                self.wfile.write(b"fixture")

            def log_message(self, *args):
                pass

        with tempfile.TemporaryDirectory() as directory, ThreadingHTTPServer(("127.0.0.1", 0), Handler) as server:
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                root = Path(directory)
                (root / "index.html").write_text(f'<a href="http://127.0.0.1:{server.server_port}/retry">retry</a><a href="/missing/">internal ignored in report</a>')
                report = root / "report.md"
                result = subprocess.run([sys.executable, str(CHECKER), str(root), "--lychee", self.lychee, "--external", "--report", str(report)], capture_output=True, text=True, timeout=40)
                details = report.read_text() if report.exists() else "no report"
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr + details + str(counts))
                self.assertGreaterEqual(counts["requests"], 2)
                self.assertTrue(report.is_file())
            finally:
                server.shutdown()
                thread.join(timeout=2)
