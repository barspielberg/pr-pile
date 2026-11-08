#!/usr/bin/env python3
"""Render a source-building tap formula from a release tag and source checksum."""

import argparse
import base64
import re


def stable_version(tag):
    if not isinstance(tag, str) or not re.fullmatch(
        r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", tag
    ):
        raise ValueError("expected a stable release tag such as v0.1.0")
    return tuple(int(part) for part in tag[1:].split("."))


def formula_version(text):
    urls = re.findall(r'^[ \t]*url "([^"\n]+)"[ \t]*$', text, re.MULTILINE)
    prefix = "https://github.com/barspielberg/pr-pile/archive/refs/tags/"
    if len(urls) != 1 or not urls[0].startswith(prefix) or not urls[0].endswith(".tar.gz"):
        raise ValueError("cannot determine pile version from formula source URL")
    tag = urls[0][len(prefix) : -len(".tar.gz")]
    return stable_version(tag)


def update_payload(tag, new_formula, existing):
    """Build the Contents API payload, refusing downgrades or unreadable state.

    existing is the Contents API response, or {} only after a confirmed 404.
    Its SHA binds the version check to the file that the API will replace.
    """
    target = stable_version(tag)
    if formula_version(new_formula) != target:
        raise ValueError("generated formula does not match the requested tag")
    if not isinstance(existing, dict):
        raise ValueError("expected an existing formula response or {} for a missing formula")
    payload = {
        "message": "Update pile to " + tag,
        "content": base64.b64encode(new_formula.encode()).decode(),
    }
    if existing:
        if (
            existing.get("encoding") != "base64"
            or not isinstance(existing.get("content"), str)
            or not isinstance(existing.get("sha"), str)
            or not existing["sha"]
        ):
            raise ValueError("existing formula response is missing content, encoding, or SHA")
        old_formula = base64.b64decode(
            "".join(existing["content"].split()), validate=True
        ).decode()
        current = formula_version(old_formula)
        if target < current:
            old_tag = "v" + ".".join(map(str, current))
            raise ValueError(f"refusing to downgrade pile from {old_tag} to {tag}")
        payload["sha"] = existing["sha"]
    return payload


def formula(tag, checksum):
    stable_version(tag)
    if not isinstance(checksum, str) or not re.fullmatch(r"[a-f0-9]{64}", checksum):
        raise ValueError("expected a lowercase SHA-256 checksum")
    return f'''class Pile < Formula
  desc "Terminal PR board for GitHub"
  homepage "https://github.com/barspielberg/pr-pile"
  url "https://github.com/barspielberg/pr-pile/archive/refs/tags/{tag}.tar.gz"
  sha256 "{checksum}"
  license all_of: ["MIT", "CC-BY-4.0"]

  depends_on "go" => :build
  depends_on "gh"

  def install
    ENV["CGO_ENABLED"] = "0"
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=#{{version}}"), "./cmd/pile"
  end

  test do
    assert_match "pile #{{version}}", shell_output("#{{bin}}/pile --version")
    assert_match "Usage: pile", shell_output("#{{bin}}/pile --help")
  end
end
'''


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag")
    parser.add_argument("checksum")
    args = parser.parse_args()
    try:
        print(formula(args.tag, args.checksum), end="")
    except ValueError as error:
        parser.error(str(error))
