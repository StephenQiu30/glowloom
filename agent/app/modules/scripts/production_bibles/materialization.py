import json
from dataclasses import dataclass
from datetime import UTC, datetime
from hashlib import sha256
from typing import Any, Literal, cast
from uuid import UUID

from pydantic import ValidationError
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.assets import (
    Asset,
    AssetNameRevision,
    AssetState,
    AssetVersion,
    find_asset_state_by_key,
    latest_asset_version_number,
    parse_asset_spec,
    spec_to_json,
)
from app.modules.governance.audit import append_audit_event
from app.modules.projects import lock_active_project_for_content_write
from app.modules.scripts.documents import repository as document_repository
from app.modules.scripts.production_bibles import repository as bible_repository
from app.modules.scripts.production_bibles.models import (
    ProductionBible,
    ProductionBibleEntity,
    ProductionBibleEntityState,
)
from app.modules.scripts.production_bibles.schemas import (
    BibleEvidence,
    BibleReviewIssue,
    ProductionBibleConfirmRequest,
)

_BIBLE_VERSION_SOURCE = "production_bible_state"


@dataclass(frozen=True, slots=True)
class ProductionBibleMaterializationIssue:
    code: str
    scope: Literal["bible", "entity", "entity_state", "world_entry"]
    subject_key: str | None
    summary: str

    def as_dict(self) -> dict[str, str | None]:
        return {
            "code": self.code,
            "scope": self.scope,
            "subject_key": self.subject_key,
            "summary": self.summary,
        }


@dataclass(frozen=True, slots=True)
class ProductionBibleStateMaterializationPlan:
    source_state_id: UUID
    state_key: str
    action: Literal["create", "reuse"]
    asset_state_id: UUID | None
    asset_version_id: UUID | None


@dataclass(frozen=True, slots=True)
class ProductionBibleEntityMaterializationPlan:
    source_entity_id: UUID
    entity_key: str
    kind: str
    action: Literal["create", "link"]
    asset_id: UUID | None
    states: tuple[ProductionBibleStateMaterializationPlan, ...]


@dataclass(frozen=True, slots=True)
class ProductionBibleMaterializationPlan:
    bible_id: UUID
    status: str
    entities: tuple[ProductionBibleEntityMaterializationPlan, ...]
    issues: tuple[ProductionBibleMaterializationIssue, ...]

    @property
    def confirmable(self) -> bool:
        return not self.issues


@dataclass(frozen=True, slots=True)
class ProductionBibleStateBinding:
    asset_state_id: UUID
    asset_version_id: UUID


@dataclass(frozen=True, slots=True)
class ProductionBibleMaterializationResult:
    bible_id: UUID
    status: Literal["confirmed"]
    revision: int
    entity_asset_ids: dict[str, UUID]
    state_bindings: dict[str, ProductionBibleStateBinding]
    replayed: bool


@dataclass(slots=True)
class _PreparedState:
    row: ProductionBibleEntityState
    spec: dict[str, Any]
    action: Literal["create", "reuse"]
    existing_state: AssetState | None = None
    existing_version: AssetVersion | None = None


@dataclass(slots=True)
class _PreparedEntity:
    row: ProductionBibleEntity
    asset: Asset | None
    states: list[_PreparedState]


@dataclass(slots=True)
class _PreparedMaterialization:
    bible: ProductionBible
    entities: list[_PreparedEntity]
    issues: list[ProductionBibleMaterializationIssue]

    def public_plan(self) -> ProductionBibleMaterializationPlan:
        return ProductionBibleMaterializationPlan(
            bible_id=self.bible.id,
            status=self.bible.status,
            entities=tuple(
                ProductionBibleEntityMaterializationPlan(
                    source_entity_id=entity.row.id,
                    entity_key=entity.row.entity_key,
                    kind=entity.row.kind,
                    action="link" if entity.asset is not None else "create",
                    asset_id=entity.asset.id if entity.asset is not None else None,
                    states=tuple(
                        ProductionBibleStateMaterializationPlan(
                            source_state_id=state.row.id,
                            state_key=state.row.state_key,
                            action=state.action,
                            asset_state_id=(
                                state.existing_state.id
                                if state.existing_state is not None
                                else None
                            ),
                            asset_version_id=(
                                state.existing_version.id
                                if state.existing_version is not None
                                else None
                            ),
                        )
                        for state in entity.states
                    ),
                )
                for entity in self.entities
            ),
            issues=tuple(self.issues),
        )


def _normalize_identity(value: str) -> str:
    return " ".join(value.strip().casefold().split())


def _clean_aliases(values: list[str]) -> list[str]:
    cleaned: list[str] = []
    normalized_seen: set[str] = set()
    for value in values:
        item = value.strip()
        normalized = _normalize_identity(item)
        if item and normalized not in normalized_seen:
            normalized_seen.add(normalized)
            cleaned.append(item)
    return cleaned


def _command_hash(bible_id: UUID, request: ProductionBibleConfirmRequest) -> str:
    payload = {
        "bible_id": str(bible_id),
        "expected_revision": request.expected_revision,
        "expected_result_hash": request.expected_result_hash,
    }
    return sha256(json.dumps(payload, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def _version_hash(spec: dict[str, Any], source_id: UUID) -> str:
    payload: dict[str, object] = {
        "schema_version": 1,
        "spec": spec,
        "prompt_description": "",
        "media_references": [],
        "source_type": _BIBLE_VERSION_SOURCE,
        "source_id": str(source_id),
    }
    return sha256(
        json.dumps(
            payload,
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode()
    ).hexdigest()


def _issue(
    issues: list[ProductionBibleMaterializationIssue],
    code: str,
    scope: Literal["bible", "entity", "entity_state", "world_entry"],
    subject_key: str | None,
    summary: str,
) -> None:
    issues.append(
        ProductionBibleMaterializationIssue(
            code=code,
            scope=scope,
            subject_key=subject_key,
            summary=summary,
        )
    )


def _validate_evidence(
    raw_items: list[dict[str, Any]],
    *,
    source_text: str | None,
    issues: list[ProductionBibleMaterializationIssue],
    scope: Literal["entity", "entity_state", "world_entry"],
    subject_key: str,
) -> None:
    if not raw_items:
        _issue(
            issues,
            "missing_source_evidence",
            scope,
            subject_key,
            "Materialized Bible facts require at least one exact source range.",
        )
        return
    if source_text is None:
        return
    for raw_item in raw_items:
        try:
            evidence = BibleEvidence.model_validate(raw_item)
        except ValidationError:
            _issue(
                issues,
                "invalid_source_evidence",
                scope,
                subject_key,
                "Bible evidence does not satisfy the exact-range contract.",
            )
            continue
        if evidence.source_end > len(source_text) or (
            source_text[evidence.source_start : evidence.source_end] != evidence.exact_anchor
        ):
            _issue(
                issues,
                "source_evidence_mismatch",
                scope,
                subject_key,
                "Bible evidence no longer matches the immutable document revision.",
            )


def _state_spec(
    entity: ProductionBibleEntity,
    state: ProductionBibleEntityState,
) -> dict[str, Any]:
    payload: dict[str, object] = {}
    for layer in (entity.stable_spec, state.state_spec):
        declared_kind = layer.get("kind")
        if declared_kind is not None and declared_kind != entity.kind:
            raise ValueError("Bible asset spec kind does not match the entity kind")
        payload.update(layer)
    payload["kind"] = entity.kind
    return spec_to_json(parse_asset_spec(entity.kind, payload))


def _identity_tokens(entity: ProductionBibleEntity) -> set[str]:
    return {
        normalized
        for normalized in (
            entity.normalized_name,
            _normalize_identity(entity.canonical_name),
            *(_normalize_identity(alias) for alias in entity.aliases),
        )
        if normalized
    }


def _asset_identity_tokens(asset: Asset) -> set[str]:
    return {
        normalized
        for normalized in (
            asset.normalized_name,
            _normalize_identity(asset.name),
            *(_normalize_identity(alias) for alias in asset.aliases),
        )
        if normalized
    }


async def _existing_assets(
    session: AsyncSession,
    project_id: UUID,
    *,
    for_update: bool,
) -> list[Asset]:
    query = select(Asset).where(Asset.project_id == project_id).order_by(Asset.id)
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return list(await session.scalars(query))


async def _existing_states(
    session: AsyncSession,
    asset_ids: list[UUID],
    *,
    for_update: bool,
) -> list[AssetState]:
    if not asset_ids:
        return []
    query = (
        select(AssetState)
        .where(AssetState.asset_id.in_(asset_ids))
        .order_by(
            AssetState.asset_id,
            AssetState.state_key,
        )
    )
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return list(await session.scalars(query))


async def _existing_versions(
    session: AsyncSession,
    version_ids: list[UUID],
    *,
    for_update: bool,
) -> list[AssetVersion]:
    if not version_ids:
        return []
    query = select(AssetVersion).where(AssetVersion.id.in_(version_ids)).order_by(AssetVersion.id)
    if for_update:
        query = query.with_for_update().execution_options(populate_existing=True)
    return list(await session.scalars(query))


async def _prepare_materialization(
    session: AsyncSession,
    bible: ProductionBible,
    *,
    for_update: bool,
) -> _PreparedMaterialization:
    issues: list[ProductionBibleMaterializationIssue] = []
    if bible.status != "needs_review":
        _issue(
            issues,
            "bible_not_reviewable",
            "bible",
            None,
            "Only a reviewed Production Bible candidate can be materialized.",
        )
    if bible.result_hash is None:
        _issue(
            issues,
            "missing_bible_result",
            "bible",
            None,
            "Production Bible provider output is unavailable.",
        )

    for raw_issue in bible.review_issues:
        try:
            review_issue = BibleReviewIssue.model_validate(raw_issue)
        except ValidationError:
            _issue(
                issues,
                "invalid_review_issue",
                "bible",
                None,
                "Production Bible review state is invalid.",
            )
            continue
        if review_issue.severity == "blocking":
            _issue(
                issues,
                "blocking_review_issue",
                "bible",
                review_issue.subject_key,
                "A blocking Production Bible review issue must be resolved first.",
            )

    current = await bible_repository.find_current_confirmed_bible(
        session,
        bible.project_id,
        for_update=for_update,
    )
    if current is not None and current.id != bible.id:
        _issue(
            issues,
            "confirmed_bible_already_exists",
            "bible",
            None,
            "The project already has a confirmed Production Bible; "
            "explicit revision linking is required.",
        )

    source_text: str | None = None
    found_revision = await document_repository.find_revision_with_document(
        session,
        bible.document_revision_id,
    )
    if found_revision is None:
        _issue(
            issues,
            "document_revision_unavailable",
            "bible",
            None,
            "The immutable document revision is unavailable.",
        )
    else:
        revision, document = found_revision
        if (
            revision.workspace_id != bible.workspace_id
            or document.project_id != bible.project_id
            or revision.normalized_hash != bible.input_hash
            or sha256(revision.normalized_text.encode()).hexdigest() != bible.input_hash
        ):
            _issue(
                issues,
                "document_revision_mismatch",
                "bible",
                None,
                "The Production Bible no longer matches its immutable document revision.",
            )
        else:
            source_text = revision.normalized_text

    entities = await bible_repository.list_entities(
        session,
        bible.id,
        for_update=for_update,
    )
    states = await bible_repository.list_entity_states(
        session,
        bible.id,
        for_update=for_update,
    )
    world_entries = await bible_repository.list_world_entries(
        session,
        bible.id,
        for_update=for_update,
    )
    states_by_entity: dict[UUID, list[ProductionBibleEntityState]] = {}
    for state in states:
        states_by_entity.setdefault(state.entity_id, []).append(state)

    existing_assets = await _existing_assets(
        session,
        bible.project_id,
        for_update=for_update,
    )
    existing_by_id = {asset.id: asset for asset in existing_assets}
    existing_states = await _existing_states(
        session,
        [asset.id for asset in existing_assets],
        for_update=for_update,
    )
    existing_state_by_id = {state.id: state for state in existing_states}
    existing_state_by_key = {(state.asset_id, state.state_key): state for state in existing_states}
    mapped_version_ids = [
        state.asset_version_id for state in states if state.asset_version_id is not None
    ]
    existing_versions = await _existing_versions(
        session,
        mapped_version_ids,
        for_update=for_update,
    )
    existing_version_by_id = {version.id: version for version in existing_versions}

    existing_identity_index: dict[tuple[str, str], list[Asset]] = {}
    for asset in existing_assets:
        for token in _asset_identity_tokens(asset):
            existing_identity_index.setdefault((asset.kind, token), []).append(asset)

    bible_identity_owner: dict[tuple[str, str], str] = {}
    used_asset_ids: dict[UUID, str] = {}
    prepared_entities: list[_PreparedEntity] = []
    known_entity_keys = {entity.entity_key for entity in entities}

    for entity in entities:
        _validate_evidence(
            entity.evidence,
            source_text=source_text,
            issues=issues,
            scope="entity",
            subject_key=entity.entity_key,
        )
        if entity.normalized_name != _normalize_identity(entity.canonical_name):
            _issue(
                issues,
                "invalid_normalized_name",
                "entity",
                entity.entity_key,
                "Entity normalized_name must match its canonical name.",
            )
        tokens = _identity_tokens(entity)
        for token in tokens:
            owner = bible_identity_owner.get((entity.kind, token))
            if owner is not None and owner != entity.entity_key:
                _issue(
                    issues,
                    "identity_token_collision",
                    "entity",
                    entity.entity_key,
                    "Two same-kind Bible entities share a canonical name or alias.",
                )
            else:
                bible_identity_owner[(entity.kind, token)] = entity.entity_key

        matches = {
            asset.id: asset
            for token in tokens
            for asset in existing_identity_index.get((entity.kind, token), [])
        }
        linked_asset: Asset | None = None
        if entity.asset_id is not None:
            linked_asset = existing_by_id.get(entity.asset_id)
            if (
                linked_asset is None
                or linked_asset.workspace_id != bible.workspace_id
                or linked_asset.project_id != bible.project_id
                or linked_asset.kind != entity.kind
            ):
                _issue(
                    issues,
                    "invalid_explicit_asset_link",
                    "entity",
                    entity.entity_key,
                    "The explicit asset link is outside the Bible entity scope or kind.",
                )
                linked_asset = None
            elif linked_asset.status != "active" or linked_asset.availability != "enabled":
                _issue(
                    issues,
                    "linked_asset_unavailable",
                    "entity",
                    entity.entity_key,
                    "The explicitly linked asset must be active and enabled.",
                )
            other_matches = set(matches) - {entity.asset_id}
            if other_matches:
                _issue(
                    issues,
                    "existing_identity_collision",
                    "entity",
                    entity.entity_key,
                    "More than one existing asset matches this confirmed identity.",
                )
            previous_owner = used_asset_ids.get(entity.asset_id)
            if previous_owner is not None and previous_owner != entity.entity_key:
                _issue(
                    issues,
                    "asset_link_reused",
                    "entity",
                    entity.entity_key,
                    "One asset cannot represent two Production Bible identities.",
                )
            else:
                used_asset_ids[entity.asset_id] = entity.entity_key
        elif matches:
            _issue(
                issues,
                "existing_identity_requires_explicit_link",
                "entity",
                entity.entity_key,
                "An existing same-kind name or alias match must be explicitly linked "
                "before confirmation.",
            )

        prepared_states: list[_PreparedState] = []
        entity_states = sorted(
            states_by_entity.get(entity.id, []),
            key=lambda item: (item.state_key != "base", item.state_key, item.id),
        )
        if entity.asset_id is None and not any(
            state.state_key == "base" for state in entity_states
        ):
            _issue(
                issues,
                "entity_missing_state",
                "entity",
                entity.entity_key,
                "A new Bible entity requires an evidence-backed base state and version.",
            )
        if (
            linked_asset is not None
            and (
                linked_asset.id,
                "base",
            )
            not in existing_state_by_key
        ):
            _issue(
                issues,
                "linked_asset_missing_base_state",
                "entity",
                entity.entity_key,
                "The explicitly linked asset has no base state.",
            )
        for state in entity_states:
            state_subject = f"{entity.entity_key}:{state.state_key}"
            _validate_evidence(
                state.evidence,
                source_text=source_text,
                issues=issues,
                scope="entity_state",
                subject_key=state_subject,
            )
            try:
                state_spec = _state_spec(entity, state)
            except (KeyError, TypeError, ValueError, ValidationError):
                _issue(
                    issues,
                    "invalid_asset_state_spec",
                    "entity_state",
                    state_subject,
                    "The merged stable and state facts do not satisfy the asset kind schema.",
                )
                state_spec = {"kind": entity.kind}

            for reference_field in ("holder_character_id", "wearer_character_id"):
                reference = state_spec.get(reference_field)
                if reference is None:
                    continue
                try:
                    referenced_asset = existing_by_id.get(UUID(str(reference)))
                except ValueError:
                    referenced_asset = None
                if (
                    referenced_asset is None
                    or referenced_asset.workspace_id != bible.workspace_id
                    or referenced_asset.project_id != bible.project_id
                    or referenced_asset.kind != "character"
                ):
                    _issue(
                        issues,
                        "invalid_related_asset_reference",
                        "entity_state",
                        state_subject,
                        "Related character references must resolve to an existing "
                        "project character asset.",
                    )

            if state.asset_state_id is not None and state.asset_version_id is not None:
                mapped_state = existing_state_by_id.get(state.asset_state_id)
                mapped_version = existing_version_by_id.get(state.asset_version_id)
                if (
                    linked_asset is None
                    or mapped_state is None
                    or mapped_version is None
                    or mapped_state.asset_id != linked_asset.id
                    or mapped_state.state_key != state.state_key
                    or mapped_state.current_version_id != mapped_version.id
                    or mapped_version.asset_state_id != mapped_state.id
                    or mapped_version.asset_id != linked_asset.id
                    or mapped_version.source_type != _BIBLE_VERSION_SOURCE
                    or mapped_version.source_id != state.id
                    or mapped_version.spec != state_spec
                ):
                    _issue(
                        issues,
                        "invalid_existing_state_mapping",
                        "entity_state",
                        state_subject,
                        "The existing state/version mapping does not match this Bible "
                        "state and source.",
                    )
                prepared_states.append(
                    _PreparedState(
                        row=state,
                        spec=state_spec,
                        action="reuse",
                        existing_state=mapped_state,
                        existing_version=mapped_version,
                    )
                )
                continue
            if state.asset_state_id is not None or state.asset_version_id is not None:
                _issue(
                    issues,
                    "partial_state_mapping",
                    "entity_state",
                    state_subject,
                    "A Bible state must link both its asset state and version or neither.",
                )
            if entity.asset_id is None and (
                state.asset_state_id is not None or state.asset_version_id is not None
            ):
                _issue(
                    issues,
                    "state_mapping_without_asset",
                    "entity_state",
                    state_subject,
                    "A Bible state cannot be mapped before its entity asset is mapped.",
                )
            if (
                linked_asset is not None
                and (
                    linked_asset.id,
                    state.state_key,
                )
                in existing_state_by_key
            ):
                _issue(
                    issues,
                    "existing_state_requires_explicit_link",
                    "entity_state",
                    state_subject,
                    "An existing state key must be explicitly mapped before Bible confirmation.",
                )
            prepared_states.append(_PreparedState(row=state, spec=state_spec, action="create"))
        prepared_entities.append(
            _PreparedEntity(row=entity, asset=linked_asset, states=prepared_states)
        )

    for entry in world_entries:
        _validate_evidence(
            entry.evidence,
            source_text=source_text,
            issues=issues,
            scope="world_entry",
            subject_key=entry.entry_key,
        )
        if not entry.facts and not entry.rules:
            _issue(
                issues,
                "empty_world_entry",
                "world_entry",
                entry.entry_key,
                "A confirmed world entry requires at least one fact or rule.",
            )
        if any(entity_key not in known_entity_keys for entity_key in entry.entity_keys):
            _issue(
                issues,
                "unknown_world_entity_reference",
                "world_entry",
                entry.entry_key,
                "A world entry references an unknown Bible entity.",
            )

    unique_issues = list(
        {
            (issue.code, issue.scope, issue.subject_key, issue.summary): issue for issue in issues
        }.values()
    )
    unique_issues.sort(
        key=lambda issue: (
            issue.scope,
            issue.subject_key or "",
            issue.code,
            issue.summary,
        )
    )
    return _PreparedMaterialization(
        bible=bible,
        entities=prepared_entities,
        issues=unique_issues,
    )


async def plan_production_bible_materialization(
    session: AsyncSession,
    bible_id: UUID,
) -> ProductionBibleMaterializationPlan:
    bible = await bible_repository.find_bible(session, bible_id)
    if bible is None:
        raise ApiError(ErrorCode.NOT_FOUND, "Production Bible not found", status_code=404)
    return (await _prepare_materialization(session, bible, for_update=False)).public_plan()


def _snapshot_result(
    bible: ProductionBible,
    entity_asset_ids: dict[str, UUID],
    state_bindings: dict[str, ProductionBibleStateBinding],
) -> dict[str, object]:
    return {
        "bible_id": str(bible.id),
        "status": "confirmed",
        "revision": bible.revision,
        "entity_asset_ids": {key: str(value) for key, value in sorted(entity_asset_ids.items())},
        "state_bindings": {
            key: {
                "asset_state_id": str(value.asset_state_id),
                "asset_version_id": str(value.asset_version_id),
            }
            for key, value in sorted(state_bindings.items())
        },
    }


def _result_from_snapshot(
    bible: ProductionBible,
    *,
    replayed: bool,
) -> ProductionBibleMaterializationResult:
    try:
        snapshot = bible.confirm_result
        if snapshot.get("bible_id") != str(bible.id) or snapshot.get("status") != "confirmed":
            raise ValueError("invalid confirmation snapshot")
        raw_entities = cast(dict[str, object], snapshot["entity_asset_ids"])
        raw_states = cast(dict[str, object], snapshot["state_bindings"])
        entity_asset_ids = {key: UUID(str(value)) for key, value in raw_entities.items()}
        state_bindings = {
            key: ProductionBibleStateBinding(
                asset_state_id=UUID(str(cast(dict[str, object], value)["asset_state_id"])),
                asset_version_id=UUID(str(cast(dict[str, object], value)["asset_version_id"])),
            )
            for key, value in raw_states.items()
        }
    except (KeyError, TypeError, ValueError) as error:
        raise ApiError(
            ErrorCode.DEPENDENCY_UNAVAILABLE,
            "Production Bible confirmation result is unavailable",
            status_code=503,
        ) from error
    return ProductionBibleMaterializationResult(
        bible_id=bible.id,
        status="confirmed",
        revision=bible.revision,
        entity_asset_ids=entity_asset_ids,
        state_bindings=state_bindings,
        replayed=replayed,
    )


async def confirm_and_materialize_production_bible(
    session: AsyncSession,
    claims: AccessTokenClaims,
    bible_id: UUID,
    request: ProductionBibleConfirmRequest,
    *,
    trace_id: str,
) -> ProductionBibleMaterializationResult:
    command_hash = _command_hash(bible_id, request)
    now = datetime.now(UTC)
    async with session.begin():
        bible = await bible_repository.find_bible(session, bible_id, for_update=True)
        if bible is None:
            raise ApiError(ErrorCode.NOT_FOUND, "Production Bible not found", status_code=404)
        project = await lock_active_project_for_content_write(
            session,
            claims,
            bible.project_id,
        )
        if project.workspace_id != bible.workspace_id:
            raise ApiError(ErrorCode.NOT_FOUND, "Production Bible not found", status_code=404)

        receipt_owner = await bible_repository.find_bible_by_confirm_idempotency(
            session,
            bible.workspace_id,
            request.idempotency_key,
            for_update=True,
        )
        if receipt_owner is not None:
            if (
                receipt_owner.id != bible.id
                or receipt_owner.confirm_command_hash != command_hash
                or receipt_owner.status != "confirmed"
            ):
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Production Bible confirmation key was used with different input",
                    status_code=409,
                )
            return _result_from_snapshot(receipt_owner, replayed=True)

        if bible.confirm_idempotency_key is not None:
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Production Bible was already confirmed with a different command",
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

        prepared = await _prepare_materialization(session, bible, for_update=True)
        plan = prepared.public_plan()
        if not plan.confirmable:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Production Bible cannot be materialized",
                status_code=409,
                next_action="resolve_production_bible_issues",
                details={"issues": [issue.as_dict() for issue in plan.issues]},
            )

        entity_asset_ids: dict[str, UUID] = {}
        state_bindings: dict[str, ProductionBibleStateBinding] = {}
        for prepared_entity in prepared.entities:
            entity = prepared_entity.row
            asset = prepared_entity.asset
            base_state: AssetState | None = None
            if asset is None:
                asset = Asset(
                    id=uuid7(),
                    workspace_id=bible.workspace_id,
                    project_id=bible.project_id,
                    kind=entity.kind,
                    name=entity.canonical_name.strip(),
                    normalized_name=entity.normalized_name,
                    aliases=_clean_aliases(list(entity.aliases)),
                    tags=["production_bible"],
                    status="active",
                    availability="enabled",
                    name_revision=1,
                    revision=1,
                    command_receipts={},
                    created_by=claims.sub,
                    created_at=now,
                    updated_at=now,
                )
                base_state = AssetState(
                    id=uuid7(),
                    workspace_id=bible.workspace_id,
                    asset_id=asset.id,
                    state_key="base",
                    label="基础状态",
                    description="",
                    status="active",
                    current_version_id=None,
                    revision=1,
                    creation_key="base",
                    command_receipts={},
                    created_by=claims.sub,
                    created_at=now,
                    updated_at=now,
                )
                session.add(asset)
                session.add(
                    AssetNameRevision(
                        asset_id=asset.id,
                        revision_no=1,
                        workspace_id=bible.workspace_id,
                        name_snapshot=asset.name,
                        normalized_name=asset.normalized_name,
                        created_by=claims.sub,
                        created_at=now,
                    )
                )
                session.add(base_state)
                append_audit_event(
                    session,
                    workspace_id=bible.workspace_id,
                    actor_id=claims.sub,
                    action="asset.created",
                    target_type="asset",
                    target_id=asset.id,
                    trace_id=trace_id,
                    metadata={
                        "revision": 1,
                        "kind": asset.kind,
                        "project_id": str(bible.project_id),
                    },
                    occurred_at=now,
                )
                await session.flush([asset, base_state])
            else:
                base_state = await find_asset_state_by_key(
                    session,
                    asset.id,
                    "base",
                )

            entity.asset_id = asset.id
            entity.updated_at = now
            entity_asset_ids[entity.entity_key] = asset.id
            next_version = await latest_asset_version_number(session, asset.id)

            for prepared_state in prepared_entity.states:
                bible_state = prepared_state.row
                binding_key = f"{entity.entity_key}:{bible_state.state_key}"
                if prepared_state.action == "reuse":
                    if (
                        prepared_state.existing_state is None
                        or prepared_state.existing_version is None
                    ):
                        raise ApiError(
                            ErrorCode.DEPENDENCY_UNAVAILABLE,
                            "Production Bible state mapping is unavailable",
                            status_code=503,
                        )
                    state = prepared_state.existing_state
                    version = prepared_state.existing_version
                else:
                    if bible_state.state_key == "base" and prepared_entity.asset is None:
                        if base_state is None:
                            raise ApiError(
                                ErrorCode.DEPENDENCY_UNAVAILABLE,
                                "Asset base state is unavailable",
                                status_code=503,
                            )
                        state = base_state
                        state.label = bible_state.label
                    else:
                        state = AssetState(
                            id=uuid7(),
                            workspace_id=bible.workspace_id,
                            asset_id=asset.id,
                            state_key=bible_state.state_key,
                            label=bible_state.label,
                            description="",
                            status="active",
                            current_version_id=None,
                            revision=1,
                            creation_key=f"production-bible-state:{bible_state.id}",
                            command_receipts={},
                            created_by=claims.sub,
                            created_at=now,
                            updated_at=now,
                        )
                        session.add(state)
                        asset.revision += 1
                        asset.updated_at = now
                        append_audit_event(
                            session,
                            workspace_id=bible.workspace_id,
                            actor_id=claims.sub,
                            action="asset.state_created",
                            target_type="asset_state",
                            target_id=state.id,
                            trace_id=trace_id,
                            metadata={
                                "asset_id": str(asset.id),
                                "asset_revision": asset.revision,
                                "state_key": state.state_key,
                            },
                            occurred_at=now,
                        )
                        await session.flush([state])

                    next_version += 1
                    previous_version_id = state.current_version_id
                    version = AssetVersion(
                        id=uuid7(),
                        workspace_id=bible.workspace_id,
                        asset_id=asset.id,
                        asset_state_id=state.id,
                        version_no=next_version,
                        schema_version=1,
                        spec=prepared_state.spec,
                        prompt_description="",
                        source_type=_BIBLE_VERSION_SOURCE,
                        source_id=bible_state.id,
                        content_hash=_version_hash(prepared_state.spec, bible_state.id),
                        created_by=claims.sub,
                        created_at=now,
                    )
                    session.add(version)
                    await session.flush([version])
                    state.current_version_id = version.id
                    state.revision += 1
                    state.updated_at = now
                    append_audit_event(
                        session,
                        workspace_id=bible.workspace_id,
                        actor_id=claims.sub,
                        action="asset.version_created",
                        target_type="asset_version",
                        target_id=version.id,
                        trace_id=trace_id,
                        metadata={
                            "asset_id": str(asset.id),
                            "asset_state_id": str(state.id),
                            "state_revision": state.revision,
                            "version_no": version.version_no,
                            "kind": asset.kind,
                            "set_as_current": True,
                            "previous_version_id": (
                                str(previous_version_id)
                                if previous_version_id is not None
                                else None
                            ),
                            "current_version_id": str(version.id),
                        },
                        occurred_at=now,
                    )

                bible_state.asset_state_id = state.id
                bible_state.asset_version_id = version.id
                bible_state.updated_at = now
                state_bindings[binding_key] = ProductionBibleStateBinding(
                    asset_state_id=state.id,
                    asset_version_id=version.id,
                )

        bible.status = "confirmed"
        bible.confirmed_at = now
        bible.confirmed_by = claims.sub
        bible.confirm_idempotency_key = request.idempotency_key
        bible.confirm_command_hash = command_hash
        bible.run_token = None
        bible.lease_expires_at = None
        bible.revision += 1
        bible.updated_at = now
        bible.confirm_result = _snapshot_result(
            bible,
            entity_asset_ids,
            state_bindings,
        )
        append_audit_event(
            session,
            workspace_id=bible.workspace_id,
            actor_id=claims.sub,
            action="script.production_bible_confirmed",
            target_type="production_bible",
            target_id=bible.id,
            trace_id=trace_id,
            metadata={
                "revision": bible.revision,
                "document_revision_id": str(bible.document_revision_id),
                "project_id": str(bible.project_id),
                "result_hash": bible.result_hash,
                "entity_count": len(prepared.entities),
                "state_count": sum(len(entity.states) for entity in prepared.entities),
            },
            occurred_at=now,
        )
        await session.flush()
        return ProductionBibleMaterializationResult(
            bible_id=bible.id,
            status="confirmed",
            revision=bible.revision,
            entity_asset_ids=entity_asset_ids,
            state_bindings=state_bindings,
            replayed=False,
        )
