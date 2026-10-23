"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Play, Square, Workflow } from "lucide-react";
import type { useCanvasWorkflow } from "../hooks/use-canvas-workflow";
import { readCanvasFilePreview } from "../lib/canvas-file-preview";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";

const states: Record<string, string> = {
  WAITING: "等待依赖",
  REQUESTED: "等待业务受理",
  QUEUED: "已排队",
  RUNNING: "处理中",
  SUBMITTING: "正在提交",
  WAITING_PROVIDER: "等待原任务",
  ARCHIVING: "登记原产物",
  SUCCEEDED: "已完成",
  FAILED: "失败",
  BLOCKED: "上游失败",
  STOPPED: "已停止",
  UNKNOWN: "提交结果待核对",
};
const resolutions: Record<string, string> = {
  NONE: "执行中",
  SUCCEEDED: "已完成",
  PARTIAL: "部分完成",
  FAILED: "失败",
  STOPPED: "已停止",
  NEEDS_RECONCILIATION: "存在待核对提交",
};
type Controller = ReturnType<typeof useCanvasWorkflow>;

/** 运行展示独立于画布草稿；收到结果不会自动覆盖用户节点。 */
export function CanvasWorkflowControls({
  workflow,
  allNodeIds,
}: {
  workflow: Controller;
  allNodeIds: string[];
}) {
  if (!workflow.available) return null;
  const preview = workflow.preview;
  const selected = new Set(preview?.request.nodeIds ?? []);
  for (const node of preview?.document.graph.nodes ?? [])
    if (node.parentId && selected.has(node.parentId)) selected.add(node.id);
  const included =
    preview?.document.graph.nodes.filter(
      (node) => node.type !== "group" && selected.has(node.id),
    ) ?? [];
  const generated = included.some(
    (node) => node.parameters?.mode === "GENERATE",
  );
  const fixedIds = new Set(
    preview?.document.graph.connections
      .filter(
        (edge) => selected.has(edge.toNodeId) && !selected.has(edge.fromNodeId),
      )
      .map((edge) => edge.fromNodeId) ?? [],
  );
  const fixed =
    preview?.document.graph.nodes.filter((node) => fixedIds.has(node.id)) ?? [];
  const active = workflow.runs.find((run) => run.lifecycle !== "CLOSED");
  return (
    <>
      <Button
        variant="ghost"
        size="sm"
        onClick={() => workflow.setHistoryOpen(true)}
      >
        <Workflow />
        流程
      </Button>
      <Button
        variant="ghost"
        size="sm"
        disabled={workflow.busy || !workflow.integration.accepting || !!active}
        onClick={() => void workflow.integration.choose(allNodeIds)}
      >
        <Play />
        运行画布
      </Button>
      {active && !active.stopRequested && (
        <Button
          variant="ghost"
          size="sm"
          disabled={workflow.busy}
          onClick={() => void workflow.stop(active)}
        >
          <Square />
          停止流程
        </Button>
      )}
      <Dialog
        open={!!preview}
        onOpenChange={(open) => {
          if (!open) workflow.cancelPreview();
        }}
      >
        <DialogContent className="flex max-h-[85dvh] flex-col">
          <DialogHeader>
            <DialogTitle>确认运行范围</DialogTitle>
            <DialogDescription>
              使用已保存的画布快照。运行中的编辑不会改变原流程，也不会自动采用结果。
            </DialogDescription>
          </DialogHeader>
          <div className="min-h-0 space-y-3 overflow-y-auto text-sm">
            <p>画布版本：{String(preview?.document.revision ?? "")}</p>
            {included.map((node) => (
              <div key={node.id} className="rounded-lg border p-3">
                <p>
                  {node.title} ·{" "}
                  {node.parameters?.mode === "AGENT"
                    ? "Agent（Mock）"
                    : node.parameters?.mode === "PROCESS"
                      ? "确定性拼接"
                      : node.parameters?.mode === "GENERATE"
                        ? "模型生成"
                        : "固定输入"}
                </p>
                {node.fileReferences?.map((file) => (
                  <p
                    key={file.referenceKey}
                    className="text-xs break-all text-muted-foreground"
                  >
                    素材版本：{file.version}
                  </p>
                ))}
              </div>
            ))}
            {fixed.length > 0 && (
              <div className="rounded-lg border p-3">
                <p>范围外输入</p>
                {fixed.map((node) => (
                  <p key={node.id}>
                    {node.title} · 按冻结连线使用固定值或固定文件版本
                  </p>
                ))}
              </div>
            )}
            <p className="text-muted-foreground">
              {generated
                ? "包含模型生成。停止后，已提交的供应商任务仍需按原任务核对，不表示撤销或退费。"
                : "所选固定输入、确定性拼接与 Agent Mock 不调用付费模型。"}
            </p>
            {generated && !workflow.options?.generationEnabled && (
              <p role="alert">
                当前PoC未开放收费生成，请调整运行范围或使用确定性处理。
              </p>
            )}
          </div>
          <div className="flex justify-end gap-2">
            <Button
              variant="outline"
              disabled={workflow.busy}
              onClick={workflow.cancelPreview}
            >
              返回编辑
            </Button>
            <Button
              disabled={
                workflow.busy ||
                !!workflow.pending ||
                (generated && !workflow.options?.generationEnabled)
              }
              onClick={() => void workflow.confirm()}
            >
              {workflow.busy ? "正在受理" : "确认运行"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
      <Dialog
        open={workflow.historyOpen}
        onOpenChange={workflow.setHistoryOpen}
      >
        <DialogContent className="flex max-h-[85dvh] flex-col sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>画布流程运行</DialogTitle>
            <DialogDescription>
              关闭页面不影响后台执行。停止只关闭新的执行准入，原在途任务会保留真实回执。
            </DialogDescription>
          </DialogHeader>
          <div className="min-h-0 space-y-4 overflow-y-auto">
            {workflow.pending && (
              <section className="space-y-2 rounded-lg border p-3">
                <p role="status">
                  原提交尚未确认，请按原命令核对，避免创建第二次运行。
                </p>
                {workflow.unaccepted && (
                  <p className="text-sm text-muted-foreground">
                    暂未找到已受理回执，原请求可能仍在处理中。重发会保留相同命令和快照。
                  </p>
                )}
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    disabled={workflow.busy}
                    onClick={() => void workflow.recover()}
                  >
                    核对原回执
                  </Button>
                  <Button
                    variant="outline"
                    disabled={workflow.busy}
                    onClick={() => void workflow.retryOriginal()}
                  >
                    重发原命令
                  </Button>
                  {workflow.unaccepted && (
                    <Button
                      variant="outline"
                      disabled={workflow.busy}
                      onClick={() => void workflow.closeSubmission()}
                    >
                      关闭未受理提交
                    </Button>
                  )}
                </div>
              </section>
            )}
            {workflow.error && (
              <p role="alert">流程状态读取失败，原运行仍保留。请重新读取。</p>
            )}
            {workflow.runs.length === 0 && !workflow.error && (
              <p className="text-sm text-muted-foreground">
                这张画布还没有流程运行。选择节点或分组，也可以运行整张画布。
              </p>
            )}
            {workflow.runs.map((run) => (
              <section
                key={run.runId}
                className="space-y-3 rounded-lg border p-3"
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <p className="text-sm">
                    版本 {String(run.graphRevision)} ·{" "}
                    {run.lifecycle === "STOPPING"
                      ? "正在停止"
                      : (resolutions[run.resolution ?? ""] ?? run.lifecycle)}
                  </p>
                  {run.lifecycle !== "CLOSED" && !run.stopRequested && (
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={workflow.busy}
                      onClick={() => void workflow.stop(run)}
                    >
                      停止此流程
                    </Button>
                  )}
                </div>
                {run.stopRequested && (
                  <p role="status" className="text-sm text-muted-foreground">
                    {run.lifecycle === "CLOSED"
                      ? "流程已停止，原运行结果已保留。"
                      : run.gateApplied
                        ? "停止门禁已生效，等待在途回执。"
                        : "停止已受理，等待执行门禁确认。"}
                  </p>
                )}
                {run.resolution === "NEEDS_RECONCILIATION" && (
                  <p role="alert" className="text-sm">
                    {run.nodes?.some((node) => node.state === "UNKNOWN")
                      ? "提交结果未知，请按原任务核对；该流程不会自动再次生成。"
                      : "引擎状态需要核对，原任务与停止门禁已保留；该流程不会自动重新启动。"}
                  </p>
                )}
                {(run.nodes ?? []).map((node) => (
                  <div
                    key={node.nodeRunId}
                    className="space-y-1 border-t pt-2 text-sm"
                  >
                    <p>
                      {node.title ?? node.canvasNodeId} ·{" "}
                      {states[node.state ?? ""] ?? node.state}
                    </p>
                    {node.textOutput && (
                      <pre
                        tabIndex={0}
                        aria-label="原流程文字结果"
                        className="max-h-40 overflow-auto rounded-md bg-muted p-2 text-sm break-words whitespace-pre-wrap"
                      >
                        {node.textOutput}
                      </pre>
                    )}
                    {node.taskId && (
                      <p className="text-xs break-all text-muted-foreground">
                        原任务：{node.taskId}
                      </p>
                    )}
                    {node.structuredOutput && (
                      <pre
                        tabIndex={0}
                        aria-label="原流程结构化结果"
                        className="max-h-64 overflow-auto rounded-md bg-muted p-2 text-sm whitespace-pre-wrap"
                      >
                        {JSON.stringify(node.structuredOutput, null, 2)}
                      </pre>
                    )}
                    {(node.outputs ?? []).map((file) => (
                      <WorkflowFileButton
                        key={file.fileId + ":" + file.version}
                        file={file}
                      />
                    ))}
                    {node.taskId &&
                      node.failureKind &&
                      ["ARCHIVING", "WAITING_PROVIDER"].includes(
                        node.state ?? "",
                      ) && (
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={workflow.busy}
                          onClick={() =>
                            void workflow.recoverOriginalTask(node)
                          }
                        >
                          恢复原任务
                        </Button>
                      )}
                    <Popover>
                      <PopoverTrigger asChild>
                        <Button variant="ghost" size="sm">
                          来源与版本
                        </Button>
                      </PopoverTrigger>
                      <PopoverContent
                        align="start"
                        className="max-h-[60dvh] overflow-auto text-xs text-muted-foreground"
                      >
                        <p className="break-all">原运行：{run.runId}</p>
                        {node.inputHash && (
                          <p className="break-all">
                            实际输入摘要：{node.inputHash}
                          </p>
                        )}
                        {(node.outputs ?? []).map((file) => (
                          <p
                            key={file.fileId + ":" + file.version}
                            className="break-all"
                          >
                            {file.mediaType} · 固定版本 {file.version}
                            {file.sha256 ? " · SHA256 " + file.sha256 : ""}
                          </p>
                        ))}
                      </PopoverContent>
                    </Popover>
                  </div>
                ))}
              </section>
            ))}
          </div>
          <div className="flex justify-end">
            <Button
              variant="outline"
              disabled={workflow.busy}
              onClick={() => void workflow.refresh()}
            >
              重新读取状态
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}

function WorkflowFileButton({
  file,
}: {
  file: CanvasContract.WorkflowRunFileVO;
}) {
  const [busy, setBusy] = useState(false);
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const read = async () => {
    if (busy || !file.fileId || !file.version) return;
    setBusy(true);
    try {
      const url = await readCanvasFilePreview({
        fileId: file.fileId,
        version: file.version,
      });
      if (!active.current) return;
      const link = document.createElement("a");
      link.href = url;
      link.download = "";
      link.rel = "noopener";
      link.click();
    } catch (error) {
      if (active.current) notifyError(error, "原素材读取失败");
    } finally {
      if (active.current) setBusy(false);
    }
  };
  return (
    <Button
      variant="outline"
      size="sm"
      disabled={busy || !file.fileId || !file.version}
      onClick={() => void read()}
    >
      {busy ? "正在授权" : "下载原素材"}
    </Button>
  );
}
