from __future__ import annotations

from datetime import UTC, datetime
from uuid import UUID

from thief.catalog.importer import DiffusionRecord, ImportManifest, records_checksum


def fixture_1k() -> tuple[ImportManifest, tuple[DiffusionRecord, ...]]:
    records = tuple(
        DiffusionRecord(
            image_name=f"{UUID(int=index + 1)}.png",
            prompt=f"fixture prompt {index:04d}, cinematic lighting",
            seed=index,
            step=50,
            cfg=7.0,
            sampler="k_lms",
            width=512 if index % 2 == 0 else 768,
            height=512 if index % 2 == 0 else 512,
        )
        for index in range(1000)
    )
    manifest = ImportManifest(
        source_name="DiffusionDB fixture",
        source_url="https://huggingface.co/datasets/poloclub/diffusiondb",
        revision="fixture-v1",
        license="CC0-1.0 fixture contract",
        collected_at=datetime(2026, 7, 23, tzinfo=UTC),
        item_count=len(records),
        checksum=records_checksum(records),
    )
    return manifest, records
