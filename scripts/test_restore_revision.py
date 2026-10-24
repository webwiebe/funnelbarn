#!/usr/bin/env python3
"""deploy/restore-revision.sh puts back the revision recorded before a deploy (issue #396).

On 2026-10-23 a production apply timed out before it changed the writer, and
the plain `rollout undo` then moved the writer one release further back. These
tests run the script against a fake kubectl that keeps rollout history the way
the real one does (an undo re-adds the old template as the newest revision):

  - an apply that never landed               -> no undo at all
  - an apply that landed                     -> undo --to-revision=<recorded>
  - nothing recorded (first reader release)  -> fails without an undo
  - a failing undo                           -> the script fails

Run with:  python3 -m unittest discover -s scripts -p 'test_*.py'
"""

from __future__ import annotations

import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
SCRIPT = REPO_ROOT / "deploy" / "restore-revision.sh"
NS = "funnelbarn-production"

FAKE_KUBECTL = r"""#!/usr/bin/env python3
import json, os, sys
state_path = os.environ["FAKE_KUBE_STATE"]
state = json.load(open(state_path))
args = sys.argv[1:]
if args[:1] == ["-n"]:
    args = args[2:]
with open(state_path + ".calls", "a") as f:
    f.write(" ".join(args) + "\n")
history = state["history"]
if args[:2] == ["rollout", "history"]:
    target = args[2]
    print(target.replace("/", ".apps/", 1))
    print("REVISION  CHANGE-CAUSE")
    for rev in history[target]:
        print(f"{rev}         <none>")
elif args[:2] == ["rollout", "undo"]:
    if state.get("undo_fails"):
        print("error: undo failed", file=sys.stderr)
        sys.exit(1)
    target, to = args[2], int(args[3].split("=", 1)[1])
    revs = history[target]
    revs.remove(to)
    revs.append(max(revs + [to]) + 1)
    state.setdefault("restored", {})[target] = to
    json.dump(state, open(state_path, "w"))
    print(f"{target} rolled back")
else:
    sys.exit(f"fake kubectl: unexpected {args}")
"""


class RestoreRevisionTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        bindir = Path(self.tmp.name) / "bin"
        bindir.mkdir()
        kubectl = bindir / "kubectl"
        kubectl.write_text(FAKE_KUBECTL)
        kubectl.chmod(0o755)
        self.state = Path(self.tmp.name) / "state.json"
        self.env = {
            **os.environ,
            "PATH": f"{bindir}{os.pathsep}{os.environ['PATH']}",
            "FAKE_KUBE_STATE": str(self.state),
        }

    def tearDown(self):
        self.tmp.cleanup()

    def given(self, history, **extra):
        self.state.write_text(json.dumps({"history": history, **extra}))

    def run_script(self, *args):
        return subprocess.run(
            ["sh", str(SCRIPT), *args], capture_output=True, text=True, env=self.env
        )

    def undo_calls(self):
        calls = Path(str(self.state) + ".calls")
        if not calls.exists():
            return []
        return [c for c in calls.read_text().splitlines() if c.startswith("rollout undo")]

    def test_record_prints_the_newest_revision(self):
        self.given({"deployment/funnelbarn": [162, 164]})
        out = self.run_script("record", NS, "deployment/funnelbarn")
        self.assertEqual(out.returncode, 0, out.stderr)
        self.assertEqual(out.stdout.strip(), "164")

    def test_an_apply_that_never_landed_is_not_undone(self):
        # The 2026-10-23 writer: recorded 164, the apply timed out, still 164.
        self.given({"deployment/funnelbarn": [162, 164]})
        out = self.run_script("restore", NS, "deployment/funnelbarn", "164")
        self.assertEqual(out.returncode, 0, out.stderr)
        self.assertIn("nothing to undo", out.stdout)
        self.assertEqual(self.undo_calls(), [])

    def test_an_apply_that_landed_goes_back_to_the_recorded_revision(self):
        self.given({"deployment/funnelbarn-web": [262, 263, 264]})
        out = self.run_script("restore", NS, "deployment/funnelbarn-web", "263")
        self.assertEqual(out.returncode, 0, out.stderr)
        self.assertEqual(self.undo_calls(), ["rollout undo deployment/funnelbarn-web --to-revision=263"])
        self.assertEqual(json.loads(self.state.read_text())["restored"], {"deployment/funnelbarn-web": 263})

    def test_a_statefulset_is_restored_the_same_way(self):
        self.given({"statefulset/funnelbarn-reader": [9, 10, 11]})
        out = self.run_script("restore", NS, "statefulset/funnelbarn-reader", "10")
        self.assertEqual(out.returncode, 0, out.stderr)
        self.assertEqual(self.undo_calls(), ["rollout undo statefulset/funnelbarn-reader --to-revision=10"])

    def test_no_recorded_revision_fails_without_an_undo(self):
        self.given({"statefulset/funnelbarn-reader": [1]})
        out = self.run_script("restore", NS, "statefulset/funnelbarn-reader", "")
        self.assertEqual(out.returncode, 1)
        self.assertIn("RUNBOOK", out.stderr)
        self.assertEqual(self.undo_calls(), [])

    def test_a_failing_undo_fails_the_script(self):
        self.given({"deployment/funnelbarn": [162, 164, 165]}, undo_fails=True)
        out = self.run_script("restore", NS, "deployment/funnelbarn", "164")
        self.assertNotEqual(out.returncode, 0)

    def test_wrong_arguments_are_a_usage_error(self):
        self.given({})
        self.assertEqual(self.run_script("restore", NS, "deployment/funnelbarn").returncode, 2)
        self.assertEqual(self.run_script("undo", NS, "deployment/funnelbarn").returncode, 2)


if __name__ == "__main__":
    unittest.main(verbosity=2)
