#!/usr/bin/env python3
"""Runs scripts/restore-proof.sh against a fake ssh, so nothing reaches a cluster.

The script's standard output lands in a public Actions log. Each assertion fails
if one property goes:

  - a count per table in the output            -> test_output_has_no_counts
  - the proof reaching for another cluster     -> test_stays_on_layer7_production
  - the Job carrying a key value               -> test_key_comes_from_the_secret_only
  - a failed restore reading as a pass         -> test_failed_restore_fails
  - an empty production count reading as a pass-> test_empty_production_count_fails
  - a table lost in the restore passing        -> test_missing_table_fails
  - the Job left behind                        -> test_job_deleted_on_every_path

Run with:  python3 -m unittest discover -s scripts -p 'test_*.py'
"""

from __future__ import annotations

import os
import pathlib
import subprocess
import tempfile
import textwrap
import unittest

REPO_ROOT = pathlib.Path(__file__).resolve().parent.parent
SCRIPT = REPO_ROOT / "scripts" / "restore-proof.sh"

# Distinct enough that a substring match cannot come from a timestamp or exit code.
EVENTS_BEFORE, EVENTS_RESTORED, EVENTS_AFTER = 7654321, 7654322, 7654323
SESSIONS = 2469135

GOOD_LOG = f"""\
PROOF restore_exit 0
PROOF elapsed_seconds 42
PROOF integrity_exit 0
PROOFROW events {EVENTS_RESTORED}
PROOFROW sessions {SESSIONS}
PROOF done
"""

FAKE_SSH = textwrap.dedent(
    """\
    #!/usr/bin/env bash
    # Records every remote command and answers the few the script asks.
    dir="$(dirname "$0")"
    cmd="${!#}"
    echo "$@" >> "$dir/calls"
    case "$cmd" in
      *" exec -i "*)
        cat > /dev/null
        n=$(cat "$dir/count-calls" 2>/dev/null || echo 0)
        echo $((n + 1)) > "$dir/count-calls"
        cat "$dir/counts-$n"
        ;;
      *" apply -f -"*) cat > "$dir/manifest.yaml" ;;
      *"get deploy funnelbarn"*imagePullSecrets*) echo '[{"name":"ghcr-secret"}]' ;;
      *"get deploy funnelbarn"*) echo 'ghcr.io/webwiebe/funnelbarn/service@sha256:abc' ;;
      *"get job"*) echo Complete ;;
      *" logs "*) cat "$dir/job-log" ;;
    esac
    """
)


class RestoreProof(unittest.TestCase):
    def run_proof(self, log=GOOD_LOG, before=None, after=None):
        """Returns the finished process and the fake's working directory."""
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        fake = pathlib.Path(tmp.name)
        ssh = fake / "ssh"
        ssh.write_text(FAKE_SSH)
        ssh.chmod(0o700)
        default_before = f"events\t{EVENTS_BEFORE}\nsessions\t{SESSIONS}\n"
        default_after = f"events\t{EVENTS_AFTER}\nsessions\t{SESSIONS}\n"
        (fake / "counts-0").write_text(default_before if before is None else before)
        (fake / "counts-1").write_text(default_after if after is None else after)
        (fake / "job-log").write_text(log)
        run = subprocess.run(
            ["bash", str(SCRIPT), str(fake / "out")],
            env={**os.environ, "PATH": f"{fake}:{os.environ['PATH']}"},
            capture_output=True,
            text=True,
            timeout=60,
        )
        return run, fake

    def test_good_restore_passes(self):
        run, _ = self.run_proof()
        self.assertEqual(run.returncode, 0, run.stdout + run.stderr)
        self.assertIn("restore_exit=0 elapsed_seconds=42 integrity_exit=0", run.stdout)
        self.assertIn("tables=2 restored=2", run.stdout)
        self.assertIn("in_range=2 outside=0", run.stdout)

    def test_output_has_no_counts(self):
        below = f"PROOF restore_exit 0\nPROOF integrity_exit 0\nPROOFROW events {EVENTS_BEFORE - 5}\nPROOFROW sessions {SESSIONS}\nPROOF done\n"
        for log in (GOOD_LOG, below):
            run, _ = self.run_proof(log=log)
            output = run.stdout + run.stderr
            for count in (EVENTS_BEFORE - 5, EVENTS_BEFORE, EVENTS_RESTORED, EVENTS_AFTER, SESSIONS):
                self.assertNotIn(str(count), output)

    def test_table_out_of_range_is_named_and_passes(self):
        log = f"PROOF restore_exit 0\nPROOF integrity_exit 0\nPROOFROW events {EVENTS_BEFORE - 5}\nPROOFROW sessions {SESSIONS}\nPROOF done\n"
        run, _ = self.run_proof(log=log)
        self.assertEqual(run.returncode, 0, run.stdout + run.stderr)
        self.assertIn("outside: events below", run.stdout)

    def test_stays_on_layer7_production(self):
        _, fake = self.run_proof()
        calls = (fake / "calls").read_text().splitlines()
        self.assertTrue(calls)
        for call in calls:
            self.assertIn("deployer@layer7.wiebe.xyz", call)
            self.assertIn("kubectl -n funnelbarn-production ", call)
        self.assertNotIn("testing", SCRIPT.read_text())

    def test_key_comes_from_the_secret_only(self):
        _, fake = self.run_proof()
        manifest = (fake / "manifest.yaml").read_text()
        self.assertIn("secretKeyRef: {name: funnelbarn-secrets, key: FUNNELBARN_R2_ACCESS_KEY_ID}", manifest)
        self.assertIn("secretKeyRef: {name: funnelbarn-secrets, key: FUNNELBARN_R2_SECRET_ACCESS_KEY}", manifest)
        self.assertEqual(manifest.count("value:"), 1)
        self.assertIn("value: funnelbarn/litestream/production/funnelbarn.db", manifest)

    def test_failed_restore_fails(self):
        run, _ = self.run_proof(log="PROOF restore_exit 1\nPROOF elapsed_seconds 3\n")
        self.assertEqual(run.returncode, 1)
        self.assertIn("restore_exit=1", run.stdout)

    def test_failed_integrity_fails(self):
        run, _ = self.run_proof(log=GOOD_LOG.replace("integrity_exit 0", "integrity_exit 1"))
        self.assertEqual(run.returncode, 1)

    def test_no_log_fails(self):
        run, _ = self.run_proof(log="")
        self.assertEqual(run.returncode, 1)

    def test_empty_production_count_fails(self):
        run, _ = self.run_proof(after="")
        self.assertEqual(run.returncode, 1)

    def test_missing_table_fails(self):
        run, _ = self.run_proof(log=GOOD_LOG.replace(f"PROOFROW sessions {SESSIONS}\n", ""))
        self.assertEqual(run.returncode, 1)
        self.assertIn("missing=['sessions']", run.stdout)

    def test_job_deleted_on_every_path(self):
        for log in (GOOD_LOG, ""):
            _, fake = self.run_proof(log=log)
            self.assertRegex((fake / "calls").read_text(), r"delete job restore-proof-\d+ ")


if __name__ == "__main__":
    unittest.main()
