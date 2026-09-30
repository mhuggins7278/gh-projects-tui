#!/usr/bin/env python3
"""Run regression tests, or the original overlay on a pre-fix checkout."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile


def main():
    evidence = Path(__file__).resolve().parent
    root = evidence.parent.parent
    replacements = {}
    for package in ("ui", "github", "config"):
        if (root / "internal" / package / "review_regression_test.go").exists():
            continue
        virtual = root / "internal" / package / "review_repro_test.go"
        if virtual.exists():
            raise SystemExit(f"Refusing to shadow an existing file: {virtual}")
        replacements[str(virtual)] = str(evidence / f"{package}_test.go.txt")

    with tempfile.TemporaryDirectory(prefix="gh-projects-review-") as temporary:
        overlay = Path(temporary) / "overlay.json"
        overlay.write_text(json.dumps({"Replace": replacements}))
        command = ["go", "test", f"-overlay={overlay}"]
        if sys.argv[1:] == ["--bench"]:
            command += ["./internal/ui", "-run=^$", "-bench=^BenchmarkReviewDetailScroll$", "-benchtime=3x", "-count=2"]
        elif not sys.argv[1:]:
            command += ["./internal/ui", "./internal/github", "./internal/config", "-run=^TestReview", "-count=1", "-v"]
        else:
            raise SystemExit("Usage: reproduce.py [--bench]")
        environment = os.environ.copy()
        environment.setdefault("GOCACHE", str(Path(tempfile.gettempdir()) / "gh-projects-tui-review-go-cache"))
        return subprocess.run(command, cwd=root, env=environment).returncode


if __name__ == "__main__":
    sys.exit(main())
