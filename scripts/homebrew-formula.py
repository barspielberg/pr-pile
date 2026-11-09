#!/usr/bin/env python3
"""Render binary Homebrew formulae from a GoReleaser checksum manifest."""

import argparse
import base64
import hashlib
from pathlib import Path
import re

PLATFORMS = (("darwin", "arm64"), ("darwin", "amd64"), ("linux", "arm64"), ("linux", "amd64"))
RELEASE_URL = "https://github.com/barspielberg/pr-pile/releases/download/"
STABLE = r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"


def release_version(tag, candidate=False):
    pattern = STABLE + (r"(?:-rc\.[1-9][0-9]*)?" if candidate else "")
    if not isinstance(tag, str) or not re.fullmatch(pattern, tag):
        raise ValueError("expected vMAJOR.MINOR.PATCH" + (" or an -rc.N candidate" if candidate else " (stable release only)"))
    return tag[1:]


def stable_version(tag):
    return tuple(int(part) for part in release_version(tag).split("."))


def archive_name(tag, system, arch, candidate=False):
    return f"pile_{release_version(tag, candidate)}_{system}_{arch}.tar.gz"


def checksums(tag, manifest, candidate=False):
    if not isinstance(manifest, str):
        raise ValueError("expected checksum manifest text")
    expected = {archive_name(tag, os, arch, candidate) for os, arch in PLATFORMS}
    found = {}
    for line in manifest.splitlines():
        match = re.fullmatch(r"([a-f0-9]{64}) [ *]([A-Za-z0-9._-]+)", line)
        if not match or match[2] not in expected or match[2] in found:
            raise ValueError("malformed, duplicate, or unexpected checksum manifest entry")
        found[match[2]] = match[1]
    if found.keys() != expected:
        raise ValueError("checksum manifest must contain all four matching release archives")
    return found


def formula_version(text):
    # Accept the previous source formula so existing taps can upgrade safely.
    urls = re.findall(r'^[ \t]*url "([^"\n]+)"[ \t]*$', text, re.MULTILINE)
    legacy = "https://github.com/barspielberg/pr-pile/archive/refs/tags/"
    if len(urls) == 1 and urls[0].startswith(legacy) and urls[0].endswith(".tar.gz"):
        return stable_version(urls[0][len(legacy):-len(".tar.gz")])
    versions = re.findall(r'^[ \t]*version "([^"\n]+)"[ \t]*$', text, re.MULTILINE)
    if len(versions) != 1:
        raise ValueError("cannot determine existing formula version")
    tag = "v" + versions[0]
    version = stable_version(tag)
    expected = {RELEASE_URL + tag + "/" + archive_name(tag, os, arch) for os, arch in PLATFORMS}
    if len(urls) != 4 or set(urls) != expected:
        raise ValueError("formula URLs do not match its stable release version")
    return version


def update_payload(tag, new_formula, existing):
    """Refuse downgrades; bind the version check to the Contents API file SHA."""
    target = stable_version(tag)
    if formula_version(new_formula) != target:
        raise ValueError("generated formula does not match the requested tag")
    if not isinstance(existing, dict):
        raise ValueError("expected existing formula response or {} after a confirmed 404")
    payload = {"message": "Update pile to " + tag, "content": base64.b64encode(new_formula.encode()).decode()}
    if existing:
        if existing.get("encoding") != "base64" or not isinstance(existing.get("content"), str) or not isinstance(existing.get("sha"), str) or not existing["sha"]:
            raise ValueError("existing formula response is missing content, encoding, or SHA")
        current = formula_version(base64.b64decode("".join(existing["content"].split()), validate=True).decode())
        if target < current:
            raise ValueError(f"refusing to downgrade pile from v{'.'.join(map(str, current))} to {tag}")
        payload["sha"] = existing["sha"]
    return payload


def formula(tag, manifest, candidate_dir=None):
    candidate = candidate_dir is not None
    version = release_version(tag, candidate)
    sums = checksums(tag, manifest, candidate)
    urls = {}
    for system, arch in PLATFORMS:
        name = archive_name(tag, system, arch, candidate)
        if candidate:
            path = Path(candidate_dir).resolve() / name
            if hashlib.sha256(path.read_bytes()).hexdigest() != sums[name]:
                raise ValueError(f"candidate archive checksum mismatch: {name}")
            urls[system, arch] = path.as_uri()
        else:
            urls[system, arch] = RELEASE_URL + tag + "/" + name
    name = "pile-candidate" if candidate else "pile"
    lines = [
        f'class {"PileCandidate" if candidate else "Pile"} < Formula',
        '  desc "Terminal PR board for GitHub"',
        '  homepage "https://github.com/barspielberg/pr-pile"',
        f'  version "{version}"',
        '  license all_of: ["MIT", "CC-BY-4.0"]',
        '  depends_on "gh"',
        '',
    ]
    for system, brew_os in [("darwin", "macos"), ("linux", "linux")]:
        lines.append(f"  on_{brew_os} do")
        for arch, brew_arch in [("arm64", "arm"), ("amd64", "intel")]:
            lines.extend([
                f"    on_{brew_arch} do",
                f'      url "{urls[system, arch]}"',
                f'      sha256 "{sums[archive_name(tag, system, arch, candidate)]}"',
                "    end",
            ])
        lines.extend(["  end", ""])
    lines.extend([
        "  def install",
        f'    bin.install "pile" => "{name}"',
        "  end", "",
        "  test do",
        f'    output = shell_output("#{{bin}}/{name} --version")',
        f'    assert_match "pile {version} (commit ", output',
        '    assert_match "built ", output',
        f'    assert_match "Usage: pile", shell_output("#{{bin}}/{name} --help")',
        "  end", "end", "",
    ])
    return "\n".join(lines)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag")
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--candidate-dir", type=Path, help="verify downloaded draft/RC archives and render a local-only pile-candidate formula")
    args = parser.parse_args()
    try:
        print(formula(args.tag, args.manifest.read_text(), args.candidate_dir), end="")
    except (OSError, ValueError) as error:
        parser.error(str(error))
