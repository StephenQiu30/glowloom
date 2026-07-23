from __future__ import annotations

import hashlib
import json
import sys
import unittest
from dataclasses import asdict, replace
from datetime import UTC, datetime
from pathlib import Path


BACKEND = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(BACKEND / "src"))


class RecordingRepository:
    def __init__(self) -> None:
        self.batch: object | None = None

    def import_batch(self, batch: object) -> object:
        from thief.catalog.importer import ImportReport

        self.batch = batch
        return ImportReport(True, 1, 2, 1)


class CatalogImporterTests(unittest.TestCase):
    def setUp(self) -> None:
        from thief.catalog.importer import DiffusionRecord

        self.records = (
            DiffusionRecord(
                image_name="00000000-0000-4000-8000-000000000001.png",
                prompt="  A cinematic portrait  ",
                seed=42,
                step=50,
                cfg=7.0,
                sampler="k_lms",
                width=512,
                height=768,
            ),
            DiffusionRecord(
                image_name="00000000-0000-4000-8000-000000000002.png",
                prompt="A cinematic portrait",
                seed=42,
                step=50,
                cfg=7.0,
                sampler="k_lms",
                width=512,
                height=768,
            ),
        )

    def test_manifest_tampering_is_rejected_before_writing(self) -> None:
        from thief.catalog.importer import CatalogImporter, InvalidManifest

        repository = RecordingRepository()
        manifest = self._manifest(self.records)
        tampered = replace(self.records[0], prompt="tampered prompt")

        with self.assertRaisesRegex(InvalidManifest, "checksum"):
            CatalogImporter(repository).execute(manifest, (tampered, self.records[1]))

        self.assertIsNone(repository.batch)

    def test_normalizes_and_deduplicates_templates_but_keeps_examples(self) -> None:
        from thief.catalog.importer import CatalogImporter
        from thief.catalog.model import TemplateStatus

        repository = RecordingRepository()
        report = CatalogImporter(repository).execute(
            self._manifest(self.records),
            self.records,
        )
        batch = repository.batch

        self.assertEqual((report.templates_created, report.examples_created), (1, 2))
        self.assertEqual(len(batch.templates), 1)
        self.assertEqual(len(batch.examples), 2)
        self.assertEqual(len(batch.search_documents), 1)
        template = batch.templates[0]
        self.assertEqual(template.prompt, "A cinematic portrait")
        self.assertEqual(template.aspect_ratio, "2:3")
        self.assertEqual(
            template.parameters,
            {
                "cfg": 7.0,
                "height": 768,
                "sampler": "k_lms",
                "seed": 42,
                "step": 50,
                "width": 512,
            },
        )
        self.assertIs(template.status, TemplateStatus.SUPPRESSED)
        self.assertNotEqual(batch.examples[0].asset_id, batch.examples[1].asset_id)
        self.assertIn("A cinematic portrait", batch.search_documents[0].search_text)

    def _manifest(self, records: tuple[object, ...]) -> object:
        from thief.catalog.importer import ImportManifest

        payload = "\n".join(
            json.dumps(asdict(record), sort_keys=True, separators=(",", ":"))
            for record in records
        )
        return ImportManifest(
            source_name="DiffusionDB fixture",
            source_url="https://huggingface.co/datasets/poloclub/diffusiondb",
            revision="fixture-v1",
            license="CC0-1.0 fixture contract",
            collected_at=datetime(2026, 7, 23, tzinfo=UTC),
            item_count=len(records),
            checksum=hashlib.sha256(payload.encode()).hexdigest(),
        )


if __name__ == "__main__":
    unittest.main()
