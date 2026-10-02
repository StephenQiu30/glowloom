"use client";
import { useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/request";
import { unknownSourceWrite, type ScriptScope } from "./source-intent";
import {
  clearFileImportIntent,
  fileImportIntentSchema,
  loadFileImportIntent,
  saveFileImportIntent,
  type FileImportIntent,
} from "./file-import-intent";
import {
  fileImportCommandSchema,
  type FileImportCommand,
  type FileImportJob,
} from "./file-import-model";
import { listFileImports, runFileImportIntent } from "./file-import-queries";
export function useFileImport(
  scope: ScriptScope,
  onAccepted: (receipt: FileImportJob) => void | Promise<void>,
) {
  const [intent, setIntent] = useState<FileImportIntent | null>(null);
  const pending = useRef<FileImportIntent | null>(null);
  const active = useRef(false);
  const rejection = useRef(false);
  const [rejected, setRejected] = useState<{
    intent: FileImportIntent;
    cause: unknown;
  } | null>(null);
  const [busy, setBusy] = useState(false);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState<string>();
  const [storageError, setStorageError] = useState<string>();
  const { origin, actorId, orgId, projectId } = scope;
  useEffect(() => {
    const controller = new AbortController();
    let live = true;
    const current = { origin, actorId, orgId, projectId };
    void listFileImports(current, 0, controller.signal)
      .then((page) => {
        if (page.current_actor_id !== actorId || page.current_org_id !== orgId)
          throw new ApiError(409, "scope_changed");
        const stored = loadFileImportIntent(sessionStorage, current);
        if (live) {
          pending.current = stored;
          setIntent(stored);
          setReady(true);
        }
      })
      .catch((cause) => {
        if (live) {
          setStorageError(
            cause instanceof Error ? cause.message : "导入原意图无法读取。",
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
    if (!intent && !storageError && !busy && !rejected) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [intent, storageError, busy, rejected]);
  function remember(value: FileImportIntent | null) {
    pending.current = value;
    setIntent(value);
  }
  async function fresh() {
    if (window.location.origin !== scope.origin)
      throw new ApiError(409, "scope_changed");
    const page = await listFileImports(scope);
    if (
      page.current_actor_id !== scope.actorId ||
      page.current_org_id !== scope.orgId
    )
      throw new ApiError(409, "scope_changed");
  }
  async function execute(original: FileImportIntent, replay: boolean) {
    if (active.current) return;
    active.current = true;
    setBusy(true);
    setError(undefined);
    let sent = false;
    try {
      await fresh();
      const stored = loadFileImportIntent(sessionStorage, scope);
      if (!stored || JSON.stringify(stored) !== JSON.stringify(original))
        throw new Error("导入存储与原键/完整原体不一致，请先恢复存储。");
      sent = true;
      const receipt = await runFileImportIntent(original);
      clearFileImportIntent(sessionStorage, scope);
      remember(null);
      setStorageError(undefined);
      setRejected(null);
      rejection.current = false;
      try {
        await onAccepted(receipt);
      } catch {
        setError("原请求已受理，最新任务或脚本事实读取未完成，请重新读取。");
      }
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "导入请求尚未确认，请人工使用原键核验。",
      );
      if (
        sent &&
        !unknownSourceWrite(cause) &&
        !(
          replay &&
          cause instanceof ApiError &&
          [401, 403, 404].includes(cause.status)
        )
      ) {
        try {
          clearFileImportIntent(sessionStorage, scope);
          remember(null);
          setRejected({ intent: original, cause });
          rejection.current = true;
        } catch {
          setStorageError("确定拒绝的导入意图存储无法清理，请恢复原存储。");
        }
      } else if (!sent && !(cause instanceof ApiError))
        setStorageError(
          cause instanceof Error ? cause.message : "导入存储不可用。",
        );
    } finally {
      active.current = false;
      setBusy(false);
    }
  }
  async function submit(command: FileImportCommand) {
    if (
      !ready ||
      active.current ||
      pending.current ||
      storageError ||
      rejection.current
    )
      return;
    const original = fileImportIntentSchema.parse({
      ...fileImportCommandSchema.parse(command),
      ...scope,
      version: 1,
      key: crypto.randomUUID(),
    });
    remember(original);
    setRejected(null);
    setError(undefined);
    try {
      saveFileImportIntent(sessionStorage, original);
    } catch (cause) {
      setStorageError(
        cause instanceof Error ? cause.message : "原导入存储失败。",
      );
      return;
    }
    await execute(original, false);
  }
  async function restoreStorage() {
    if (active.current) return;
    active.current = true;
    setBusy(true);
    try {
      await fresh();
      const stored = loadFileImportIntent(sessionStorage, scope);
      if (
        pending.current &&
        stored &&
        JSON.stringify(stored) !== JSON.stringify(pending.current)
      )
        throw new Error("存储与本页原导入不同，请保留两份事实后核验。");
      const original = pending.current ?? stored;
      if (original) saveFileImportIntent(sessionStorage, original);
      remember(original);
      setReady(true);
      setStorageError(undefined);
      setError(undefined);
    } catch (cause) {
      setStorageError(
        cause instanceof Error ? cause.message : "原导入存储仍不可用。",
      );
    } finally {
      active.current = false;
      setBusy(false);
    }
  }
  return {
    ready,
    busy,
    intent,
    rejected,
    error,
    storageError,
    locked: !ready || busy || Boolean(intent || storageError || rejected),
    submit,
    replay: () =>
      pending.current && !storageError
        ? execute(pending.current, true)
        : Promise.resolve(),
    restoreStorage,
    acknowledgeLatest: () => {
      if (!pending.current && !active.current) {
        rejection.current = false;
        setRejected(null);
        setError(undefined);
      }
    },
  };
}
