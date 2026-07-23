from __future__ import annotations

import hashlib
import json
import math
from dataclasses import asdict, dataclass
from datetime import datetime
from typing import Protocol
from uuid import NAMESPACE_URL, UUID, uuid5

from thief.catalog.model import (
    Category,
    GenerationExample,
    PromptTemplate,
    SourceAttribution,
    TemplateStatus,
)


@dataclass(frozen=True, slots=True)
class DiffusionRecord:
    image_name: str
    prompt: str
    seed: int
    step: int
    cfg: float
    sampler: str
    width: int
    height: int
@dataclass(frozen=True, slots=True)
class ImportManifest:
    source_name: str
    source_url: str
    revision: str
    license: str
    collected_at: datetime
    item_count: int
    checksum: str
@dataclass(frozen=True, slots=True)
class SearchDocument:
    template_id: UUID
    search_text: str
@dataclass(frozen=True, slots=True)
class CatalogImportBatch:
    id: UUID
    manifest: ImportManifest
    category: Category
    templates: tuple[PromptTemplate, ...]
    examples: tuple[GenerationExample, ...]
    search_documents: tuple[SearchDocument, ...]
@dataclass(frozen=True, slots=True)
class ImportReport:
    manifest_created: bool
    templates_created: int
    examples_created: int
    search_documents_created: int
class CatalogImportRepository(Protocol):
    def import_batch(self, batch: CatalogImportBatch) -> ImportReport: ...


class InvalidManifest(ValueError):
    pass
def records_checksum(records: tuple[DiffusionRecord, ...]) -> str:
    payload = "\n".join(
        json.dumps(asdict(record), sort_keys=True, separators=(",", ":"))
        for record in records
    )
    return hashlib.sha256(payload.encode()).hexdigest()


class CatalogImporter:
    def __init__(self, repository: CatalogImportRepository) -> None:
        self._repository = repository

    def execute(
        self,
        manifest: ImportManifest,
        records: tuple[DiffusionRecord, ...],
    ) -> ImportReport:
        _validate(manifest, records)
        category = Category(
            id=_stable_id("category:uncategorized"),
            slug="uncategorized",
            name="Uncategorized",
        )
        templates: dict[str, PromptTemplate] = {}
        examples: list[GenerationExample] = []
        searches: dict[UUID, SearchDocument] = {}
        for record in records:
            prompt = " ".join(record.prompt.split())
            parameters: dict[str, int | float | str] = {
                "cfg": record.cfg,
                "height": record.height,
                "sampler": record.sampler,
                "seed": record.seed,
                "step": record.step,
                "width": record.width,
            }
            content_hash = _content_hash(prompt, parameters)
            template = templates.setdefault(
                content_hash,
                _template(manifest, record, category.id, prompt, parameters),
            )
            asset_id = _stable_id(
                f"asset:{manifest.source_name}:{record.image_name}"
            )
            examples.append(
                GenerationExample(
                    id=_stable_id(f"example:{template.id}:{asset_id}"),
                    template_id=template.id,
                    asset_id=asset_id,
                    alt_text=template.title,
                    position=0,
                )
            )
            searches[template.id] = SearchDocument(
                template_id=template.id,
                search_text="\n".join(
                    (template.title, prompt, template.source_model, manifest.source_name)
                ),
            )

        return self._repository.import_batch(
            CatalogImportBatch(
                id=_stable_id(
                    f"manifest:{manifest.source_name}:{manifest.revision}:"
                    f"{manifest.checksum}"
                ),
                manifest=manifest,
                category=category,
                templates=tuple(templates.values()),
                examples=tuple(examples),
                search_documents=tuple(searches.values()),
            )
        )
def _validate(
    manifest: ImportManifest,
    records: tuple[DiffusionRecord, ...],
) -> None:
    if manifest.item_count != len(records):
        raise InvalidManifest("item count does not match manifest")
    if not records or records_checksum(records) != manifest.checksum:
        raise InvalidManifest("checksum does not match manifest")
    for record in records:
        if not record.prompt.strip() or record.width <= 0 or record.height <= 0:
            raise InvalidManifest("record contains invalid content")
def _template(
    manifest: ImportManifest,
    record: DiffusionRecord,
    category_id: UUID,
    prompt: str,
    parameters: dict[str, int | float | str],
) -> PromptTemplate:
    content_hash = _content_hash(prompt, parameters)
    template_id = _stable_id(f"template:{content_hash}")
    divisor = math.gcd(record.width, record.height)
    return PromptTemplate(
        id=template_id,
        slug=f"diffusiondb-{content_hash[:24]}",
        title=prompt[:80],
        prompt=prompt,
        negative_prompt=None,
        source_model="stable-diffusion-1.x",
        aspect_ratio=f"{record.width // divisor}:{record.height // divisor}",
        parameters=parameters,
        category_id=category_id,
        source=SourceAttribution(
            name=manifest.source_name,
            url=manifest.source_url,
            object_id=record.image_name,
            revision=manifest.revision,
            license=manifest.license,
            collected_at=manifest.collected_at,
        ),
        content_hash=content_hash,
        status=TemplateStatus.SUPPRESSED,
        published_at=None,
    )
def _content_hash(
    prompt: str,
    parameters: dict[str, int | float | str],
) -> str:
    payload = json.dumps(
        {"parameters": parameters, "prompt": prompt},
        sort_keys=True,
        separators=(",", ":"),
    )
    return hashlib.sha256(payload.encode()).hexdigest()
def _stable_id(value: str) -> UUID:
    return uuid5(NAMESPACE_URL, f"thief:{value}")
