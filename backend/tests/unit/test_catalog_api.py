from __future__ import annotations

import asyncio
import sys
import unittest
from datetime import UTC, datetime
from pathlib import Path
from uuid import UUID

import httpx


BACKEND = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(BACKEND / "src"))

from thief.catalog.model import (  # noqa: E402
    Category,
    GenerationExample,
    PromptTemplate,
    SourceAttribution,
    TemplateStatus,
)
from thief.catalog.query import (  # noqa: E402
    CatalogFilter,
    CatalogPage,
    CatalogTemplate,
    CategorySummary,
    InvalidCatalogQuery,
)


TEMPLATE_ID = UUID("00000000-0000-4000-8000-000000000101")
CATEGORY_ID = UUID("00000000-0000-4000-8000-000000000102")
ASSET_ID = UUID("00000000-0000-4000-8000-000000000103")
NOW = datetime(2026, 7, 23, 8, 0, tzinfo=UTC)


class FakeCatalogQueries:
    def __init__(self) -> None:
        self.filters: list[CatalogFilter] = []

    def list_templates(self, filters: CatalogFilter) -> CatalogPage:
        self.filters.append(filters)
        if filters.cursor == "broken":
            raise InvalidCatalogQuery("cursor is invalid")
        return CatalogPage(items=(_catalog_template(),), next_cursor="opaque-next")

    def get_template(self, template_id: UUID) -> CatalogTemplate | None:
        return _catalog_template() if template_id == TEMPLATE_ID else None

    def list_categories(self) -> tuple[CategorySummary, ...]:
        return (
            CategorySummary(
                category=Category(CATEGORY_ID, "portrait", "Portrait"),
                template_count=1,
            ),
        )


class CatalogApiTests(unittest.TestCase):
    def test_public_catalog_contract_and_filters(self) -> None:
        asyncio.run(self._assert_public_catalog_contract())

    async def _assert_public_catalog_contract(self) -> None:
        from thief.api.app import create_app

        catalog = FakeCatalogQueries()
        transport = httpx.ASGITransport(app=create_app(catalog=catalog))
        async with httpx.AsyncClient(transport=transport, base_url="https://test") as client:
            listed = await client.get(
                "/v1/templates",
                params={
                    "category": "portrait",
                    "model": "sdxl",
                    "aspect_ratio": "1:1",
                    "source": "fixture",
                    "cursor": "opaque",
                    "limit": 2,
                },
            )
            searched = await client.get("/v1/search", params={"q": "cinematic"})
            detail = await client.get(f"/v1/templates/{TEMPLATE_ID}")
            categories = await client.get("/v1/categories")

        self.assertEqual(listed.status_code, 200)
        self.assertEqual(listed.json()["next_cursor"], "opaque-next")
        self.assertEqual(
            listed.json()["items"][0],
            {
                "id": str(TEMPLATE_ID),
                "slug": "cinematic-portrait",
                "title": "Cinematic portrait",
                "prompt": "a cinematic portrait",
                "source_model": "sdxl",
                "aspect_ratio": "1:1",
                "source": {
                    "name": "fixture",
                    "url": "https://example.test/items/1",
                    "license": "test-fixture",
                },
            },
        )
        self.assertEqual(catalog.filters[0].category, "portrait")
        self.assertEqual(catalog.filters[0].limit, 2)
        self.assertEqual(catalog.filters[1].query, "cinematic")
        self.assertEqual(searched.status_code, 200)
        self.assertEqual(detail.status_code, 200)
        self.assertEqual(detail.json()["parameters"], {"seed": 42})
        self.assertEqual(detail.json()["source"]["revision"], "revision-1")
        self.assertEqual(detail.json()["examples"][0]["asset_id"], str(ASSET_ID))
        self.assertEqual(
            categories.json(),
            {
                "items": [
                    {
                        "id": str(CATEGORY_ID),
                        "slug": "portrait",
                        "name": "Portrait",
                        "template_count": 1,
                    }
                ]
            },
        )

    def test_invalid_queries_and_hidden_details_use_common_errors(self) -> None:
        asyncio.run(self._assert_errors())

    async def _assert_errors(self) -> None:
        from thief.api.app import create_app

        transport = httpx.ASGITransport(app=create_app(catalog=FakeCatalogQueries()))
        async with httpx.AsyncClient(transport=transport, base_url="https://test") as client:
            responses = (
                await client.get("/v1/templates", params={"limit": 51}),
                await client.get("/v1/templates", params={"cursor": "broken"}),
                await client.get("/v1/search", params={"q": "  "}),
            )
            missing = await client.get(
                "/v1/templates/00000000-0000-4000-8000-000000000999"
            )

        for response in responses:
            self.assertEqual(response.status_code, 400)
            self.assertEqual(response.json()["code"], "invalid_query")
            self.assertEqual(response.json()["details"], {})
            self.assertTrue(response.json()["trace_id"])
        self.assertEqual(missing.status_code, 404)
        self.assertEqual(missing.json()["code"], "template_not_found")


def _catalog_template() -> CatalogTemplate:
    template = PromptTemplate(
        id=TEMPLATE_ID,
        slug="cinematic-portrait",
        title="Cinematic portrait",
        prompt="a cinematic portrait",
        negative_prompt="blurry",
        source_model="sdxl",
        aspect_ratio="1:1",
        parameters={"seed": 42},
        category_id=CATEGORY_ID,
        source=SourceAttribution(
            "fixture",
            "https://example.test/items/1",
            "source-1",
            "revision-1",
            "test-fixture",
            NOW,
        ),
        content_hash="a" * 64,
        status=TemplateStatus.PUBLISHED,
        published_at=NOW,
    )
    example = GenerationExample(
        UUID("00000000-0000-4000-8000-000000000104"),
        TEMPLATE_ID,
        ASSET_ID,
        "Example",
        0,
    )
    return CatalogTemplate(template=template, examples=(example,))


if __name__ == "__main__":
    unittest.main()
