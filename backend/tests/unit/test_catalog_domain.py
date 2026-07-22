from __future__ import annotations

import sys
import unittest
from dataclasses import replace
from datetime import UTC, datetime
from pathlib import Path
from uuid import uuid4


BACKEND = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(BACKEND / "src"))

from thief.catalog.model import (  # noqa: E402
    PromptTemplate,
    SourceAttribution,
    TemplateStatus,
    normalize_slug,
)


class CatalogDomainTests(unittest.TestCase):
    def setUp(self) -> None:
        self.now = datetime(2026, 7, 22, 16, 0, tzinfo=UTC)

    def test_publication_states_are_fixed_and_fail_closed(self) -> None:
        self.assertEqual(
            {status.value for status in TemplateStatus},
            {"published", "suppressed", "deleted"},
        )
        template = self._template(TemplateStatus.PUBLISHED)

        self.assertTrue(template.is_public())
        self.assertFalse(
            replace(template, status=TemplateStatus.SUPPRESSED).is_public()
        )
        self.assertFalse(replace(template, status=TemplateStatus.DELETED).is_public())

    def test_template_keeps_prompt_model_and_complete_source_attribution(self) -> None:
        template = self._template(TemplateStatus.PUBLISHED)

        self.assertEqual(template.prompt, "a cinematic portrait")
        self.assertEqual(template.source_model, "stable-diffusion")
        self.assertEqual(template.aspect_ratio, "1:1")
        self.assertEqual(template.content_hash, "a" * 64)
        self.assertEqual(template.source.object_id, "source-42")
        self.assertEqual(template.source.revision, "revision-1")
        self.assertEqual(template.source.license, "test-fixture")
        self.assertEqual(template.source.collected_at, self.now)

    def test_slug_has_one_stable_url_form(self) -> None:
        self.assertEqual(normalize_slug("  Cinematic Portrait  "), "cinematic-portrait")
        with self.assertRaisesRegex(ValueError, "slug is required"):
            normalize_slug("  ")
        with self.assertRaisesRegex(ValueError, "slug contains unsupported characters"):
            normalize_slug("portrait/1")

    def _template(self, status: TemplateStatus) -> PromptTemplate:
        return PromptTemplate(
            id=uuid4(),
            slug="cinematic-portrait",
            title="Cinematic portrait",
            prompt="a cinematic portrait",
            negative_prompt=None,
            source_model="stable-diffusion",
            aspect_ratio="1:1",
            category_id=uuid4(),
            source=SourceAttribution(
                name="fixture",
                url="https://example.test/items/42",
                object_id="source-42",
                revision="revision-1",
                license="test-fixture",
                collected_at=self.now,
            ),
            content_hash="a" * 64,
            status=status,
            published_at=self.now,
        )


if __name__ == "__main__":
    unittest.main()
