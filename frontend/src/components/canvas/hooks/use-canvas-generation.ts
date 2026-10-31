"use client";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import useSWR from "@/components/canvas/runtime/lib/use-live-query";
import { unstable_serialize, useSWRConfig } from "swr";
import * as api from "@/components/canvas/runtime/api/canvas-generation-bff";
import * as tasks from "@/components/canvas/runtime/api/generation-bff";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { useModelBillingApproval } from "@/components/canvas/runtime/components/credits/model-billing-provider";
import {
  notifyError,
  useErrorNotification,
} from "@/components/canvas/runtime/components/feedback/error-notification";
import { type CanvasGenerationProjection } from "../lib/canvas-generation-projection";
import type { CanvasPreparedDocument } from "../types/canvas-document-integration";

/** Reads restore the original commands; only explicit submit/retry can create work. */
export function useCanvasGeneration({
  canvasId,
  identity,
  enabled,
  prepare,
}: {
  canvasId: string;
  identity: string;
  enabled: boolean;
  prepare: () => Promise<CanvasPreparedDocument>;
}) {
  const approveBilling = useModelBillingApproval();
  const { cache } = useSWRConfig();
  const lifecycle = useMemo(
    () => ({
      identity,
      canvasId,
      scope: crypto.randomUUID(),
      activeRef: { current: enabled },
      keys: { current: new Set<string>() },
      grantsRef: { current: new Map<string, Promise<string>>() },
      recoveries: { current: new Map<string, string>() },
    }),
    [identity, canvasId, enabled],
  );
  const {
    activeRef,
    keys: previewKeysRef,
    grantsRef,
    recoveries: recoveredPreviewsRef,
  } = lifecycle;
  const previewScope = useId();
  const busy = useRef(new Set<string>());
  const [failedMedia, setFailedMedia] = useState<Map<string, string>>(
    () => new Map(),
  );
  const [submitting, setSubmitting] = useState<string[]>([]);
  const query = useSWR(
    enabled ? ["canvas-generation", identity, canvasId] : null,
    async (_key, options) =>
      unwrap(await api.listCanvasGenerations({ canvasId }, options)),
    {
      shouldRetryOnError: false,
      revalidateOnFocus: false,
    },
  );
  useErrorNotification(query.error, "画布任务读取失败");
  useEffect(() => {
    activeRef.current = enabled;
    const currentKeys = previewKeysRef.current;
    const currentGrants = grantsRef.current;
    const currentRecoveries = recoveredPreviewsRef.current;
    return () => {
      activeRef.current = false;
      currentGrants.clear();
      currentRecoveries.clear();
      currentKeys.forEach((key) => cache.delete(key));
      currentKeys.clear();
    };
  }, [
    enabled,
    cache,
    previewScope,
    activeRef,
    grantsRef,
    recoveredPreviewsRef,
    previewKeysRef,
  ]);
  const previewKey = (run: CanvasContract.CanvasGenerationVO) =>
    JSON.stringify([
      identity,
      canvasId,
      run.commandId,
      run.taskId,
      run.outputs,
    ]);
  const ready = (query.data ?? []).filter(
    (run) => run.state === "SUCCEEDED" && run.taskId && run.outputs?.length,
  );
  // Read grantsRef independently of task polling. One completed grant must not
  // cancel slower siblings, and another account/canvas owns a separate cache.
  const previews = useSWR(
    enabled && ready.length
      ? [
          "canvas-generation-previews",
          previewScope,
          lifecycle.scope,
          identity,
          canvasId,
          ready.map((run) => [run.commandId, run.taskId, run.outputs]),
        ]
      : null,
    async () => {
      previewKeysRef.current.add(
        unstable_serialize([
          "canvas-generation-previews",
          previewScope,
          lifecycle.scope,
          identity,
          canvasId,
          ready.map((run) => [run.commandId, run.taskId, run.outputs]),
        ]),
      );
      const keys = new Set(ready.map(previewKey));
      for (const key of grantsRef.current.keys())
        if (!keys.has(key)) {
          grantsRef.current.delete(key);
          recoveredPreviewsRef.current.delete(key);
        }
      const results = await Promise.allSettled(
        ready.map(async (run) => {
          const key = previewKey(run);
          const outputIndex = Number(
            run.outputs![0].referenceKey?.replace("output-", ""),
          );
          if (!Number.isInteger(outputIndex) || outputIndex < 1)
            throw new Error("生成结果编号无效");
          let pending = grantsRef.current.get(key);
          if (!pending) {
            pending = tasks
              .readGenerationFile({ taskId: run.taskId!, outputIndex })
              .then(unwrap)
              .then((grant) => {
                if (!grant.url?.startsWith("/api/file/content?ticket="))
                  throw new Error("生成结果读取授权无效");
                return grant.url;
              });
            grantsRef.current.set(key, pending);
          }
          return [run.commandId!, await pending] as const;
        }),
      );
      if (!activeRef.current)
        return {
          scope: lifecycle.scope,
          identity,
          canvasId,
          sourceKeys: {},
          urls: {},
          failedCommands: [],
          error: undefined,
        };
      return {
        scope: lifecycle.scope,
        identity,
        canvasId,
        sourceKeys: Object.fromEntries(
          ready.map((run) => [run.commandId!, previewKey(run)]),
        ),
        urls: Object.fromEntries(
          results.flatMap((result) =>
            result.status === "fulfilled" ? [result.value] : [],
          ),
        ),
        failedCommands: results.flatMap((result, index) =>
          result.status === "rejected" ? [ready[index].commandId] : [],
        ),
        error: results.find((result) => result.status === "rejected")?.reason,
      };
    },
    {
      shouldRetryOnError: false,
      revalidateOnFocus: false,
      revalidateOnReconnect: false,
      keepPreviousData: true,
      dedupingInterval: 0,
    },
  );
  const currentPreviews =
    enabled &&
    previews.data?.scope === lifecycle.scope &&
    previews.data?.identity === identity &&
    previews.data.canvasId === canvasId
      ? previews.data
      : undefined;
  useErrorNotification(
    previews.error ?? currentPreviews?.error,
    "生成结果预览失败",
  );
  const runs: CanvasGenerationProjection[] = (query.data ?? []).map((run) => {
    const key = previewKey(run);
    const preview =
      currentPreviews?.sourceKeys[run.commandId ?? ""] === key
        ? currentPreviews
        : undefined;
    const url = preview?.urls[run.commandId ?? ""];
    const failed =
      (!!url && failedMedia.get(key) === url) ||
      preview?.failedCommands.includes(run.commandId);
    return {
      ...run,
      previewFailed: failed,
      previewUrl: enabled && !failed ? url : undefined,
    };
  });
  const action = async (nodeId: string, operation: () => Promise<unknown>) => {
    if (!enabled || busy.current.has(nodeId)) return;
    busy.current.add(nodeId);
    setSubmitting([...busy.current]);
    try {
      await operation();
    } catch (error) {
      if (activeRef.current) notifyError(error, "画布生成未完成");
    } finally {
      try {
        if (activeRef.current) await query.mutate();
      } finally {
        busy.current.delete(nodeId);
        if (activeRef.current) setSubmitting([...busy.current]);
      }
    }
  };
  const generate = (nodeId: string) =>
    action(nodeId, async () => {
      const head = await prepare();
      if (!activeRef.current) return;
      const body: CanvasContract.CanvasGenerateRequest = {
        canvasId,
        nodeId,
        commandId: crypto.randomUUID(),
        expectedRevision: head.revision,
        expectedProjectBindingRevision: head.projectBindingRevision,
        expectedEpoch: head.epoch,
        expectedCommitSeq: head.commitSeq,
        expectedGraphHash: head.graphHash,
      };
      const options = unwrap(
        await tasks.getGenerationOptions({ modality: "image" }),
      );
      if (options.billingEnabled) {
        const quote = unwrap(await api.quoteCanvasGenerationCredits(body));
        if (!activeRef.current || !(await approveBilling(quote))) return;
        body.quoteId = quote.id;
      }
      if (!activeRef.current) return;
      unwrap(await api.submitCanvasGeneration(body));
    });
  const refresh = (run: CanvasGenerationProjection, retry = false) =>
    action(run.nodeId!, async () => {
      if (run.state === "SUCCEEDED") {
        const key = previewKey(run);
        grantsRef.current.delete(key);
        recoveredPreviewsRef.current.delete(key);
        setFailedMedia((previous) => {
          const next = new Map(previous);
          next.delete(key);
          return next;
        });
        await previews.mutate();
        return;
      }
      unwrap(
        await (retry ? api.retryCanvasGeneration : api.refreshCanvasGeneration)(
          { canvasId, commandId: run.commandId! },
        ),
      );
    });
  const recoverPreview = (nodeId: string, source: string) => {
    if (!enabled || !activeRef.current) return;
    const run = runs.find(
      (value) => value.nodeId === nodeId && value.previewUrl === source,
    );
    if (!run || run.state !== "SUCCEEDED") return;
    const key = previewKey(run);
    if (recoveredPreviewsRef.current.has(key)) {
      // Repeated events for the old URL while renewal is pending are duplicates.
      if (recoveredPreviewsRef.current.get(key) === source) return;
      setFailedMedia((previous) => new Map(previous).set(key, source));
      return;
    }
    recoveredPreviewsRef.current.set(key, source);
    grantsRef.current.delete(key);
    void previews.mutate();
  };
  const previewLoaded = (nodeId: string, source: string) => {
    const run = runs.find(
      (value) => value.nodeId === nodeId && value.previewUrl === source,
    );
    if (run) recoveredPreviewsRef.current.delete(previewKey(run));
  };
  return {
    ready: enabled && !!query.data && !query.error,
    runs,
    submitting,
    generate,
    refresh,
    recoverPreview,
    previewLoaded,
  };
}
