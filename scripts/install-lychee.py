#!/usr/bin/env python3
"""Install a pinned, checksum-verified Lychee binary into the given directory."""

import argparse
import hashlib
import io
from pathlib import Path
import platform
import sys
import tarfile
import time
import urllib.request

VERSION = "lychee-v0.24.2"
ASSETS = {
    ("Linux", "x86_64"): ("x86_64-unknown-linux-musl", "73657a111819a30c47c08352896796f23d64e4eb2b3ed39b6d32149241566fc5"),
    ("Linux", "aarch64"): ("aarch64-unknown-linux-musl", "5d0b0e3aeab240f41920c633a6eaf97599be6eedda034b36e858ede7dba5e535"),
    ("Darwin", "arm64"): ("aarch64-apple-darwin", "c9d3740ea2d891854d37116c9fba840f37b6e7c89d330e7db84ac333631c4977"),
    ("Darwin", "x86_64"): ("x86_64-apple-darwin", "887503a9cff667d322b8d0892b40bf49976eb9507af8483220a3706cdad55978"),
}


def install(directory):
    target, checksum = ASSETS[(platform.system(), platform.machine())]
    url = f"https://github.com/lycheeverse/lychee/releases/download/{VERSION}/lychee-{target}.tar.gz"
    for attempt in range(3):
        try:
            with urllib.request.urlopen(url, timeout=30) as response:
                archive = response.read()
            if hashlib.sha256(archive).hexdigest() != checksum:
                raise ValueError("Lychee download SHA-256 mismatch")
            with tarfile.open(fileobj=io.BytesIO(archive)) as package:
                members = [m for m in package if Path(m.name).name == "lychee" and m.isfile()]
                if len(members) != 1:
                    raise ValueError("expected one Lychee executable")
                binary = package.extractfile(members[0]).read()
            directory.mkdir(parents=True, exist_ok=True)
            path = directory / "lychee"
            path.write_bytes(binary)
            path.chmod(0o755)
            print(f"Installed {VERSION} at {path} (SHA-256 verified)")
            return
        except (OSError, ValueError, tarfile.TarError) as error:
            if attempt == 2:
                raise RuntimeError(f"Lychee installation failed after 3 attempts: {error}") from error
            print(f"Lychee install attempt {attempt + 1} failed: {error}; retrying", file=sys.stderr)
            time.sleep(attempt + 1)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    try:
        install(args.directory)
    except (KeyError, RuntimeError) as error:
        print(f"INSTALL ERROR: {error}", file=sys.stderr)
        sys.exit(2)
