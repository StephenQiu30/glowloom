from __future__ import annotations

from datetime import datetime
from uuid import UUID

from pydantic import BaseModel

from thief.catalog.query import CatalogPage, CatalogTemplate, CategorySummary


class SourceSummaryView(BaseModel):
    name: str
    url: str
    license: str


class TemplateSummaryView(BaseModel):
    id: UUID
    slug: str
    title: str
    prompt: str
    source_model: str
    aspect_ratio: str
    source: SourceSummaryView


class TemplatePageView(BaseModel):
    items: list[TemplateSummaryView]
    next_cursor: str | None


class SourceView(SourceSummaryView):
    object_id: str
    revision: str
    collected_at: datetime


class ExampleView(BaseModel):
    asset_id: UUID
    alt_text: str
    position: int


class TemplateDetailView(TemplateSummaryView):
    negative_prompt: str | None
    parameters: dict[str, int | float | str]
    source: SourceView
    content_hash: str
    examples: list[ExampleView]


class CategoryView(BaseModel):
    id: UUID
    slug: str
    name: str
    template_count: int


class CategoryPageView(BaseModel):
    items: list[CategoryView]


def page_view(page: CatalogPage) -> TemplatePageView:
    return TemplatePageView(
        items=[summary_view(item) for item in page.items],
        next_cursor=page.next_cursor,
    )


def summary_view(item: CatalogTemplate) -> TemplateSummaryView:
    template = item.template
    return TemplateSummaryView(
        id=template.id,
        slug=template.slug,
        title=template.title,
        prompt=template.prompt,
        source_model=template.source_model,
        aspect_ratio=template.aspect_ratio,
        source=SourceSummaryView(
            name=template.source.name,
            url=template.source.url,
            license=template.source.license,
        ),
    )


def detail_view(item: CatalogTemplate) -> TemplateDetailView:
    template = item.template
    summary = summary_view(item)
    return TemplateDetailView(
        **summary.model_dump(exclude={"source"}),
        negative_prompt=template.negative_prompt,
        parameters=template.parameters,
        source=SourceView(
            name=template.source.name,
            url=template.source.url,
            license=template.source.license,
            object_id=template.source.object_id,
            revision=template.source.revision,
            collected_at=template.source.collected_at,
        ),
        content_hash=template.content_hash,
        examples=[
            ExampleView(
                asset_id=example.asset_id,
                alt_text=example.alt_text,
                position=example.position,
            )
            for example in item.examples
        ],
    )


def category_page_view(items: tuple[CategorySummary, ...]) -> CategoryPageView:
    return CategoryPageView(
        items=[
            CategoryView(
                id=item.category.id,
                slug=item.category.slug,
                name=item.category.name,
                template_count=item.template_count,
            )
            for item in items
        ]
    )
