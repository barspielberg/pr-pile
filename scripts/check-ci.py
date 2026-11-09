#!/usr/bin/env python3
"""Fail the stable required check unless every expected dependency succeeded."""

import json
import os
import sys


def successful(needs):
    return isinstance(needs, dict) and set(needs) == {"test", "package", "docs"} and all(
        isinstance(job, dict) and job.get("result") == "success" for job in needs.values()
    )


if __name__ == "__main__":
    try:
        passed = successful(json.loads(os.environ["CI_NEEDS"]))
    except (KeyError, ValueError):
        passed = False
    print("All CI dependencies succeeded" if passed else "CI dependency failed, was skipped/cancelled, or is missing")
    sys.exit(0 if passed else 1)
