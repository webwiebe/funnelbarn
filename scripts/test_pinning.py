#!/usr/bin/env python3
"""Digest pinning and the rollback's deployed-SHA lookup (issue #364).

  - deploy/pin-release.sh pins images by digest when given digests
  - deploy/deployed-sha.sh finds the SHA from the release annotation, and
    still finds it from a tag-form image; a digest-form image carries no SHA

Run with:  python3 -m unittest discover -s scripts -p 'test_*.py'
"""

from __future__ import annotations

import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
PIN = REPO_ROOT / "deploy" / "pin-release.sh"
DEPLOYED_SHA = REPO_ROOT / "deploy" / "deployed-sha.sh"

SHA = "0123456789abcdef0123456789abcdef01234567"
SERVICE_DIGEST = "sha256:" + "a" * 64
WEB_DIGEST = "sha256:" + "b" * 64


def deployed_sha(annotation: str, image: str) -> str:
    out = subprocess.run(
        [str(DEPLOYED_SHA), annotation, image], capture_output=True, text=True, check=True
    )
    return out.stdout.strip()


class DeployedShaTest(unittest.TestCase):
    def test_annotation_wins_over_a_digest_form_image(self):
        image = f"ghcr.io/webwiebe/funnelbarn/service@{SERVICE_DIGEST}"
        self.assertEqual(deployed_sha(SHA, image), SHA)

    def test_annotation_wins_over_a_tag_form_image(self):
        image = "ghcr.io/webwiebe/funnelbarn/service:ffff"
        self.assertEqual(deployed_sha(SHA, image), SHA)

    def test_tag_form_image_is_the_fallback_before_the_first_annotation(self):
        image = f"ghcr.io/webwiebe/funnelbarn/service:{SHA}"
        self.assertEqual(deployed_sha("", image), SHA)

    def test_digest_form_image_without_annotation_yields_nothing(self):
        # The caller then assumes the release carries migrations (safe default).
        image = f"ghcr.io/webwiebe/funnelbarn/service@{SERVICE_DIGEST}"
        self.assertEqual(deployed_sha("", image), "")

    def test_nothing_known_yields_nothing(self):
        self.assertEqual(deployed_sha("", ""), "")


class PinReleaseTest(unittest.TestCase):
    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.tmp, ignore_errors=True)
        env_dir = self.tmp / "deploy" / "k8s" / "production"
        env_dir.mkdir(parents=True)
        shutil.copy(
            REPO_ROOT / "deploy" / "k8s" / "production" / "kustomization.yaml", env_dir
        )
        for f in (REPO_ROOT / "deploy" / "k8s" / "production").glob("*.yaml"):
            shutil.copy(f, env_dir)
        self.kustomization = env_dir / "kustomization.yaml"

    def pin(self, *args: str) -> subprocess.CompletedProcess:
        return subprocess.run(
            [str(PIN), "production", SHA, "v1.2.3", *args],
            cwd=self.tmp,
            capture_output=True,
            text=True,
        )

    def test_without_digests_only_the_tag_is_set(self):
        self.assertEqual(self.pin().returncode, 0)
        text = self.kustomization.read_text()
        self.assertIn(f"newTag: {SHA}", text)
        self.assertNotIn("digest:", text)

    def test_with_digests_each_image_gets_its_own_digest(self):
        result = self.pin(SERVICE_DIGEST, WEB_DIGEST)
        self.assertEqual(result.returncode, 0, result.stderr)
        text = self.kustomization.read_text()
        service, web = text.split("- name: funnelbarn/web")
        service = service.split("- name: funnelbarn/service")[1]
        self.assertIn(f"digest: {SERVICE_DIGEST}", service)
        self.assertNotIn(WEB_DIGEST, service)
        self.assertIn(f"digest: {WEB_DIGEST}", web)

    def test_a_malformed_digest_is_refused(self):
        result = self.pin("latest", WEB_DIGEST)
        self.assertNotEqual(result.returncode, 0)

    def test_one_digest_alone_is_a_usage_error(self):
        result = self.pin(SERVICE_DIGEST)
        self.assertEqual(result.returncode, 2)


if __name__ == "__main__":
    unittest.main(verbosity=2)
