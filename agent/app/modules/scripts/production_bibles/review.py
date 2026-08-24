import hashlib
import json
from datetime import UTC, datetime
from typing import cast
from uuid import UUID

from pydantic import ValidationError
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit import append_audit_event
from app.modules.projects import lock_active_project_for_content_write
from app.modules.scripts.production_bibles import repository
from app.modules.scripts.production_bibles.models import ProductionBible
from app.modules.scripts.production_bibles.schemas import (
    BibleReviewIssue,
    ProductionBibleReviewIssueResolutionRequest,
)


def _receipt_key(bible_id: UUID, idempotency_key: str) -> str:
    return hashlib.sha256(f"{bible_id}:{idempotency_key}".encode()).hexdigest()


def _command_hash(
    bible_id: UUID,
    request: ProductionBibleReviewIssueResolutionRequest,
) -> str:
    payload = {
        "bible_id": str(bible_id),
        **request.model_dump(mode="json", exclude={"idempotency_key"}),
    }
    return hashlib.sha256(
        json.dumps(
            payload,
            ensure_ascii=False,
            sort_keys=True,
            separators=(",", ":"),
        ).encode()
    ).hexdigest()


def _corrected_result_hash(
    previous_result_hash: str,
    request: ProductionBibleReviewIssueResolutionRequest,
) -> str:
    payload = {
        "previous_result_hash": previous_result_hash,
        "issue_key": request.issue_key,
        "resolution_note": request.resolution_note,
        "correction": request.correction.model_dump(mode="json"),
    }
    return hashlib.sha256(
        json.dumps(
            payload,
            ensure_ascii=False,
            sort_keys=True,
            separators=(",", ":"),
        ).encode()
    ).hexdigest()


def _validated_review_issues(bible: ProductionBible) -> list[BibleReviewIssue]:
    try:
        return [BibleReviewIssue.model_validate(item) for item in bible.review_issues]
    except ValidationError as error:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Production Bible review state is invalid",
            status_code=409,
            next_action="rerun_production_bible_review",
        ) from error


def _validate_replay(
    bible: ProductionBible,
    *,
    receipt_key: str,
    command_hash: str,
) -> bool:
    raw_receipt: object = bible.review_receipts.get(receipt_key)
    if raw_receipt is None:
        return False
    if not isinstance(raw_receipt, dict):
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Production Bible review receipt is invalid",
            status_code=409,
        )
    receipt = cast(dict[str, object], raw_receipt)
    if receipt.get("command_hash") != command_hash:
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Production Bible review resolution key was used with different input",
            status_code=409,
        )
    if (
        receipt.get("result_hash") is None
        or receipt.get("revision") is None
        or receipt.get("issue_key") is None
    ):
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Production Bible review receipt is invalid",
            status_code=409,
        )
    return True


async def resolve_production_bible_review_issue(
    session: AsyncSession,
    claims: AccessTokenClaims,
    bible_id: UUID,
    request: ProductionBibleReviewIssueResolutionRequest,
    *,
    trace_id: str,
) -> None:
    command_hash = _command_hash(bible_id, request)
    receipt_key = _receipt_key(bible_id, request.idempotency_key)
    now = datetime.now(UTC)
    async with session.begin():
        bible = await repository.find_bible(session, bible_id, for_update=True)
        if bible is None:
            raise ApiError(ErrorCode.NOT_FOUND, "Production Bible not found", status_code=404)
        project = await lock_active_project_for_content_write(
            session,
            claims,
            bible.project_id,
        )
        if project.workspace_id != bible.workspace_id:
            raise ApiError(ErrorCode.NOT_FOUND, "Production Bible not found", status_code=404)

        if _validate_replay(
            bible,
            receipt_key=receipt_key,
            command_hash=command_hash,
        ):
            return
        if bible.status != "needs_review":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Only a reviewable Production Bible can be corrected",
                status_code=409,
            )
        if bible.revision != request.expected_revision:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Production Bible has changed",
                status_code=409,
                details={"current_revision": bible.revision},
            )
        if bible.result_hash != request.expected_result_hash:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Production Bible result has changed",
                status_code=409,
                details={"current_revision": bible.revision},
            )

        issues = _validated_review_issues(bible)
        issue = next((item for item in issues if item.issue_key == request.issue_key), None)
        if issue is None:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Production Bible review issue is unavailable",
                status_code=409,
            )

        correction = request.correction
        expected_subject_key = f"{correction.entity_key}/state:{correction.state_key}"
        if issue.scope != "entity_state" or issue.subject_key != expected_subject_key:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Review correction does not target the issue subject",
                status_code=409,
            )
        entity = await repository.find_entity_by_key(
            session,
            bible.id,
            correction.entity_key,
            for_update=True,
        )
        if entity is None:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Production Bible review entity is unavailable",
                status_code=409,
            )
        state = await repository.find_entity_state_by_key(
            session,
            entity.id,
            correction.state_key,
            for_update=True,
        )
        if state is None:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Production Bible review state is unavailable",
                status_code=409,
            )
        if entity.episode_numbers and not set(correction.episode_numbers).issubset(
            entity.episode_numbers
        ):
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "State episode numbers must remain within the entity episode range",
                status_code=409,
            )

        previous_episode_numbers = list(state.episode_numbers)
        previous_result_hash = request.expected_result_hash
        result_hash = _corrected_result_hash(previous_result_hash, request)
        state.episode_numbers = list(correction.episode_numbers)
        state.updated_at = now
        bible.review_issues = [
            item.model_dump(mode="json") for item in issues if item.issue_key != request.issue_key
        ]
        bible.result_hash = result_hash
        bible.revision += 1
        bible.updated_at = now
        bible.review_receipts = {
            **bible.review_receipts,
            receipt_key: {
                "command_hash": command_hash,
                "issue_key": request.issue_key,
                "revision": bible.revision,
                "result_hash": result_hash,
            },
        }
        append_audit_event(
            session,
            workspace_id=bible.workspace_id,
            actor_id=claims.sub,
            action="script.production_bible_review_issue_resolved",
            target_type="production_bible",
            target_id=bible.id,
            trace_id=trace_id,
            metadata={
                "issue_key": issue.issue_key,
                "issue_code": issue.code,
                "correction_kind": correction.kind,
                "entity_key": correction.entity_key,
                "state_key": correction.state_key,
                "previous_episode_numbers": previous_episode_numbers,
                "episode_numbers": list(correction.episode_numbers),
                "previous_result_hash": previous_result_hash,
                "result_hash": result_hash,
                "revision": bible.revision,
            },
            occurred_at=now,
        )
        await session.flush()
