"use client";

import useSWR, { type Key, type SWRConfiguration, type SWRResponse } from "swr";
import { useSearchQuery, isSearchQuery } from "./use-search-query";
import type { RecoveryRequestConfig } from "./read-recovery";
import { revision, type OnlineProjection } from "./online-projections";

type Row = Record<string, unknown>;
export function matchesProjection(
  key: readonly unknown[],
  event: OnlineProjection,
) {
  const [name, , id] = key;
  switch (name) {
    case "credit-activity":
      return id === "personal"
        ? event.resourceType === "credit.account"
        : id === "admin" &&
            event.resourceType === "credit.activity" &&
            event.resourceId === "admin";
    case "my-credit-projects":
    case "my-credits":
    case "my-credit-ledger":
      return event.resourceType === "credit.account";

    case "public-asset-managed":
      return event.resourceType === "asset.review";
    case "public-assets":
      if (!isSearchQuery(key))
        return (
          event.resourceType === "asset.review" ||
          (event.resourceType === "project.directory" &&
            event.payload.resourceType === "public-asset")
        );
      return (
        event.resourceType === "search.scope" &&
        event.payload.resourceType === "public-asset"
      );
    case "asset-review":
      return (
        event.resourceType === "asset.review" &&
        String(event.payload.assetId) === String(id)
      );
    case "asset-review-pending":
      return event.resourceType === "asset.review";
    case "my-assets":
      if (!isSearchQuery(key))
        return (
          event.resourceType === "asset.import" ||
          event.resourceType === "asset.review" ||
          (event.resourceType === "project.directory" &&
            event.payload.resourceType === "asset")
        );
      return (
        event.resourceType === "search.scope" &&
        (event.payload.resourceType === "asset" ||
          (event.payload.resourceType == null &&
            event.resourceId.endsWith("-asset")))
      );
    case "home-root-projects":
    case "personal-projects":
      if (!isSearchQuery(key))
        return (
          event.resourceType === "project.directory" &&
          event.payload.resourceType === "project"
        );
      return (
        event.resourceType === "search.scope" &&
        event.payload.resourceType === "project"
      );
    case "workspace-search":
      return (
        event.resourceType === "search.scope" &&
        ["asset", "project"].includes(String(event.payload.resourceType))
      );
    case "project-canvases":
      if (!isSearchQuery(key))
        return (
          event.resourceType === "canvas.directory" &&
          event.payload.resourceType === "canvas"
        );
      return (
        event.resourceType === "search.scope" &&
        event.payload.resourceType === "canvas"
      );
    case "canvas-version-history":
      return (
        event.resourceType === "canvas.directory" &&
        event.payload.resourceType === "canvas-snapshot" &&
        String(event.payload.canvasId) === String(key[3])
      );
    case "canvas-search-status":
      return (
        event.resourceType === "search.scope" &&
        (event.payload.resourceType === "canvas" ||
          (event.payload.resourceType === "canvas-snapshot" &&
            String(event.payload.canvasId) === String(id)))
      );
    case "canvas-workflows":
      return (
        event.resourceType === "workflow.run" &&
        String(event.payload.canvasId) === String(id)
      );
    case "canvas-generation":
      return event.resourceType === "canvas.source"
        ? String(event.payload.canvasId) === String(id)
        : event.resourceType === "generation.task";
    case "task-record-detail":
    case "generation-detail":
      return (
        event.resourceType === "generation.task" &&
        event.resourceId === String(id)
      );
    case "task-records":
      return event.resourceType === "generation.task";
    case "canvas-generation-history":
      return (
        event.resourceType === "generation.task" &&
        (id === "all" || event.payload.modality === id)
      );
    case "generation-history":
      return (
        event.resourceType === "generation.task" &&
        event.payload.modality === id
      );
    case "file-transfers":
      return event.resourceType === "file.transfer";
    case "asset-import-jobs":
      return event.resourceType === "asset.import";
    case "asset-import":
      return (
        event.resourceType === "asset.import" && event.resourceId === String(id)
      );
    case "asset-export-jobs":
      return event.resourceType === "asset.export";
    case "asset-export":
      return (
        event.resourceType === "asset.export" && event.resourceId === String(id)
      );
    case "notification-summary":
      return event.resourceType === "notification.mailbox";
    case "asset-preview":
      return (
        event.resourceType === "file.delivery" &&
        String(event.payload.fileId) === String(id) &&
        String(event.payload.fileVersion) === String(key[3])
      );
    case "generation-file":
      return (
        event.resourceType === "file.delivery" &&
        String(event.payload.fileId) === String(key[4]) &&
        String(event.payload.fileVersion) === String(key[5])
      );
    default:
      return false;
  }
}
const liveNames = new Set([
  "credit-activity",
  "my-credit-projects",
  "my-credits",
  "my-credit-ledger",
  "my-assets",
  "public-assets",
  "public-asset-managed",
  "asset-review",
  "asset-review-pending",
  "home-root-projects",
  "personal-projects",
  "workspace-search",
  "project-canvases",
  "canvas-version-history",
  "canvas-search-status",
  "canvas-workflows",
  "task-records",
  "task-record-detail",
  "canvas-generation",
  "generation-detail",
  "generation-history",
  "canvas-generation-history",
  "file-transfers",
  "asset-import-jobs",
  "asset-import",
  "asset-export-jobs",
  "asset-export",
  "notification-summary",
  "asset-preview",
  "generation-file",
]);
export function patchProjection(
  data: unknown,
  key: readonly unknown[],
  event: OnlineProjection,
): { data: unknown; read: boolean } {
  if (key[0] === "my-credits" && event.resourceType === "credit.account") {
    const previous = data as Row | undefined;
    if (
      previous &&
      revision(previous.version) >= revision(event.resourceVersion)
    )
      return { data, read: false };
    return {
      data: { ...previous, ...event.payload, version: event.resourceVersion },
      read: !previous,
    };
  }
  if (
    data == null ||
    event.resourceType === "credit.account" ||
    event.resourceType === "credit.activity" ||
    event.resourceType === "search.scope" ||
    event.resourceType === "asset.review" ||
    ["my-assets", "project-canvases", "canvas-version-history"].includes(
      String(key[0]),
    )
  )
    return { data, read: true };
  const name = key[0];
  if (event.resourceType === "workflow.run") {
    const rows = Array.isArray(data) ? data : [data];
    const old = rows.find(
      (row) => String((row as Row).runId) === event.resourceId,
    ) as Row | undefined;
    return {
      data,
      read: !old || revision(old.runSeq) < revision(event.resourceVersion),
    };
  }
  if (name === "asset-preview" || name === "generation-file") {
    // Once mounted, a valid grant survives unrelated status events and reconnects.
    return {
      data,
      read:
        (data as Row).status !== "READY" &&
        ["READY", "FAILED", "CANCELLED", "EXPIRED"].includes(
          String(event.payload.status),
        ),
    };
  }
  if (event.resourceType === "notification.mailbox") {
    const previous = data as Row;
    if (
      previous.dataEpoch === event.payload.dataEpoch &&
      revision(previous.mailboxVersion) >= revision(event.resourceVersion)
    )
      return { data, read: false };
    return { data: event.payload, read: false };
  }
  if (
    ["asset.import", "asset.export", "canvas.source"].includes(
      event.resourceType,
    )
  ) {
    const rows = Array.isArray(data) ? data : [data];
    const row = rows.find(
      (value) =>
        String(
          (value as Row)[
            event.resourceType === "canvas.source" ? "sourceId" : "jobId"
          ],
        ) === event.resourceId,
    ) as Row | undefined;
    return {
      data,
      read:
        !row || revision(row.syncRevision) < revision(event.resourceVersion),
    };
  }
  let found = false;
  let detail = false;
  let pendingCommand = false;
  function merge(row: Row): Row {
    const id =
      event.resourceType === "file.transfer"
        ? row.fileId
        : name === "canvas-generation"
          ? row.taskId
          : row.id;
    if (String(id) !== event.resourceId) {
      if (
        name === "canvas-generation" &&
        !row.taskId &&
        row.commandId &&
        row.commandId === event.payload.commandId
      )
        pendingCommand = true;
      return row;
    }
    found = true;
    const versionKey =
      name === "canvas-generation" ? "taskSyncRevision" : "syncRevision";
    if (revision(row[versionKey]) >= revision(event.resourceVersion))
      return row;
    if (
      event.resourceType === "generation.task" &&
      (event.payload.state === "SUCCEEDED" || event.payload.hasText)
    ) {
      detail = true;
      return row;
    }
    return {
      ...row,
      ...event.payload,
      [versionKey]: event.resourceVersion,
      ...(name === "canvas-generation" &&
      ["FAILED", "CANCELLED", "DISPATCH_FAILED"].includes(
        String(event.payload.state),
      )
        ? { pending: false }
        : {}),
    };
  }
  const next = Array.isArray(data)
    ? data.map(merge)
    : Array.isArray((data as Row).records)
      ? {
          ...(data as Row),
          records: ((data as Row).records as Row[]).map(merge),
        }
      : merge(data as Row);
  // A different canvas's task must not trigger a request for this canvas.
  return {
    data:
      Array.isArray(data) &&
      Array.isArray(next) &&
      data.every((row, index) => row === next[index])
        ? data
        : next,
    read: detail || pendingCommand || (!found && name !== "canvas-generation"),
  };
}

/** Preserve newer accepted rows if a request started before a push or another request. */
export function mergeProjectionSnapshot(
  incoming: unknown,
  previous: unknown,
  key: readonly unknown[],
): unknown {
  if (!incoming || !previous) return incoming;
  const match = (row: Row) =>
    String(row.runId ?? row.sourceId ?? row.fileId ?? row.jobId ?? row.id);
  const rows = Array.isArray(previous)
    ? previous
    : Array.isArray((previous as Row).records)
      ? ((previous as Row).records as Row[])
      : [previous];
  const merge = (row: Row) => {
    const old = rows.find((value) => match(value as Row) === match(row)) as
      Row | undefined;
    if (!old) return row;
    if (key[0] === "canvas-workflows")
      return revision(old.runSeq) > revision(row.runSeq) ? old : row;
    if (key[0] === "my-credits")
      return revision(old.version) > revision(row.version) ? old : row;
    if (key[0] === "notification-summary")
      return old.dataEpoch === row.dataEpoch &&
        revision(old.mailboxVersion) > revision(row.mailboxVersion)
        ? old
        : row;
    if (key[0] === "canvas-generation") {
      if (revision(old.syncRevision) > revision(row.syncRevision)) return old;
      if (revision(old.taskSyncRevision) > revision(row.taskSyncRevision))
        return {
          ...row,
          state: old.state,
          failureKind: old.failureKind,
          pending: old.pending,
          outputs: old.outputs,
          textOutput: old.textOutput,
          taskSyncRevision: old.taskSyncRevision,
        };
    }
    return revision(old.syncRevision) > revision(row.syncRevision) ? old : row;
  };
  return Array.isArray(incoming)
    ? incoming.map(merge)
    : Array.isArray((incoming as Row).records)
      ? {
          ...(incoming as Row),
          records: ((incoming as Row).records as Row[]).map(merge),
        }
      : merge(incoming as Row);
}

/** Same cache contract as SWR; business reads are driven by a concrete event or bounded recovery. */
export default function useLiveQuery<Data, QueryKey extends Key = Key>(
  key: QueryKey,
  fetcher:
    ((key: QueryKey, options: RecoveryRequestConfig) => Promise<Data>) | null,
  config: SWRConfiguration<Data> = {},
): SWRResponse<Data> {
  const array = Array.isArray(key) ? key : [];
  const live = liveNames.has(String(array[0]));
  const projected = useSearchQuery(
    live ? key : null,
    fetcher,
    config,
    matchesProjection,
    {
      strict: isSearchQuery(array),
      live,
      patch: patchProjection,
      merge: mergeProjectionSnapshot,
    },
  );
  const ordinary = useSWR(
    live ? null : key,
    fetcher ? (readKey: QueryKey) => fetcher(readKey, {}) : null,
    config,
  );
  return live ? projected : ordinary;
}
