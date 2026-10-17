#!/usr/bin/env python3
"""Asserts that no workflow step reaches outside its own runner's workspace.

The defect this locks down: every self-hosted job began with a "Reclaim
workspace" step that looped over /Users/wiebe/github-runners/*/_work/... and
deleted the .cache of every runner on the host. A job starting on one runner
wiped the Go module cache of a job still building on another: CI run
38992458520 (go-quality, first attempt, 2026-10-17) failed with files missing
under .cache/go-mod on runner-8. The host runs CI for the whole estate, so a
step like that can break other repositories' jobs as well.

Each assertion fails if the corresponding half of the fix is removed:

  - a glob over the runners root comes back       -> test_no_glob_over_the_runners_root
  - a reclaim step names a path of its own        -> test_reclaim_steps_use_their_own_workspace
  - the reclaim script deletes outside its own
    workspace, or leaves read-only files behind   -> test_reclaim_script_clears_only_its_own_cache

Run with:  python3 -m unittest discover -s scripts -p 'test_*.py'
"""

from __future__ import annotations

import os
import re
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

import yaml

REPO_ROOT = Path(__file__).resolve().parent.parent
WORKFLOWS = sorted((REPO_ROOT / ".github" / "workflows").glob("*.y*ml"))
RECLAIM_STEP = "Reclaim workspace"

# A wildcard anywhere under a runners root: github-runners/*, github-runners/runner-*, ...
RUNNERS_GLOB = re.compile(r"github-runners/[^\s\"']*[*?\[]")


def reclaim_steps() -> list[tuple[str, str, dict]]:
    found = []
    for path in WORKFLOWS:
        data = yaml.safe_load(path.read_text())
        for job_name, job in (data.get("jobs") or {}).items():
            for step in job.get("steps") or []:
                if str(step.get("name", "")).startswith(RECLAIM_STEP):
                    found.append((path.name, job_name, step))
    return found


class RunnerScopeTest(unittest.TestCase):
    def executable_script(self, workflow: str, job: str, step: dict) -> str:
        """Returns the step's script, or fails without running a script that
        names a host path: the old step would delete the caches of every runner
        on the machine running this test."""
        script = step.get("run", "")
        if "/Users/" in script or RUNNERS_GLOB.search(script):
            self.fail(f"{workflow} job {job}: reclaim names a host path; not executing it")
        return script

    def test_no_glob_over_the_runners_root(self):
        for path in WORKFLOWS:
            for lineno, line in enumerate(path.read_text().splitlines(), 1):
                self.assertIsNone(
                    RUNNERS_GLOB.search(line),
                    f"{path.name}:{lineno} globs over other runners' directories: {line.strip()}",
                )

    def test_reclaim_steps_use_their_own_workspace(self):
        steps = reclaim_steps()
        self.assertTrue(steps, "no reclaim steps found; update this test if they were removed on purpose")
        for workflow, job, step in steps:
            script = step.get("run", "")
            where = f"{workflow} job {job}"
            self.assertIn("${GITHUB_WORKSPACE:?}", script, f"{where}: reclaim must target $GITHUB_WORKSPACE")
            self.assertNotIn("/Users/", script, f"{where}: reclaim must not name an absolute path")

    def test_reclaim_script_clears_only_its_own_cache(self):
        for workflow, job, step in reclaim_steps():
            script = self.executable_script(workflow, job, step)
            with self.subTest(workflow=workflow, job=job), tempfile.TemporaryDirectory() as root:
                own = Path(root, "runner-1", "_work", "funnelbarn", "funnelbarn")
                other = Path(root, "runner-2", "_work", "funnelbarn", "funnelbarn")
                for ws in (own, other):
                    mod = ws / ".cache" / "go-mod" / "example.com" / "m@v1.0.0"
                    mod.mkdir(parents=True)
                    (mod / "m.go").write_text("package m\n")
                    # Go writes the module cache read-only, which is why the step exists.
                    (mod / "m.go").chmod(stat.S_IRUSR)
                    mod.chmod(stat.S_IRUSR | stat.S_IXUSR)
                (own / "keep.txt").write_text("checkout\n")

                env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin"), "GITHUB_WORKSPACE": str(own)}
                subprocess.run(["bash", "-e", "-c", script], env=env, check=True)

                self.assertFalse((own / ".cache").exists(), "own .cache must be cleared")
                self.assertTrue((own / "keep.txt").exists(), "only .cache may be removed")
                self.assertTrue(
                    (other / ".cache" / "go-mod" / "example.com" / "m@v1.0.0" / "m.go").exists(),
                    "another runner's cache must be left alone",
                )
                for dirpath, _, _ in os.walk(other):
                    Path(dirpath).chmod(stat.S_IRWXU)

    def test_reclaim_script_refuses_an_unset_workspace(self):
        for workflow, job, step in reclaim_steps():
            script = self.executable_script(workflow, job, step)
            with self.subTest(workflow=workflow, job=job):
                env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin")}
                result = subprocess.run(["bash", "-e", "-c", script], env=env, capture_output=True)
                self.assertNotEqual(result.returncode, 0, "an unset GITHUB_WORKSPACE must stop the step")


if __name__ == "__main__":
    unittest.main()
