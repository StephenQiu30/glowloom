"use client";

import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ApiError } from "@/lib/request";
import {
  clearSourceIntent,
  loadSourceIntent,
  saveSourceIntent,
  sourceIntentSchema,
  unknownSourceWrite,
  type ScriptScope,
  type SourceIntent,
} from "./source-intent";
import {
  sourceCommandSchema,
  type SourceCommand,
  type SourceReceipt,
} from "./source-model";
import {
  getWorkspace,
  requireScriptScope,
  runSourceIntent,
  scriptScopeKey,
  scriptWorkspaceKey,
} from "./source-queries";

export function useSourceWriter(
  scope: ScriptScope,
  onAccepted: (receipt: SourceReceipt) => void,
) {
  const { origin, actorId, orgId, projectId } = scope;
  const cache = useQueryClient();
  const [intent, setIntent] = useState<SourceIntent | null>(null);
  const pending = useRef<SourceIntent | null>(null);
  const operation = useRef(false);
  const conflict = useRef(false);
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [storageError, setStorageError] = useState<string | undefined>();
  const [error, setError] = useState<string | undefined>();
  const [rejected, setRejected] = useState<{
    intent: SourceIntent;
    error: unknown;
  } | null>(null);
  function remember(value: SourceIntent | null) {
    pending.current = value;
    setIntent(value);
  }
  useEffect(() => {
    const controller = new AbortController();
    let live = true;
    const current = { origin, actorId, orgId, projectId };
    (async () => {
      const workspace = await getWorkspace(projectId, controller.signal);
      requireScriptScope(workspace, current);
      const saved = loadSourceIntent(window.sessionStorage, current);
      if (live) {
        pending.current = saved;
        setIntent(saved);
        setReady(true);
      }
    })().catch((failure: unknown) => {
      if (live) {
        setStorageError(
          failure instanceof Error
            ? failure.message
            : "原意图读取失败，请先恢复当前主体与浏览器存储。",
        );
        setReady(true);
      }
    });
    return () => {
      live = false;
      controller.abort();
    };
  }, [origin, actorId, orgId, projectId]);
  useEffect(() => {
    if (!intent && !storageError && !busy) return;
    function prevent(event: BeforeUnloadEvent) {
      event.preventDefault();
      event.returnValue = "";
    }
    window.addEventListener("beforeunload", prevent);
    return () => window.removeEventListener("beforeunload", prevent);
  }, [intent, storageError, busy]);
  async function refresh() {
    const results = await Promise.allSettled([
      cache.invalidateQueries({ queryKey: scriptWorkspaceKey(projectId) }),
      cache.invalidateQueries({ queryKey: scriptScopeKey(scope) }),
      cache.invalidateQueries({ queryKey: ["project", projectId] }),
      cache.invalidateQueries({ queryKey: ["projects"] }),
      cache.invalidateQueries({ queryKey: ["canvases", projectId] }),
    ]);
    if (results.some((result) => result.status === "rejected"))
      setError(
        "最新事实读取未完成，请重试读取工作区；原保存回执不会被当作最新头。",
      );
  }
  async function execute(original: SourceIntent, replay = false) {
    if (operation.current) return;
    operation.current = true;
    setBusy(true);
    setError(undefined);
    let sent = false;
    try {
      if (window.location.origin !== original.origin)
        throw new ApiError(409, "scope_changed");
      const current = await getWorkspace(projectId);
      requireScriptScope(current, original);
      const stored = loadSourceIntent(window.sessionStorage, scope);
      if (!stored || JSON.stringify(stored) !== JSON.stringify(original))
        throw new Error("浏览器存储与原意图不一致，请先恢复原键与原正文。");
      sent = true;
      const receipt = await runSourceIntent(original);
      clearSourceIntent(window.sessionStorage, scope);
      remember(null);
      setStorageError(undefined);
      setRejected(null);
      conflict.current = false;
      await refresh();
      onAccepted(receipt);
    } catch (failure) {
      const message =
        failure instanceof Error
          ? failure.message
          : "保存结果尚未确认，请人工使用原键核验。";
      setError(message);
      if (
        sent &&
        !unknownSourceWrite(failure) &&
        !(
          replay &&
          failure instanceof ApiError &&
          [401, 403, 404].includes(failure.status)
        )
      ) {
        try {
          clearSourceIntent(window.sessionStorage, scope);
          remember(null);
          setRejected({ intent: original, error: failure });
          conflict.current =
            failure instanceof ApiError && failure.status === 409;
          await refresh();
        } catch {
          setStorageError(
            "浏览器存储无法清理确定拒绝的原意图，请恢复存储后核验原键。",
          );
        }
      } else if (!sent && !(failure instanceof ApiError))
        setStorageError(message);
    } finally {
      operation.current = false;
      setBusy(false);
    }
  }
  async function submit(command: SourceCommand) {
    if (
      !ready ||
      operation.current ||
      pending.current ||
      storageError ||
      conflict.current
    )
      return;
    const checked = sourceCommandSchema.parse(command);
    const next = sourceIntentSchema.parse({
      ...checked,
      ...scope,
      version: 1,
      key: crypto.randomUUID(),
    });
    remember(next);
    setRejected(null);
    setError(undefined);
    try {
      saveSourceIntent(window.sessionStorage, next);
    } catch (failure) {
      setStorageError(
        failure instanceof Error
          ? failure.message
          : "原请求存储失败，当前不会发送修改。",
      );
      return;
    }
    await execute(next);
  }
  async function replay() {
    if (pending.current && !storageError) await execute(pending.current, true);
  }
  async function restoreStorage() {
    if (operation.current) return;
    operation.current = true;
    setBusy(true);
    try {
      const workspace = await getWorkspace(projectId);
      requireScriptScope(workspace, scope);
      let stored = loadSourceIntent(window.sessionStorage, scope);
      if (!stored && pending.current) {
        saveSourceIntent(window.sessionStorage, pending.current);
        stored = loadSourceIntent(window.sessionStorage, scope);
      }
      if (
        pending.current &&
        JSON.stringify(stored) !== JSON.stringify(pending.current)
      )
        throw new Error(
          "浏览器存储与保留的原键/原正文不一致，请恢复正确的原意图。",
        );
      remember(stored);
      setStorageError(undefined);
      setError(undefined);
      setReady(true);
    } catch (failure) {
      setStorageError(
        failure instanceof Error ? failure.message : "原请求存储仍不可恢复。",
      );
    } finally {
      operation.current = false;
      setBusy(false);
    }
  }
  function acknowledgeLatest() {
    if (!pending.current && !operation.current) {
      conflict.current = false;
      setRejected(null);
      setError(undefined);
    }
  }
  const conflicted =
    rejected?.error instanceof ApiError && rejected.error.status === 409;
  return {
    intent,
    ready,
    busy,
    storageError,
    error,
    rejected,
    conflicted,
    locked: !ready || busy || Boolean(intent || storageError),
    submit,
    replay,
    restoreStorage,
    acknowledgeLatest,
  };
}
