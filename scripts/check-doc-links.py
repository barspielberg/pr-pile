#!/usr/bin/env python3
"""Check built Astro HTML, preserving the deployed /pr-pile URL namespace."""

import argparse
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import time

SITE = "https://barspielberg.github.io/pr-pile"


def check(dist, lychee, external=False, report=None):
    dist = Path(dist).resolve()
    if not dist.is_dir() or not (dist / "index.html").is_file():
        print("BUILD ERROR: expected built HTML including index.html", file=sys.stderr)
        return 2
    try:
        version = subprocess.run([str(lychee), "--version"], check=True, capture_output=True, text=True)
        if version.stdout.strip() != "lychee 0.24.2":
            raise ValueError("expected pinned lychee 0.24.2")
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"INSTALL ERROR: {error}; run scripts/install-lychee.py", file=sys.stderr)
        return 2
    with tempfile.TemporaryDirectory(prefix="pile-links-") as temporary:
        root = Path(temporary)
        site = root / "pr-pile"
        shutil.copytree(dist, site)
        html = sorted(site.rglob("*.html"))
        if not external:
            # Check self-hosted absolute links locally too; --offline excludes
            # other hosts without letting network failures affect PRs.
            for path in html:
                path.write_text(re.sub(re.escape(SITE) + r"(?=[/\"'#?]|$)", "/pr-pile", path.read_text()))
        inputs = root / "inputs.txt"
        inputs.write_text("\n".join(map(str, html)) + "\n")
        command = [str(lychee), "--no-progress", "--no-ignore", "--files-from", str(inputs), "--root-dir", str(root)]
        if external:
            command += ["--scheme", "https", "--scheme", "http", "--exclude", r"^https://barspielberg\.github\.io/pr-pile(?:/|$)",
                        "--max-retries", "0", "--timeout", "20", "--format", "markdown"]
            if report:
                command += ["--output", str(Path(report).resolve())]
        else:
            command += ["--offline", "--include-fragments=anchor-only", "--index-files", "index.html", "--fallback-extensions", "html"]
        # Lychee does not retry every HTTP failure status. Retry the external
        # pass itself so transient 5xx responses also get a second chance.
        attempts = 3 if external else 1
        for attempt in range(attempts):
            result = subprocess.run(command, timeout=180)
            if result.returncode == 0:
                break
            if attempt + 1 < attempts:
                print(f"External report attempt {attempt + 1} failed; retrying", file=sys.stderr)
                time.sleep(2)
        if result.returncode:
            print("LINK CHECK FAILED: see Lychee diagnostics above", file=sys.stderr)
            return 1
        print(f"Checked {len(html)} built HTML pages ({'external report' if external else 'internal pages and anchors'})")
        return 0


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("dist", type=Path)
    parser.add_argument("--lychee", type=Path, required=True)
    parser.add_argument("--external", action="store_true")
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    sys.exit(check(args.dist, args.lychee.resolve(), args.external, args.report))
