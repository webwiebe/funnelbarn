#!/usr/bin/env python3
"""scripts/rotate-r2-token.sh handles live credentials, so what it must never do
is proven here against a stub sops: put a value in a command line or its output,
write anything after a bad paste, or reuse one token for two environments.

Run with:  python3 -m unittest discover -s scripts -p 'test_*.py'
"""

from __future__ import annotations

import os
import pathlib
import stat
import subprocess
import tempfile
import unittest

SCRIPT = pathlib.Path(__file__).resolve().parent / "rotate-r2-token.sh"

STUB_SOPS = """#!/bin/sh
printf '%s\\n' "$*" >> "$SOPS_LOG"
printf 'stdin=' >> "$SOPS_LOG"
cat >> "$SOPS_LOG"
printf '\\n' >> "$SOPS_LOG"
"""

ENVS = ("production", "staging", "testing")


def pair(char: str) -> tuple[str, str]:
    return char * 32, char * 64


class RotateR2Token(unittest.TestCase):
    def setUp(self) -> None:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.root = pathlib.Path(tmp.name)
        subprocess.run(["git", "init", "-q"], cwd=self.root, check=True)
        for env in ENVS:
            path = self.root / "deploy/k8s" / env / "secret.yaml"
            path.parent.mkdir(parents=True)
            path.write_text("stringData: {}\n")
        bin_dir = self.root / "stub-bin"
        bin_dir.mkdir()
        sops = bin_dir / "sops"
        sops.write_text(STUB_SOPS)
        sops.chmod(sops.stat().st_mode | stat.S_IEXEC)
        self.log = self.root / "sops.log"
        self.log.touch()
        self.env = {
            **os.environ,
            "PATH": f"{bin_dir}{os.pathsep}{os.environ['PATH']}",
            "SOPS_LOG": str(self.log),
        }

    def run_script(self, typed: list[str], *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["bash", str(SCRIPT), *args],
            cwd=self.root,
            env=self.env,
            input="".join(f"{line}\n" for line in typed),
            capture_output=True,
            text=True,
        )

    def test_sets_both_keys_per_environment_through_stdin(self) -> None:
        tokens = [pair("a"), pair("b"), pair("c")]
        result = self.run_script([value for token in tokens for value in token])
        self.assertEqual(result.returncode, 0, result.stderr)

        calls = self.log.read_text().splitlines()
        argv = calls[0::2]
        stdin = calls[1::2]
        self.assertEqual(len(argv), 6)
        expected = []
        for env, (key_id, secret) in zip(ENVS, tokens):
            for key, value in (
                ("FUNNELBARN_R2_ACCESS_KEY_ID", key_id),
                ("FUNNELBARN_R2_SECRET_ACCESS_KEY", secret),
            ):
                expected.append(
                    (f'set --value-stdin deploy/k8s/{env}/secret.yaml ["stringData"]["{key}"]', f'stdin="{value}"')
                )
        self.assertEqual(list(zip(argv, stdin)), expected)

    def test_prints_no_value(self) -> None:
        tokens = [pair("a"), pair("b"), pair("c")]
        result = self.run_script([value for token in tokens for value in token])
        output = result.stdout + result.stderr
        for key_id, secret in tokens:
            self.assertNotIn(key_id, output)
            self.assertNotIn(secret, output)

    def test_no_value_in_a_command_line(self) -> None:
        tokens = [pair("a"), pair("b"), pair("c")]
        self.run_script([value for token in tokens for value in token])
        argv = self.log.read_text().splitlines()[0::2]
        for line in argv:
            for char in "abc":
                self.assertNotIn(char * 32, line)

    def test_a_bad_paste_writes_nothing(self) -> None:
        cases = {
            "short key id": ["abc", pair("a")[1]],
            "short secret": [pair("a")[0], "abc"],
            "uppercase key id": [pair("A")[0], pair("a")[1]],
        }
        for name, typed in cases.items():
            with self.subTest(name):
                # Production is good, testing is bad: the good one must not land.
                result = self.run_script([*pair("b"), *typed], "production", "testing")
                self.assertEqual(result.returncode, 1)
                self.assertIn("nothing written", result.stderr)
                self.assertEqual(self.log.read_text(), "")

    def test_one_token_for_two_environments_writes_nothing(self) -> None:
        result = self.run_script([*pair("a"), *pair("a")], "staging", "testing")
        self.assertEqual(result.returncode, 1)
        self.assertIn("already used", result.stderr)
        self.assertEqual(self.log.read_text(), "")

    def test_unknown_environment_stops_before_any_prompt(self) -> None:
        result = self.run_script([], "prod")
        self.assertEqual(result.returncode, 1)
        self.assertIn("no SOPS file", result.stderr)

    def test_one_environment_touches_only_that_file(self) -> None:
        result = self.run_script(list(pair("a")), "testing")
        self.assertEqual(result.returncode, 0, result.stderr)
        argv = self.log.read_text().splitlines()[0::2]
        self.assertEqual(len(argv), 2)
        for line in argv:
            self.assertIn("deploy/k8s/testing/secret.yaml", line)


if __name__ == "__main__":
    unittest.main()
