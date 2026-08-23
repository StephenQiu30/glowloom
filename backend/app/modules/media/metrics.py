from typing import Literal, cast

from prometheus_client import Counter, Histogram

StorageProfileLabel = Literal["default", "unregistered"]
StorageOperationLabel = Literal[
    "ensure_bucket",
    "presign_upload",
    "presign_download",
    "stat",
    "put",
    "copy",
    "stream",
    "delete",
    "unregistered",
]
StorageResultLabel = Literal[
    "succeeded",
    "not_found",
    "access_denied",
    "unavailable",
    "rejected",
    "cancelled",
    "failed",
    "unregistered",
]
MediaKindLabel = Literal["image", "video", "audio", "subtitle", "document", "unregistered"]
MediaProbeResultLabel = Literal[
    "succeeded",
    "probe_timeout",
    "probe_tool_unavailable",
    "unsupported_media",
    "invalid_probe_output",
    "invalid_image_metadata",
    "unsupported_document_mime",
    "document_too_large",
    "document_not_utf8",
    "utf8_bom_not_allowed",
    "empty_document",
    "cancelled",
    "failed",
    "unregistered",
]

REGISTERED_STORAGE_OPERATIONS = frozenset(
    {
        "ensure_bucket",
        "presign_upload",
        "presign_download",
        "stat",
        "put",
        "copy",
        "stream",
        "delete",
    }
)
REGISTERED_STORAGE_RESULTS = frozenset(
    {
        "succeeded",
        "not_found",
        "access_denied",
        "unavailable",
        "rejected",
        "cancelled",
        "failed",
    }
)
REGISTERED_MEDIA_KINDS = frozenset({"image", "video", "audio", "subtitle", "document"})
REGISTERED_MEDIA_PROBE_RESULTS = frozenset(
    {
        "succeeded",
        "probe_timeout",
        "probe_tool_unavailable",
        "unsupported_media",
        "invalid_probe_output",
        "unsupported_document_mime",
        "document_too_large",
        "document_not_utf8",
        "utf8_bom_not_allowed",
        "empty_document",
        "invalid_image_metadata",
        "cancelled",
        "failed",
    }
)

STORAGE_OPERATIONS = Counter(
    "lanverse_storage_operations_total",
    "Object storage operation outcomes",
    ("storage_profile", "operation", "result"),
)
STORAGE_OPERATION_DURATION = Histogram(
    "lanverse_storage_operation_duration_seconds",
    "Object storage operation duration",
    ("storage_profile", "operation"),
)
STORAGE_BYTES = Counter(
    "lanverse_storage_bytes_total",
    "Bytes successfully written to or fully streamed from object storage",
    ("storage_profile", "operation"),
)
MEDIA_PROBE_RESULTS = Counter(
    "lanverse_media_probe_results_total",
    "Media probe outcomes",
    ("kind", "result"),
)
MEDIA_PROBE_DURATION = Histogram(
    "lanverse_media_probe_duration_seconds",
    "Media probe duration",
    ("kind",),
)


def storage_profile_label(value: str) -> StorageProfileLabel:
    return "default" if value == "default" else "unregistered"


def storage_operation_label(value: str) -> StorageOperationLabel:
    return cast(
        StorageOperationLabel,
        value if value in REGISTERED_STORAGE_OPERATIONS else "unregistered",
    )


def storage_result_label(value: str) -> StorageResultLabel:
    return cast(
        StorageResultLabel,
        value if value in REGISTERED_STORAGE_RESULTS else "unregistered",
    )


def media_kind_label(value: str) -> MediaKindLabel:
    return cast(
        MediaKindLabel,
        value if value in REGISTERED_MEDIA_KINDS else "unregistered",
    )


def media_probe_result_label(value: str) -> MediaProbeResultLabel:
    return cast(
        MediaProbeResultLabel,
        value if value in REGISTERED_MEDIA_PROBE_RESULTS else "unregistered",
    )


def observe_storage_operation(
    *,
    storage_profile: str,
    operation: str,
    result: str,
    duration_seconds: float,
    bytes_processed: int | None = None,
) -> None:
    profile_label = storage_profile_label(storage_profile)
    operation_label = storage_operation_label(operation)
    result_label = storage_result_label(result)
    try:
        STORAGE_OPERATIONS.labels(
            storage_profile=profile_label,
            operation=operation_label,
            result=result_label,
        ).inc()
    except Exception:
        pass
    try:
        STORAGE_OPERATION_DURATION.labels(
            storage_profile=profile_label,
            operation=operation_label,
        ).observe(max(duration_seconds, 0))
    except Exception:
        pass
    if (
        bytes_processed is not None
        and bytes_processed >= 0
        and result_label == "succeeded"
        and operation_label in {"put", "stream"}
    ):
        try:
            STORAGE_BYTES.labels(
                storage_profile=profile_label,
                operation=operation_label,
            ).inc(bytes_processed)
        except Exception:
            pass


def observe_media_probe(
    *,
    kind: str,
    result: str,
    duration_seconds: float,
) -> None:
    kind_label = media_kind_label(kind)
    result_label = media_probe_result_label(result)
    try:
        MEDIA_PROBE_RESULTS.labels(kind=kind_label, result=result_label).inc()
    except Exception:
        pass
    try:
        MEDIA_PROBE_DURATION.labels(kind=kind_label).observe(max(duration_seconds, 0))
    except Exception:
        pass
