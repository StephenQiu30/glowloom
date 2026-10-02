"use client";
import { useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/request";
import { freshLibrary } from "./library-queries";
import { unknownLibraryWrite } from "./library-intent";
import {
  clearPurgeIntent,
  loadPurgeIntent,
  savePurgeIntent,
} from "./library-purge-intent";
import {
  purgeIntentSchema,
  type PurgeIntent,
  type PurgeJob,
  type PurgeReview,
  purgeTerminal,
  requirePurgeRecoveryBudget,
} from "./library-purge-model";
import { runLibraryPurge } from "./library-purge-query";
import {
  checkPurgePlan,
  clearPurgePlan,
  createPurgePlan,
  loadPurgePlan,
  recordPurgePlanJob,
  savePurgePlan,
  type PurgePlan,
} from "./library-purge-plan";
import type { LibraryIdentity } from "./library-model";
import {
  requirePurgeForeground,
  waitPurgeTerminal,
} from "./library-purge-runner";
type PurgeCommand =
  | Omit<
      Extract<PurgeIntent, { action: "create" }>,
      keyof LibraryIdentity | "version" | "key"
    >
  | Omit<
      Extract<PurgeIntent, { action: "cancel" | "reconcile" }>,
      keyof LibraryIdentity | "version" | "key"
    >;
const message = (cause: unknown) =>
  cause instanceof Error
    ? cause.message
    : "原清理结果未确认，请保留原键并人工恢复。";
export function usePurgeWriter(
  identity: LibraryIdentity,
  onAccepted: (job: PurgeJob) => void | Promise<void>,
) {
  const [intent, setIntent] = useState<PurgeIntent | null>(null),
    [ready, setReady] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string>(),
    [storageError, setStorageError] = useState<string>(),
    [rejected, setRejected] = useState(false);
  const [plan, setPlan] = useState<PurgePlan | null>(null),
    [planComplete, setPlanComplete] = useState(false),
    [running, setRunning] = useState(false);
  const runner = useRef<AbortController | null>(null);
  const pendingPlan = useRef<PurgePlan | null>(null),
    planWrite = useRef<{ value: PurgePlan; previous: PurgePlan | null } | null>(
      null,
    );
  const pending = useRef<PurgeIntent | null>(null),
    active = useRef(false),
    mounted = useRef(true);
  const { origin, actorId, orgId, libraryId } = identity,
    kind = identity.scope.kind,
    projectId =
      identity.scope.kind === "project" ? identity.scope.project_id : undefined;
  useEffect(() => {
    mounted.current = true;
    const controller = new AbortController();
    const scopeIdentity: LibraryIdentity = {
      origin,
      actorId,
      orgId,
      libraryId,
      scope: kind === "project" ? { kind, project_id: projectId! } : { kind },
    };
    void freshLibrary(scopeIdentity, controller.signal)
      .then(() => {
        if (!mounted.current || controller.signal.aborted) return;
        let saved = loadPurgeIntent(sessionStorage, scopeIdentity);
        const storedPlan = loadPurgePlan(sessionStorage, scopeIdentity),
          frozen = storedPlan?.batches.find(
            (batch) => batch.input && !batch.job,
          );
        requirePurgeRecoveryBudget(storedPlan, saved);
        if (frozen && !saved) {
          saved = purgeIntentSchema.parse({
            ...scopeIdentity,
            version: 1,
            key: frozen.key,
            action: "create",
            body: frozen.input,
          });
          requirePurgeRecoveryBudget(storedPlan, saved);
          savePurgeIntent(sessionStorage, saved);
        }
        if (mounted.current && !controller.signal.aborted) {
          pending.current = saved;
          pendingPlan.current = storedPlan;
          setIntent(saved);
          setPlan(storedPlan);
          setReady(true);
        }
      })
      .catch((cause: unknown) => {
        if (mounted.current && !controller.signal.aborted) {
          setStorageError(message(cause));
          setReady(true);
        }
      });
    return () => {
      mounted.current = false;
      controller.abort();
      runner.current?.abort();
    };
  }, [origin, actorId, orgId, libraryId, kind, projectId]);
  useEffect(() => {
    if (!intent && !plan && !busy && !storageError) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [intent, plan, busy, storageError]);
  const remember = (original: PurgeIntent | null) => {
    pending.current = original;
    if (mounted.current) setIntent(original);
  };
  function storePlan(value: PurgePlan) {
    const previous = pendingPlan.current;
    planWrite.current = { value, previous };
    try {
      requirePurgeRecoveryBudget(value, pending.current);
      savePurgePlan(sessionStorage, value, previous);
      planWrite.current = null;
    } catch (cause) {
      setStorageError(message(cause));
      throw cause;
    }
    pendingPlan.current = value;
    setPlan(value);
  }
  async function execute(
    original: PurgeIntent,
    replay: boolean,
    signal?: AbortSignal,
  ) {
    if (active.current) return;
    active.current = true;
    setBusy(true);
    setError(undefined);
    let sent = false;
    try {
      await freshLibrary(identity, signal);
      requirePurgeRecoveryBudget(
        loadPurgePlan(sessionStorage, identity),
        original,
      );
      if (
        JSON.stringify(loadPurgeIntent(sessionStorage, identity)) !==
        JSON.stringify(original)
      )
        throw new Error("原键与完整正文不一致，请恢复原存储。");
      signal?.throwIfAborted();
      sent = true;
      const job = await runLibraryPurge(original, signal);
      if (pendingPlan.current && original.action === "create")
        storePlan(recordPurgePlanJob(pendingPlan.current, job));
      clearPurgeIntent(sessionStorage, identity);
      remember(null);
      setRejected(false);
      setStorageError(undefined);
      if (mounted.current) {
        try {
          await onAccepted(job);
        } catch {
          setError("清理受理已确认；最新状态读取失败，请重新读取。");
        }
      }
      return job;
    } catch (cause) {
      if (mounted.current) {
        setError(message(cause));
        if (
          sent &&
          !unknownLibraryWrite(cause) &&
          !(
            replay &&
            cause instanceof ApiError &&
            [401, 403, 404].includes(cause.status)
          )
        )
          setRejected(true);
      }
    } finally {
      active.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  async function submit(
    command: PurgeCommand,
    frozenKey = crypto.randomUUID(),
    signal?: AbortSignal,
  ) {
    if (!ready || active.current || pending.current || storageError) return;
    if (command.action !== "create" && runner.current) return;
    const original = purgeIntentSchema.parse({
      ...identity,
      version: 1,
      key: frozenKey,
      ...command,
    });
    remember(original);
    try {
      requirePurgeRecoveryBudget(pendingPlan.current, original);
      savePurgeIntent(sessionStorage, original);
    } catch (cause) {
      setStorageError(message(cause));
      return;
    }
    return execute(original, false, signal);
  }
  async function replay() {
    if (pending.current && !storageError && !rejected)
      await execute(pending.current, true);
  }
  async function restoreStorage() {
    if (active.current) return;
    active.current = true;
    setBusy(true);
    try {
      await freshLibrary(identity);
      requirePurgeRecoveryBudget(
        planWrite.current?.value ?? loadPurgePlan(sessionStorage, identity),
        pending.current ?? loadPurgeIntent(sessionStorage, identity),
      );
      if (pending.current) savePurgeIntent(sessionStorage, pending.current);
      else remember(loadPurgeIntent(sessionStorage, identity));
      if (planWrite.current) {
        const { value, previous } = planWrite.current,
          stored = loadPurgePlan(sessionStorage, identity);
        if (JSON.stringify(stored) !== JSON.stringify(value))
          savePurgePlan(sessionStorage, value, previous);
        pendingPlan.current = value;
        setPlan(value);
        planWrite.current = null;
      } else {
        const stored = loadPurgePlan(sessionStorage, identity);
        pendingPlan.current = stored;
        setPlan(stored);
      }
      setStorageError(undefined);
      setReady(true);
    } catch (cause) {
      setStorageError(message(cause));
    } finally {
      active.current = false;
      setBusy(false);
    }
  }
  async function releaseRejected() {
    if (active.current || !rejected) return false;
    active.current = true;
    setBusy(true);
    try {
      await freshLibrary(identity);
      if (pendingPlan.current && pending.current?.action === "create") {
        const key = pending.current.key;
        if (
          pendingPlan.current.batches.some(
            (batch) => batch.key === key && !batch.job,
          )
        ) {
          clearPurgePlan(sessionStorage, identity);
          pendingPlan.current = null;
          setPlan(null);
          setPlanComplete(false);
        }
      }
      clearPurgeIntent(sessionStorage, identity);
      remember(null);
      setRejected(false);
      setError(undefined);
      return true;
    } catch (cause) {
      setError(message(cause));
      return false;
    } finally {
      active.current = false;
      setBusy(false);
    }
  }
  async function continuePlan() {
    if (
      !ready ||
      active.current ||
      pending.current ||
      storageError ||
      !pendingPlan.current ||
      runner.current
    )
      return;
    const controller = new AbortController();
    runner.current = controller;
    setRunning(true);
    setError(undefined);
    const stopHidden = () => {
        if (document.visibilityState !== "visible") controller.abort();
      },
      stopPage = () => controller.abort();
    document.addEventListener("visibilitychange", stopHidden);
    window.addEventListener("pagehide", stopPage);
    try {
      while (pendingPlan.current && mounted.current) {
        requirePurgeForeground(identity, controller.signal);
        const outstanding = pendingPlan.current.batches.find(
          (batch) => batch.job && !purgeTerminal(batch.job),
        );
        if (outstanding?.job) {
          const terminal = await waitPurgeTerminal(
            identity,
            outstanding.job,
            controller.signal,
          );
          storePlan(recordPurgePlanJob(pendingPlan.current, terminal));
        }
        active.current = true;
        setBusy(true);
        const checked = await checkPurgePlan(
          pendingPlan.current,
          controller.signal,
        );
        requirePurgeForeground(identity, controller.signal);
        storePlan(checked.plan);
        setPlanComplete(checked.index === null);
        active.current = false;
        setBusy(false);
        if (checked.index === null) break;
        const batch = checked.plan.batches[checked.index];
        const job = await submit(
          { action: "create", body: batch.input! },
          batch.key,
          controller.signal,
        );
        if (!job || pending.current) break;
        const terminal = await waitPurgeTerminal(
          identity,
          job,
          controller.signal,
        );
        requirePurgeForeground(identity, controller.signal);
        storePlan(recordPurgePlanJob(pendingPlan.current!, terminal));
      }
    } catch (cause) {
      if (mounted.current)
        setError(
          controller.signal.aborted
            ? "完整计划已停止，原范围、子键与任务均已保留。请明确核验后继续。"
            : message(cause),
        );
    } finally {
      document.removeEventListener("visibilitychange", stopHidden);
      window.removeEventListener("pagehide", stopPage);
      runner.current = null;
      active.current = false;
      if (mounted.current) {
        setBusy(false);
        setRunning(false);
      }
    }
  }
  async function startPlan(review: PurgeReview) {
    if (
      active.current ||
      pending.current ||
      pendingPlan.current ||
      storageError ||
      !ready
    )
      return;
    try {
      storePlan(createPurgePlan(identity, review));
    } catch (cause) {
      setError(message(cause));
      return;
    }
    await continuePlan();
  }
  async function endPlan() {
    if (
      active.current ||
      pending.current ||
      storageError ||
      !pendingPlan.current ||
      runner.current
    )
      return false;
    active.current = true;
    setBusy(true);
    setError(undefined);
    try {
      await freshLibrary(identity);
      for (const batch of pendingPlan.current.batches) {
        if (batch.input && !batch.job)
          throw new Error("已冻结子键尚未确认受理结果，请先恢复原子批。");
        if (batch.job) {
          const actual = await import("./library-purge-query").then((module) =>
            module.getLibraryPurge(identity, batch.job!.id),
          );
          if (!purgeTerminal(actual))
            throw new Error(
              "原子批仍在执行或结果未知，不能结束计划。请核对/取消并等待真实终态。",
            );
        }
      }
      clearPurgePlan(sessionStorage, identity);
      pendingPlan.current = null;
      setPlan(null);
      setPlanComplete(false);
      return true;
    } catch (cause) {
      setError(message(cause));
      return false;
    } finally {
      active.current = false;
      setBusy(false);
    }
  }
  return {
    intent,
    plan,
    planComplete,
    running,
    ready,
    busy,
    error,
    storageError,
    rejected,
    locked: !ready || busy || Boolean(intent || plan || storageError),
    submit,
    replay,
    restoreStorage,
    releaseRejected,
    startPlan,
    continuePlan,
    endPlan,
    stopPlan: () => runner.current?.abort(),
  };
}
export type PurgeWriter = ReturnType<typeof usePurgeWriter>;
