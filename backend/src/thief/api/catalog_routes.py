from __future__ import annotations

from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Query
from fastapi.responses import JSONResponse

from thief.api.catalog_views import (
    CategoryPageView,
    TemplateDetailView,
    TemplatePageView,
    category_page_view,
    detail_view,
    page_view,
)
from thief.api.errors import error_response
from thief.catalog.query import (
    CatalogFilter,
    CatalogQueries,
    InvalidCatalogQuery,
)

Limit = Annotated[int, Query(ge=1, le=50)]


def catalog_router(catalog: CatalogQueries) -> APIRouter:
    router = APIRouter(prefix="/v1", tags=["catalog"])

    @router.get("/templates", response_model=TemplatePageView)
    def list_templates(
        category: str | None = None,
        model: str | None = None,
        aspect_ratio: str | None = None,
        source: str | None = None,
        cursor: str | None = None,
        limit: Limit = 20,
    ) -> TemplatePageView | JSONResponse:
        return _list(
            catalog,
            CatalogFilter(
                category=category,
                model=model,
                aspect_ratio=aspect_ratio,
                source=source,
                cursor=cursor,
                limit=limit,
            ),
        )

    @router.get("/search", response_model=TemplatePageView)
    def search_templates(
        q: str,
        category: str | None = None,
        model: str | None = None,
        aspect_ratio: str | None = None,
        source: str | None = None,
        cursor: str | None = None,
        limit: Limit = 20,
    ) -> TemplatePageView | JSONResponse:
        if not q.strip():
            return _invalid_query()
        return _list(
            catalog,
            CatalogFilter(
                query=q.strip(),
                category=category,
                model=model,
                aspect_ratio=aspect_ratio,
                source=source,
                cursor=cursor,
                limit=limit,
            ),
        )

    @router.get("/templates/{template_id}", response_model=TemplateDetailView)
    def get_template(template_id: UUID) -> TemplateDetailView | JSONResponse:
        result = catalog.get_template(template_id)
        if result is None:
            return error_response(404, "template_not_found", "Template was not found.")
        return detail_view(result)

    @router.get("/categories", response_model=CategoryPageView)
    def list_categories() -> CategoryPageView:
        return category_page_view(catalog.list_categories())

    return router


def _list(
    catalog: CatalogQueries,
    filters: CatalogFilter,
) -> TemplatePageView | JSONResponse:
    if any(value is not None and not value.strip() for value in _strings(filters)):
        return _invalid_query()
    try:
        page = catalog.list_templates(filters)
    except InvalidCatalogQuery:
        return _invalid_query()
    return page_view(page)


def _strings(filters: CatalogFilter) -> tuple[str | None, ...]:
    return (
        filters.category,
        filters.model,
        filters.aspect_ratio,
        filters.source,
        filters.cursor,
    )
def _invalid_query() -> JSONResponse:
    return error_response(400, "invalid_query", "Catalog query is invalid.")
