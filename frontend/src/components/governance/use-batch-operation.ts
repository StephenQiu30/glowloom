"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { RequestError } from "@/lib/request";
import { reauthenticate } from "@/api/identity";
import { authErrorMessage } from "@/components/auth/contracts";
import {
  executeBatch,
  type BatchEntry,
  type BatchOutcome,
} from "./batch-execution";
export type BatchAction<T> = {
  title: string;
  effect: string;
  destructive?: boolean;
  reason?: boolean;
  password?: boolean;
  compact?: boolean;
  label: (row: T) => string;
  prepare: (
    row: T,
    signal: AbortSignal,
    refresh: boolean,
  ) => Promise<{ row: T; skip?: string }>;
  execute: (
    entry: BatchEntry<T>,
    reason: string,
    signal: AbortSignal,
  ) => Promise<BatchOutcome>;
  recover?: (
    entry: BatchEntry<T>,
    signal: AbortSignal,
  ) => Promise<BatchOutcome | undefined>;
};
export function useBatchOperation<T extends { id?: string }>(
  onSettled: (successful: string[]) => void,
) {
  const [entries, setEntries] = useState<BatchEntry<T>[]>([]);
  const [action, setAction] = useState<BatchAction<T> | null>(null);
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const [generation, setGeneration] = useState(0);
  const [denied, setDenied] = useState(false);
  function checkAuthority(error: unknown) {
    if (error instanceof RequestError && [401, 403].includes(error.status))
      setDenied(true);
  }
  const lock = useRef(false);
  const flight = useRef<AbortController | null>(null);
  const savedReason = useRef("");
  const trigger = useRef<HTMLElement | null>(null);
  const resultTrigger = useRef<HTMLButtonElement | null>(null);
  const registerResultTrigger = useCallback(
    (element: HTMLButtonElement | null) => {
      resultTrigger.current = element;
    },
    [],
  );
  function restoreFocus() {
    const original = trigger.current;
    const target =
      original?.isConnected && !original.matches(":disabled")
        ? original
        : resultTrigger.current;
    if (!target?.isConnected) return false;
    target.focus();
    return true;
  }
  useEffect(
    () => () => {
      flight.current?.abort();
    },
    [],
  );
  const unresolved = entries.some((e) => e.status === "unknown");
  async function prepare(rows: T[], next: BatchAction<T>, refresh = false) {
    if (lock.current || unresolved) return;
    trigger.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    lock.current = true;
    setBusy(true);
    setOpen(true);
    setError("");
    setSubmitted(false);
    setGeneration((value) => value + 1);
    setEntries([]);
    setAction(next);
    const c = new AbortController();
    flight.current = c;
    try {
      const result = await Promise.all(
        rows
          .filter((r) => r.id)
          .map(async (row) => {
            const base: BatchEntry<T> = {
              id: row.id!,
              row,
              label: next.label(row),
              requestId: crypto.randomUUID(),
              status: "ready",
              message: "将执行",
            };
            try {
              const p = await next.prepare(row, c.signal, refresh);
              return {
                ...base,
                row: p.row,
                status: p.skip ? ("skipped" as const) : ("ready" as const),
                message: p.skip ?? "将执行",
              };
            } catch (e) {
              checkAuthority(e);
              return {
                ...base,
                status: "failed" as const,
                message: authErrorMessage(e),
              };
            }
          }),
      );
      if (!c.signal.aborted) setEntries(result);
    } finally {
      lock.current = false;
      if (!c.signal.aborted) setBusy(false);
    }
  }
  async function run(reason: string, password: string, replay = false) {
    if (!action || lock.current || (replay && !action.recover)) return;
    lock.current = true;
    setBusy(true);
    setError("");
    const c = new AbortController();
    flight.current = c;
    const succeeded: string[] = [];
    try {
      if (action.password)
        await reauthenticate({ password }, { signal: c.signal });
      if (!replay) savedReason.current = reason.trim();
      setSubmitted(true);
      const work = entries.map((e) =>
        replay && e.status === "unknown"
          ? { ...e, status: "ready" as const }
          : e,
      );
      await executeBatch(
        work,
        async (entry) => {
          // Replayed writes retain the frozen request identity, revision and reason.
          if (!replay) {
            const p = await action.prepare(entry.row, c.signal, false);
            if (p.skip) return { status: "skipped", message: p.skip };
          }
          try {
            return await action.execute(entry, savedReason.current, c.signal);
          } catch (error) {
            checkAuthority(error);
            throw error;
          }
        },
        (entry) => {
          if (entry.status === "success" || entry.status === "submitted")
            succeeded.push(entry.id);
          setEntries((current) =>
            current.map((e) => (e.id === entry.id ? entry : e)),
          );
        },
        c.signal,
      );
      if (!c.signal.aborted) onSettled(succeeded);
    } catch (e) {
      if (!c.signal.aborted) {
        checkAuthority(e);
        setError(authErrorMessage(e));
      }
    } finally {
      lock.current = false;
      if (!c.signal.aborted) setBusy(false);
    }
  }
  async function recover() {
    if (!action?.recover || lock.current) return;
    lock.current = true;
    setBusy(true);
    setError("");
    const c = new AbortController();
    flight.current = c;
    const succeeded: string[] = [];
    try {
      for (const entry of entries.filter((e) => e.status === "unknown")) {
        try {
          const result = await action.recover(entry, c.signal);
          if (c.signal.aborted) break;
          if (result) {
            setEntries((list) =>
              list.map((e) =>
                e.id === entry.id
                  ? {
                      ...e,
                      status: result.status ?? "success",
                      message: result.message,
                    }
                  : e,
              ),
            );
            succeeded.push(entry.id);
          } else
            setError(
              "尚未找到成功回执。可继续查询，或使用原请求安全恢复；不会新建请求标识。",
            );
        } catch (e) {
          if (!c.signal.aborted) {
            checkAuthority(e);
            setError(authErrorMessage(e));
          }
        }
      }
      if (!c.signal.aborted) onSettled(succeeded);
    } finally {
      lock.current = false;
      if (!c.signal.aborted) setBusy(false);
    }
  }
  return {
    entries,
    generation,
    denied,
    action,
    open,
    setOpen,
    busy,
    error,
    submitted,
    unresolved,
    registerResultTrigger,
    restoreFocus,
    prepare,
    run,
    recover,
    retry: () =>
      action &&
      prepare(
        entries.filter((e) => e.status === "failed").map((e) => e.row),
        action,
        true,
      ),
  };
}
