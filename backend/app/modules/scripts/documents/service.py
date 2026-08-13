import hashlib
import json
from datetime import UTC, datetime
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.config import Settings
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit import append_audit_event
from app.modules.media import MediaStorage, read_utf8_document_version
from app.modules.projects import (
    lock_active_project_for_content_write,
    project_for_content_read,
)
from app.modules.scripts.documents import repository
from app.modules.scripts.documents.analysis import DocumentAnalysis, analyze_document
from app.modules.scripts.documents.schemas import (
    DocumentRevisionResponse,
    FormatIssueResponse,
    NarrativeBlockResponse,
    PaginatedScriptDocuments,
    ScriptDocumentAnalysisResponse,
    ScriptDocumentImportRequest,
    ScriptDocumentResponse,
)
from app.modules.scripts.models import (
    DocumentRevision,
    FormatIssue,
    NarrativeBlock,
    ScriptDocument,
)


def _document_response(document: ScriptDocument) -> ScriptDocumentResponse:
    return ScriptDocumentResponse(
        id=document.id,
        workspace_id=document.workspace_id,
        project_id=document.project_id,
        title=document.title,
        source_type=cast(Literal["text", "media"], document.source_type),
        source_media_version_id=document.source_media_version_id,
        language=document.language,
        rights_declaration=document.rights_declaration,
        status=cast(Literal["active", "archived"], document.status),
        revision=document.revision,
        created_by=document.created_by,
        created_at=document.created_at,
    )


def _revision_response(revision: DocumentRevision) -> DocumentRevisionResponse:
    return DocumentRevisionResponse(
        id=revision.id,
        workspace_id=revision.workspace_id,
        document_id=revision.document_id,
        version_no=revision.version_no,
        source_type=cast(Literal["text", "media"], revision.source_type),
        source_media_version_id=revision.source_media_version_id,
        raw_text=revision.raw_text,
        raw_hash=revision.raw_hash,
        normalized_text=revision.normalized_text,
        normalized_hash=revision.normalized_hash,
        normalizer_version=revision.normalizer_version,
        normalization_map=revision.normalization_map,
        codepoint_count=revision.codepoint_count,
        analysis_status=cast(
            Literal["deterministic", "ai_candidate_required", "rejected"],
            revision.analysis_status,
        ),
        analyzer_version=revision.analyzer_version,
        created_by=revision.created_by,
        created_at=revision.created_at,
    )


def _block_response(block: NarrativeBlock) -> NarrativeBlockResponse:
    return NarrativeBlockResponse(
        id=block.id,
        document_revision_id=block.document_revision_id,
        position=block.position,
        kind=cast(
            Literal[
                "preamble",
                "episode_marker",
                "scene_heading",
                "dialogue",
                "narration",
                "action",
                "separator",
            ],
            block.kind,
        ),
        source_start=block.source_start,
        source_end=block.source_end,
        text_hash=block.text_hash,
        metadata=block.block_metadata,
    )


def _issue_response(issue: FormatIssue) -> FormatIssueResponse:
    return FormatIssueResponse(
        id=issue.id,
        document_revision_id=issue.document_revision_id,
        position=issue.position,
        code=issue.code,
        severity=cast(Literal["warning", "blocking"], issue.severity),
        source_start=issue.source_start,
        source_end=issue.source_end,
        line_number=issue.line_number,
        column_number=issue.column_number,
        next_action=issue.next_action,
        details=issue.issue_details,
    )


def _analysis_response(
    document: ScriptDocument,
    revision: DocumentRevision,
    blocks: list[NarrativeBlock],
    issues: list[FormatIssue],
) -> ScriptDocumentAnalysisResponse:
    return ScriptDocumentAnalysisResponse(
        document=_document_response(document),
        revision=_revision_response(revision),
        blocks=[_block_response(block) for block in blocks],
        issues=[_issue_response(issue) for issue in issues],
    )


def _input_hash(
    request: ScriptDocumentImportRequest,
    raw_hash: str,
) -> str:
    canonical = json.dumps(
        {
            "source_type": request.input_type,
            "source_media_version_id": (
                str(request.media_version_id)
                if request.media_version_id is not None
                else None
            ),
            "title": request.title,
            "language": request.language,
            "rights_declaration": request.rights_declaration,
            "raw_hash": raw_hash,
        },
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    )
    return hashlib.sha256(canonical.encode("utf-8")).hexdigest()


def _metadata_matches(
    document: ScriptDocument,
    request: ScriptDocumentImportRequest,
) -> bool:
    return (
        document.source_type == request.input_type
        and document.source_media_version_id == request.media_version_id
        and document.title == request.title
        and document.language == request.language
        and document.rights_declaration == request.rights_declaration
    )


def _require_analysis_invariants(text: str, analysis: DocumentAnalysis) -> None:
    reconstructed = "".join(
        text[block.start_codepoint : block.end_codepoint]
        for block in analysis.blocks
    )
    if reconstructed != text or analysis.content_hash != hashlib.sha256(
        text.encode("utf-8")
    ).hexdigest():
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Document analysis did not preserve the source",
            status_code=500,
        )


async def _stored_analysis(
    session: AsyncSession,
    document: ScriptDocument,
) -> ScriptDocumentAnalysisResponse:
    revision = await repository.find_initial_revision(session, document.id)
    if revision is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Document revision is unavailable",
            status_code=500,
        )
    blocks = await repository.list_blocks(session, revision.id)
    issues = await repository.list_issues(session, revision.id)
    return _analysis_response(document, revision, blocks, issues)


def _conflicting_idempotency() -> ApiError:
    return ApiError(
        ErrorCode.RESOURCE_CONFLICT,
        "Idempotency key was used with different input",
        status_code=409,
    )


async def import_document(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    request: ScriptDocumentImportRequest,
    storage: MediaStorage,
    settings: Settings,
    *,
    trace_id: str,
) -> ScriptDocumentAnalysisResponse:
    now = datetime.now(UTC)
    async with session.begin():
        project = await lock_active_project_for_content_write(
            session, claims, project_id
        )
        existing = await repository.find_document_by_idempotency(
            session, project.project_id, request.idempotency_key
        )
        if existing is not None:
            if not _metadata_matches(existing, request):
                raise _conflicting_idempotency()
            stored = await _stored_analysis(session, existing)
            if request.input_type == "text":
                assert request.text is not None
                raw_hash = hashlib.sha256(request.text.encode("utf-8")).hexdigest()
                if existing.input_hash != _input_hash(request, raw_hash):
                    raise _conflicting_idempotency()
            return stored

        if request.input_type == "text":
            assert request.text is not None
            raw_text = request.text
        else:
            assert request.media_version_id is not None
            raw_text = await read_utf8_document_version(
                session,
                project.workspace_id,
                request.media_version_id,
                storage,
                max_bytes=settings.script_document_max_bytes,
                max_codepoints=settings.script_document_max_codepoints,
            )
        raw_hash = hashlib.sha256(raw_text.encode("utf-8")).hexdigest()
        analysis = analyze_document(raw_text)
        _require_analysis_invariants(raw_text, analysis)
        input_hash = _input_hash(request, raw_hash)
        document_id = uuid7()
        inserted_id = await session.scalar(
            insert(ScriptDocument)
            .values(
                id=document_id,
                workspace_id=project.workspace_id,
                project_id=project.project_id,
                title=request.title,
                source_type=request.input_type,
                source_media_version_id=request.media_version_id,
                language=request.language,
                rights_declaration=request.rights_declaration,
                input_hash=input_hash,
                status="active",
                revision=1,
                idempotency_key=request.idempotency_key,
                created_by=claims.sub,
                created_at=now,
            )
            .on_conflict_do_nothing(
                constraint="uq_scr_document_project_idempotency"
            )
            .returning(ScriptDocument.id)
        )
        if inserted_id is None:
            existing = await repository.find_document_by_idempotency(
                session, project.project_id, request.idempotency_key
            )
            if (
                existing is None
                or not _metadata_matches(existing, request)
                or existing.input_hash != input_hash
            ):
                raise _conflicting_idempotency()
            return await _stored_analysis(session, existing)

        document = await repository.find_document(session, inserted_id)
        if document is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Document state is unavailable",
                status_code=500,
            )
        revision = DocumentRevision(
            id=uuid7(),
            workspace_id=project.workspace_id,
            document_id=document.id,
            version_no=1,
            source_type=request.input_type,
            source_media_version_id=request.media_version_id,
            raw_text=raw_text,
            raw_hash=raw_hash,
            normalized_text=raw_text,
            normalized_hash=raw_hash,
            normalizer_version="identity-v1",
            normalization_map={"type": "identity", "codepoint_count": len(raw_text)},
            codepoint_count=len(raw_text),
            analysis_status=analysis.status,
            analyzer_version=analysis.analyzer_version,
            created_by=claims.sub,
            created_at=now,
        )
        session.add(revision)
        await session.flush()
        blocks = [
            NarrativeBlock(
                id=uuid7(),
                workspace_id=project.workspace_id,
                document_revision_id=revision.id,
                position=block.position,
                kind=block.kind,
                source_start=block.start_codepoint,
                source_end=block.end_codepoint,
                text_hash=block.text_hash,
                block_metadata=block.metadata,
                created_at=now,
            )
            for block in analysis.blocks
        ]
        issues = [
            FormatIssue(
                id=uuid7(),
                workspace_id=project.workspace_id,
                document_revision_id=revision.id,
                position=position,
                code=issue.code,
                severity=issue.severity,
                source_start=issue.start_codepoint,
                source_end=issue.end_codepoint,
                line_number=issue.line_number,
                column_number=issue.column_number,
                next_action=issue.next_action,
                issue_details=issue.details,
                created_at=now,
            )
            for position, issue in enumerate(analysis.issues, start=1)
        ]
        session.add_all([*blocks, *issues])
        append_audit_event(
            session,
            workspace_id=project.workspace_id,
            actor_id=claims.sub,
            action="script.document_imported",
            target_type="document_revision",
            target_id=revision.id,
            trace_id=trace_id,
            metadata={
                "document_id": str(document.id),
                "project_id": str(project.project_id),
                "source_type": request.input_type,
                "analysis_status": analysis.status,
                "block_count": len(blocks),
                "issue_count": len(issues),
            },
            occurred_at=now,
        )
        await session.flush()
    return _analysis_response(document, revision, blocks, issues)


async def get_revision(
    session: AsyncSession,
    claims: AccessTokenClaims,
    revision_id: UUID,
) -> ScriptDocumentAnalysisResponse:
    found = await repository.find_revision_with_document(session, revision_id)
    if found is None:
        raise ApiError(
            ErrorCode.NOT_FOUND, "Document revision not found", status_code=404
        )
    revision, document = found
    try:
        await project_for_content_read(session, claims, document.project_id)
    except ApiError as error:
        if error.code in {ErrorCode.NOT_FOUND, ErrorCode.FORBIDDEN}:
            raise ApiError(
                ErrorCode.NOT_FOUND,
                "Document revision not found",
                status_code=404,
            ) from error
        raise
    blocks = await repository.list_blocks(session, revision.id)
    issues = await repository.list_issues(session, revision.id)
    return _analysis_response(document, revision, blocks, issues)


async def list_documents(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    *,
    limit: int,
    offset: int,
) -> PaginatedScriptDocuments:
    await project_for_content_read(session, claims, project_id)
    documents, total = await repository.list_documents(
        session, project_id, limit=limit, offset=offset
    )
    return PaginatedScriptDocuments(
        items=[_document_response(document) for document in documents],
        total=total,
        limit=limit,
        offset=offset,
    )
