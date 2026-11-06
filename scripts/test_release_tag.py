#!/usr/bin/env python3
"""Tests deploy/release-tag.sh, which turns a deployed commit into the release
tag production and staging report as their version (#489).

Each test builds a throwaway origin and clone in a temp dir. Each assertion
fails if one behaviour is removed:

  - a tag input passing through                -> test_tag_prints_itself
  - a sha resolving to its release tag         -> test_sha_prints_its_release_tag
  - the highest tag winning on a shared commit -> test_highest_release_tag_wins
  - non-release tags being ignored             -> test_ignores_tags_that_are_not_vXYZ
  - waiting for binary-release's tag           -> test_fetches_a_tag_pushed_after_the_first_attempt
  - failing without a tag                      -> test_sha_without_a_tag_fails

Run with:  python3 -m unittest discover -s scripts -p 'test_*.py'
"""

from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent.parent / "deploy" / "release-tag.sh"

GIT_ENV = {
    "GIT_AUTHOR_NAME": "t",
    "GIT_AUTHOR_EMAIL": "t@example.invalid",
    "GIT_COMMITTER_NAME": "t",
    "GIT_COMMITTER_EMAIL": "t@example.invalid",
    "GIT_CONFIG_GLOBAL": os.devnull,
    "GIT_CONFIG_NOSYSTEM": "1",
}


class ReleaseTagTest(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        root = Path(self._tmp.name)
        self.origin = root / "origin"
        self.clone = root / "clone"
        self.env = {**os.environ, **GIT_ENV, "RELEASE_TAG_WAIT": "0"}
        self.git(root, "init", "-q", "-b", "main", str(self.origin))
        self.git(self.origin, "commit", "-q", "--allow-empty", "-m", "one")
        self.first = self.git(self.origin, "rev-parse", "HEAD")
        self.git(self.origin, "commit", "-q", "--allow-empty", "-m", "two")
        self.sha = self.git(self.origin, "rev-parse", "HEAD")
        # binary-release makes annotated tags.
        self.git(self.origin, "tag", "-a", "v0.6.176", "-m", "r", self.first)
        self.git(root, "clone", "-q", str(self.origin), str(self.clone))

    def tearDown(self):
        self._tmp.cleanup()

    def git(self, cwd: Path, *args: str) -> str:
        out = subprocess.run(
            ["git", *args], cwd=cwd, env={**os.environ, **GIT_ENV},
            check=True, capture_output=True, text=True,
        )
        return out.stdout.strip()

    def tag_in_clone(self, *args: str):
        self.git(self.clone, "tag", *args)

    def run_script(self, arg: str, attempts: str = "1") -> subprocess.CompletedProcess:
        return subprocess.run(
            ["bash", str(SCRIPT), arg], cwd=self.clone,
            env={**self.env, "RELEASE_TAG_ATTEMPTS": attempts},
            capture_output=True, text=True,
        )

    def test_tag_prints_itself(self):
        r = self.run_script("v0.6.176")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stdout, "v0.6.176\n")

    def test_unknown_tag_fails(self):
        r = self.run_script("v9.9.9")
        self.assertEqual(r.returncode, 1)
        self.assertEqual(r.stdout, "")

    def test_sha_prints_its_release_tag(self):
        r = self.run_script(self.first)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stdout, "v0.6.176\n")

    def test_highest_release_tag_wins(self):
        self.tag_in_clone("-a", "v0.6.9", "-m", "r", self.sha)
        self.tag_in_clone("-a", "v0.6.10", "-m", "r", self.sha)
        r = self.run_script(self.sha)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stdout, "v0.6.10\n")

    def test_ignores_tags_that_are_not_vXYZ(self):
        # binary-release also pushes sdks/go/vX.Y.Z on the same commit.
        self.tag_in_clone("-a", "sdks/go/v0.6.177", "-m", "r", self.sha)
        self.tag_in_clone("funnelbarn-js-v1.2.3", self.sha)
        self.tag_in_clone("v0.6.177-rc1", self.sha)
        r = self.run_script(self.sha)
        self.assertEqual(r.returncode, 1)
        self.assertEqual(r.stdout, "")

    def test_fetches_a_tag_pushed_after_the_first_attempt(self):
        # The tag lands on origin after the clone, as when binary-release runs
        # beside the pipeline.
        self.git(self.origin, "tag", "-a", "v0.6.177", "-m", "r", self.sha)
        r = self.run_script(self.sha, attempts="2")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stdout, "v0.6.177\n")
        self.assertIn("attempt 1/2", r.stderr)

    def test_sha_without_a_tag_fails(self):
        r = self.run_script(self.sha, attempts="2")
        self.assertEqual(r.returncode, 1)
        self.assertEqual(r.stdout, "")
        self.assertIn("no vX.Y.Z tag points at " + self.sha, r.stderr)

    def test_unknown_commit_fails(self):
        r = self.run_script("0" * 40)
        self.assertEqual(r.returncode, 1)
        self.assertEqual(r.stdout, "")

    def test_requires_one_argument(self):
        r = subprocess.run(["bash", str(SCRIPT)], cwd=self.clone, env=self.env,
                           capture_output=True, text=True)
        self.assertEqual(r.returncode, 2)


if __name__ == "__main__":
    unittest.main()
