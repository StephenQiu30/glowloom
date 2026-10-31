"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import useSWR from "@/components/canvas/runtime/lib/use-live-query";
import * as api from "@/components/canvas/runtime/api/workflow-bff";
import { refreshGenerationTask } from "@/components/canvas/runtime/api/generation-bff";
import { ApiError, unwrap } from "@/components/canvas/runtime/lib/request";
import type { CanvasPreparedDocument } from "../types/canvas-document-integration";
import {
  claimWorkflowCommand,
  clearWorkflowCommand,
  readWorkflowCommand,
} from "../lib/canvas-workflow-journal";
import {
  notifyError,
  useErrorNotification,
} from "@/components/canvas/runtime/components/feedback/error-notification";

type Preview = {
  scope: string;
  document: CanvasContract.CanvasVO;
  request: CanvasContract.WorkflowSubmitRequest;
};

/** 页面只保存/发出显式命令和读取回执，绝不在浏览器调度节点。 */
export function useCanvasWorkflow({
  canvasId,
  identity,
  enabled,
  canStart,
  prepare,
}: {
  canvasId: string;
  identity: string;
  enabled: boolean;
  canStart: boolean;
  prepare: () => Promise<CanvasPreparedDocument>;
}) {
  const scope = useMemo(
    () => ({
      activeRef: { current: enabled },
      busyRef: { current: false },
      token: crypto.randomUUID(),
      key: "glowloom.workflow.pending." + identity + "." + canvasId,
    }),
    [canvasId, identity, enabled],
  );
  const { activeRef, busyRef } = scope;
  useEffect(() => {
    activeRef.current = enabled;
    return () => {
      activeRef.current = false;
    };
  }, [enabled, activeRef]);
  const [busyState, setBusy] = useState({ token: scope.token, value: false });
  const busy = busyState.token === scope.token && busyState.value;
  const [preview, setPreview] = useState<Preview>();
  const [unacceptedId, setUnacceptedId] = useState<string>();
  const [historyOpen, setHistoryOpen] = useState(false);
  const stopCommands = useRef(new Map<string, string>());
  const options = useSWR(
    enabled ? ["workflow-options", identity] : null,
    async () => unwrap(await api.getWorkflowOptions()),
    { shouldRetryOnError: false, revalidateOnFocus: false },
  );
  const available = enabled && options.data?.enabled === true;
  const query = useSWR(
    available ? ["canvas-workflows", identity, canvasId] : null,
    async (_key, options) =>
      unwrap(await api.listWorkflowHistory({ canvasId }, options)),
    { shouldRetryOnError: false, revalidateOnFocus: false },
  );
  const journal = useSWR(
    enabled ? ["workflow-journal", identity, canvasId] : null,
    async () => readWorkflowCommand(scope.key, canvasId),
    { shouldRetryOnError: false, revalidateOnFocus: false },
  );
  useErrorNotification(options.error, "流程能力读取失败");
  useErrorNotification(query.error, "流程回执读取失败");
  useErrorNotification(journal.error, "原运行命令读取失败");

  const { mutate: refreshJournal } = journal;
  useEffect(() => {
    if (!enabled) return;
    const refresh = () => {
      // SWR retains the read error for the existing error notification.
      void refreshJournal().catch(() => undefined);
    };
    const changed = (event: StorageEvent) => {
      if (event.key === scope.key || event.key === null) refresh();
    };
    window.addEventListener("storage", changed);
    window.addEventListener("focus", refresh);
    window.addEventListener("pageshow", refresh);
    return () => {
      window.removeEventListener("storage", changed);
      window.removeEventListener("focus", refresh);
      window.removeEventListener("pageshow", refresh);
    };
  }, [enabled, scope.key, refreshJournal]);

  const clear = async (commandId: string) => {
    const current = await clearWorkflowCommand(scope.key, canvasId, commandId);
    await journal.mutate(current, { revalidate: false });
  };
  const execute = async (operation: () => Promise<void>, label: string) => {
    if (!activeRef.current || busyRef.current) return;
    busyRef.current = true;
    setBusy({ token: scope.token, value: true });
    try {
      await operation();
    } catch (error) {
      if (activeRef.current) notifyError(error, label);
    } finally {
      busyRef.current = false;
      if (activeRef.current) setBusy({ token: scope.token, value: false });
    }
  };
  const recover = () =>
    execute(async () => {
      if (!journal.data) return;
      try {
        unwrap(
          await api.getWorkflowCommand({ commandId: journal.data.commandId }),
        );
        if (!activeRef.current) return;
        await clear(journal.data.commandId!);
        setUnacceptedId(undefined);
        await query.mutate();
      } catch (error) {
        if (
          activeRef.current &&
          error instanceof ApiError &&
          error.code === 40400
        ) {
          setUnacceptedId(journal.data.commandId);
          return;
        }
        throw error;
      }
    }, "原运行回执核对未完成");
  const send = async (
    request: CanvasContract.WorkflowSubmitRequest,
    fresh = false,
  ) => {
    try {
      await claimWorkflowCommand(scope.key, canvasId, request, fresh);
    } catch (error) {
      if (activeRef.current) {
        setPreview(undefined);
        setHistoryOpen(true);
        await journal.mutate();
      }
      throw error;
    }
    if (!activeRef.current) return;
    await journal.mutate(readWorkflowCommand(scope.key, canvasId), {
      revalidate: false,
    });
    let run: CanvasContract.WorkflowRunVO;
    try {
      run = unwrap(await api.submitWorkflowRun(request));
    } catch (error) {
      if (
        fresh &&
        error instanceof ApiError &&
        error.code >= 40000 &&
        error.code < 50000
      )
        await clear(request.commandId!);
      else if (activeRef.current) {
        setPreview(undefined);
        setHistoryOpen(true);
      }
      throw error;
    }
    if (!activeRef.current) return;
    await clear(request.commandId!);
    setPreview(undefined);
    setUnacceptedId(undefined);
    setHistoryOpen(true);
    await query.mutate(
      (previous) => [
        run,
        ...(previous ?? []).filter((row) => row.runId !== run.runId),
      ],
      { revalidate: false },
    );
  };
  const choose = (nodeIds: string[]) =>
    execute(async () => {
      if (!canStart || !available || !options.data?.acceptNewRuns)
        throw new Error("流程未开放新运行");
      if (journal.isLoading || journal.error)
        throw new Error("请先恢复原运行命令记录");
      const current = readWorkflowCommand(scope.key, canvasId);
      await journal.mutate(current, { revalidate: false });
      if (current) {
        setHistoryOpen(true);
        throw new Error("请先核对原运行回执，避免重复受理");
      }
      if (query.data?.some((row) => row.lifecycle !== "CLOSED")) {
        setHistoryOpen(true);
        throw new Error("该画布已有未关闭运行，请查看原运行状态");
      }
      const document = await prepare();
      if (!activeRef.current) return;
      const ids = [...new Set(nodeIds)];
      if (
        !ids.length ||
        ids.length > 20 ||
        ids.some((id) => !document.graph.nodes.some((node) => node.id === id))
      )
        throw new Error("请选择1至20个节点或分组");
      setPreview({
        scope: scope.key,
        document,
        request: {
          commandId: crypto.randomUUID(),
          canvasId,
          expectedRevision: document.revision,
          expectedProjectBindingRevision: document.projectBindingRevision,
          expectedEpoch: document.epoch,
          expectedCommitSeq: document.commitSeq,
          expectedGraphHash: document.graphHash,
          nodeIds: ids,
        },
      });
    }, "运行范围准备未完成");
  return {
    integration: {
      agentEnabled:
        options.data?.nodeTypes?.includes("agent.fixed-research@1") === true,
      agentProfiles: options.data?.agentProfiles,
      enabled: available,
      accepting: canStart && options.data?.acceptNewRuns === true,
      busy,
      choose,
    },
    available,
    options: options.data,
    runs: query.data ?? [],
    error: query.error,
    busy,
    preview: preview?.scope === scope.key ? preview : undefined,
    historyOpen,
    setHistoryOpen,
    pending: journal.data,
    unaccepted: !!journal.data && journal.data.commandId === unacceptedId,
    cancelPreview: () => {
      if (!busyRef.current) setPreview(undefined);
    },
    confirm: () =>
      execute(async () => {
        if (!canStart) throw new Error("画布访问已暂停，请先验证权限");
        if (preview?.scope === scope.key) await send(preview.request, true);
      }, "流程受理未完成，请核对原命令"),
    recover,
    closeSubmission: () =>
      execute(async () => {
        const original = journal.data;
        if (!original) return;
        const receipt = unwrap(
          await api.closeWorkflowSubmission({ commandId: original.commandId }),
        );
        if (!activeRef.current) return;
        if (
          receipt.commandId !== original.commandId ||
          !["ABANDONED", "ACCEPTED"].includes(receipt.state ?? "") ||
          (receipt.state === "ACCEPTED" &&
            receipt.run?.commandId !== original.commandId)
        )
          throw new Error("原提交关闭回执无效，命令记录仍保留");
        await clear(original.commandId!);
        setUnacceptedId(undefined);
        setHistoryOpen(true);
        await query.mutate();
      }, "原提交关闭未确认，请保留原命令核对"),
    retryOriginal: () =>
      execute(async () => {
        if (!canStart) throw new Error("画布访问已暂停，可先核对原运行回执");
        if (journal.data) await send(journal.data);
      }, "原流程命令重发未完成"),
    refresh: () =>
      execute(async () => {
        await query.mutate();
      }, "流程读取失败"),
    recoverOriginalTask: (node: CanvasContract.WorkflowRunNodeVO) =>
      execute(async () => {
        if (!node.taskId) throw new Error("原任务身份缺失");
        unwrap(await refreshGenerationTask({ taskId: node.taskId }));
        if (activeRef.current) await query.mutate();
      }, "原任务恢复未完成"),
    stop: (run: CanvasContract.WorkflowRunVO) =>
      execute(async () => {
        if (!run.runId) throw new Error("运行身份缺失");
        const commandId =
          stopCommands.current.get(run.runId) ?? crypto.randomUUID();
        stopCommands.current.set(run.runId, commandId);
        unwrap(await api.stopWorkflowRun({ runId: run.runId, commandId }));
        if (activeRef.current) await query.mutate();
      }, "停止回执未确认，请核对原运行"),
  };
}
