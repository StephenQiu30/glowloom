"use client";

import { useEffect, useRef, useState } from "react";
import {
  adoptCanvasStoryboardShot,
  listCanvasStoryboardCandidates,
  quoteCanvasStoryboardShot,
} from "@/api/canvases";
import { listProjectModels } from "@/api/models";
import { confirmOperationQuote, getOperation } from "@/api/operations";
import { FeedbackAlert } from "@/components/feedback/feedback-alert";
import {
  activeTask,
  generationStatusLabel,
  requireQuote,
  requireTask,
  type Quote,
  type Task,
} from "@/components/generation/contracts";
import { modelAvailableForMode } from "@/components/generation/model-availability";
import { ModelPicker } from "@/components/generation/model-picker";
import { QuoteCostDetail } from "@/components/generation/quote-cost-detail";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { isProjectID } from "@/components/projects/project";
import { Button } from "@/components/ui/button";
import { readErrorMessage } from "@/lib/read-error";
import { RequestError } from "@/lib/request";
import type { CanvasStoryboardShot } from "./canvas-storyboard-state";

const mode = "text2video";
type QuoteIntent = {
  key: string;
  revision: number;
  shotRevision: number;
  modelKey: string;
};
type AdoptIntent = {
  key: string;
  revision: number;
  shotRevision: number;
  operationID: string;
  outputID: string;
};
type QuoteDisplay = {
  intent: QuoteIntent;
  quote: Quote;
};
type DeliveryAction = "quote" | "confirm" | "check" | "adopt";

function accepted(task: Task) {
  return !["draft", "quoted", "expired"].includes(task.status);
}
function quoteUnexpired(expiresAt: string) {
  return Date.parse(expiresAt) > Date.now();
}
function uncertain(error: unknown) {
  return (
    !(error instanceof RequestError) ||
    error.status === 0 ||
    error.status === 408 ||
    error.status === 429 ||
    error.status >= 500
  );
}

export function CanvasStoryboardDelivery({
  active,
  projectID,
  canvasID,
  boardID,
  revision,
  shot,
  writable,
  onAdopted,
  onStale,
}: {
  active: boolean;
  projectID: string;
  canvasID: string;
  boardID: string;
  revision: number;
  shot: CanvasStoryboardShot;
  writable: boolean;
  onAdopted: (view: API.StoryboardView) => void;
  onStale: () => void;
}) {
  const shotID = shot.id;
  const [models, setModels] = useState<API.ModelResponse[]>();
  const [modelError, setModelError] = useState("");
  const [modelRetry, setModelRetry] = useState(0);
  const [modelKey, setModelKey] = useState("");
  const [candidates, setCandidates] = useState<API.TaskSummary[]>();
  const [candidateError, setCandidateError] = useState("");
  const [candidateRefresh, setCandidateRefresh] = useState(0);
  const [selectedTask, setSelectedTask] = useState("");
  const [preview, setPreview] = useState<Task>();
  const [previewError, setPreviewError] = useState("");
  const [quote, setQuote] = useState<QuoteDisplay>();
  const [quotePending, setQuotePending] = useState<QuoteIntent>();
  const [confirmUnknown, setConfirmUnknown] = useState(false);
  const [adoptPending, setAdoptPending] = useState<AdoptIntent>();
  const [feedback, setFeedback] = useState("");
  const [busy, setBusy] = useState<DeliveryAction>();
  const action = useRef<AbortController | null>(null);
  const selectedModel =
    models?.find((item) => item.key === modelKey) ?? models?.[0];
  const currentQuote =
    quote?.intent.shotRevision === shot.revision &&
    quote.intent.modelKey === selectedModel?.key;
  const tasksActive =
    candidates?.some((task) => activeTask(task.status)) ?? false;

  useEffect(() => () => action.current?.abort(), []);
  useEffect(() => {
    if (!active || !writable) return;
    const controller = new AbortController();
    void listProjectModels(
      { project_id: projectID, capability: "video.generate", mode, limit: 200 },
      { signal: controller.signal },
    )
      .then((page) => {
        if (!controller.signal.aborted) {
          setModelError("");
          setModels(
            (page.items ?? []).filter(
              (item) => !!item.key && modelAvailableForMode(item, mode),
            ),
          );
        }
      })
      .catch((error) => {
        if (!controller.signal.aborted) setModelError(readErrorMessage(error));
      });
    return () => controller.abort();
  }, [active, writable, projectID, modelRetry]);

  useEffect(() => {
    if (!active) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function load() {
      try {
        const result = await listCanvasStoryboardCandidates(
          { id: canvasID, board_id: boardID, shot_id: shotID },
          { signal: controller.signal },
        );
        if (controller.signal.aborted) return;
        if (
          !Array.isArray(result) ||
          result.some(
            (task) =>
              !isProjectID(task.id) ||
              task.project_id !== projectID ||
              task.source?.canvas_id !== canvasID ||
              task.source.node_id !== boardID ||
              task.source.shot_id !== shotID,
          )
        )
          throw new RequestError("invalid_receipt");
        setCandidates(result);
        setCandidateError("");
        if (result.some((task) => activeTask(task.status)))
          timer = setTimeout(() => void load(), 4000);
      } catch (error) {
        if (controller.signal.aborted) return;
        setCandidateError(readErrorMessage(error));
        if (
          !(error instanceof RequestError) ||
          ![401, 403, 404].includes(error.status)
        )
          timer = setTimeout(() => void load(), 8000);
        else setCandidates(undefined);
      }
    }
    void load();
    return () => {
      controller.abort();
      if (timer) clearTimeout(timer);
    };
  }, [active, projectID, canvasID, boardID, shotID, candidateRefresh]);

  useEffect(() => {
    if (!active || !selectedTask) return;
    const controller = new AbortController();
    void getOperation(
      { id: selectedTask, project_id: projectID },
      { signal: controller.signal },
    )
      .then((response) => {
        if (controller.signal.aborted) return;
        const task = requireTask(response, projectID);
        if (
          task.id !== selectedTask ||
          task.source?.canvas_id !== canvasID ||
          task.source.node_id !== boardID ||
          task.source.shot_id !== shotID
        )
          throw new RequestError("invalid_receipt");
        setPreview(task);
        setPreviewError("");
      })
      .catch((error) => {
        if (!controller.signal.aborted) {
          setPreview(undefined);
          setPreviewError(readErrorMessage(error));
        }
      });
    return () => controller.abort();
  }, [
    active,
    selectedTask,
    projectID,
    canvasID,
    boardID,
    shotID,
    candidateRefresh,
    tasksActive,
  ]);

  async function perform(
    name: DeliveryAction,
    callback: (signal: AbortSignal) => Promise<void>,
    failure: (error: unknown) => string,
  ) {
    if (action.current) return;
    const controller = new AbortController();
    action.current = controller;
    setBusy(name);
    setFeedback("");
    try {
      await callback(controller.signal);
    } catch (error) {
      if (!controller.signal.aborted) setFeedback(failure(error));
    } finally {
      if (!controller.signal.aborted) {
        action.current = null;
        setBusy(undefined);
      }
    }
  }

  function requestQuote() {
    if (!writable || !selectedModel?.key || action.current || confirmUnknown)
      return;
    const intent = quotePending ?? {
      key: crypto.randomUUID(),
      revision,
      shotRevision: shot.revision,
      modelKey: selectedModel.key,
    };
    setQuotePending(intent);
    void perform(
      "quote",
      async (signal) => {
        const response = await quoteCanvasStoryboardShot(
          { id: canvasID },
          {
            expected_revision: intent.revision,
            board_id: boardID,
            shot_id: shotID,
            shot_revision: intent.shotRevision,
            model_key: intent.modelKey,
          },
          {
            signal,
            headers: {
              "Content-Type": "application/json",
              "Idempotency-Key": intent.key,
            },
          },
        );
        if (signal.aborted) return;
        setQuote({ quote: requireQuote(response), intent });
        setQuotePending(undefined);
      },
      (error) => {
        if (uncertain(error)) return "报价结果待确认，请重试原报价。";
        setQuotePending(undefined);
        if (error instanceof RequestError && error.status === 409) {
          onStale();
          return "故事板、模型或任务状态已变化，请重新报价。";
        }
        return readErrorMessage(error);
      },
    );
  }

  function confirm() {
    if (!writable || !quote?.quote.confirm_key || action.current) return;
    const current = quote.quote;
    const confirmKey = current.confirm_key!;
    if (
      !confirmUnknown &&
      (!currentQuote ||
        !current.confirmable ||
        !quoteUnexpired(current.expires_at))
    ) {
      setFeedback("报价已过期或镜头已变化，请重新报价。");
      return;
    }
    setConfirmUnknown(true);
    void perform(
      "confirm",
      async (signal) => {
        const response = await confirmOperationQuote(
          { id: current.operation_id },
          { project_id: projectID },
          {
            signal,
            headers: {
              "Content-Type": "application/json",
              "Idempotency-Key": confirmKey,
            },
          },
        );
        if (signal.aborted) return;
        if (!response.reservation_id) throw new RequestError("invalid_receipt");
        setConfirmUnknown(false);
        setQuote(undefined);
        setSelectedTask(current.operation_id);
        setCandidateRefresh((version) => version + 1);
      },
      (error) => {
        if (uncertain(error)) return "确认结果待核对，请查询原任务。";
        setConfirmUnknown(false);
        if (
          error instanceof RequestError &&
          [409, 410].includes(error.status)
        ) {
          onStale();
          setQuote(undefined);
          return "报价已失效，请重新读取镜头后报价。";
        }
        return readErrorMessage(error);
      },
    );
  }

  function checkConfirmation() {
    if (!quote) return;
    const operationID = quote.quote.operation_id;
    void perform(
      "check",
      async (signal) => {
        const task = requireTask(
          await getOperation(
            { id: operationID, project_id: projectID },
            { signal },
          ),
          projectID,
        );
        if (signal.aborted) return;
        if (task.id !== operationID) throw new RequestError("invalid_receipt");
        if (!accepted(task)) {
          setFeedback("原任务尚未确认，可继续核对或重试原确认。");
          return;
        }
        setConfirmUnknown(false);
        setQuote(undefined);
        setPreview(task);
        setSelectedTask(task.id);
        setCandidateRefresh((version) => version + 1);
      },
      readErrorMessage,
    );
  }

  function adopt(intent: AdoptIntent) {
    if (!writable || action.current) return;
    setAdoptPending(intent);
    void perform(
      "adopt",
      async (signal) => {
        const result = await adoptCanvasStoryboardShot(
          { id: canvasID },
          {
            expected_revision: intent.revision,
            board_id: boardID,
            shot_id: shotID,
            shot_revision: intent.shotRevision,
            operation_id: intent.operationID,
            output_id: intent.outputID,
          },
          {
            signal,
            headers: {
              "Content-Type": "application/json",
              "Idempotency-Key": intent.key,
            },
          },
        );
        if (signal.aborted) return;
        onAdopted(result);
        setAdoptPending(undefined);
      },
      (error) => {
        if (uncertain(error)) return "采用结果待确认，可重试原采用操作。";
        setAdoptPending(undefined);
        if (error instanceof RequestError && error.status === 409) {
          onStale();
          return "镜头或候选已变化，请核对当前版本。";
        }
        return readErrorMessage(error);
      },
    );
  }

  const task = preview?.id === selectedTask ? preview : undefined;
  const oldCandidate = task?.source?.shot_revision !== shot.revision;
  const outputs = task?.outputs?.filter(
    (output) =>
      output.kind === "video" &&
      output.moderation_status === "passed" &&
      isProjectID(output.media_asset_id) &&
      isProjectID(output.id),
  );

  return (
    <section aria-label="镜头样片" className="space-y-3 border-t pt-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="font-medium">镜头样片</h3>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setCandidateRefresh((version) => version + 1)}
        >
          刷新任务
        </Button>
      </div>
      {modelError ? (
        <FeedbackAlert
          tone="error"
          title={modelError}
          actions={
            <Button
              variant="outline"
              size="feedback"
              onClick={() => setModelRetry((version) => version + 1)}
            >
              重读模型
            </Button>
          }
        />
      ) : writable && models ? (
        models.length ? (
          <div className="flex flex-wrap items-center gap-2">
            <ModelPicker
              compact
              label="样片视频模型"
              models={models}
              mode={mode}
              value={selectedModel?.current_version?.id}
              disabled={!!busy || confirmUnknown || !!quotePending}
              onSelect={(model) => setModelKey(model.key ?? "")}
            />
            <Button
              variant="outline"
              size="sm"
              disabled={!!busy || confirmUnknown || tasksActive}
              onClick={requestQuote}
            >
              {quotePending ? "重试报价" : quote ? "重新报价" : "查看报价"}
            </Button>
          </div>
        ) : (
          <p className="text-muted-foreground">当前没有可用的视频模型。</p>
        )
      ) : null}
      {quote ? (
        <div className="space-y-2 rounded-md border bg-muted/40 p-3">
          <p className="text-muted-foreground">
            {models?.find((item) => item.key === quote.intent.modelKey)
              ?.display_name ?? "视频模型"}{" "}
            · 文生视频{currentQuote ? ` · ${shot.duration_ms / 1000} 秒` : ""}
          </p>
          <QuoteCostDetail
            amount={quote.quote.quote_micros}
            detail={quote.quote.quote_detail}
          />
          {confirmUnknown ? (
            <div className="flex flex-wrap gap-2">
              <Button size="sm" disabled={!!busy} onClick={checkConfirmation}>
                查询原任务
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={!!busy || !writable}
                onClick={confirm}
              >
                重试原确认
              </Button>
            </div>
          ) : (
            <Button
              size="sm"
              disabled={
                !writable ||
                !!busy ||
                !currentQuote ||
                !quote.quote.confirmable ||
                !quote.quote.confirm_key
              }
              onClick={confirm}
            >
              确认制作此镜
            </Button>
          )}
        </div>
      ) : null}
      {feedback ? (
        <FeedbackAlert
          title={feedback}
          tone={
            confirmUnknown || adoptPending || quotePending ? "info" : "error"
          }
        />
      ) : null}
      {candidateError ? (
        <FeedbackAlert title={candidateError} tone="error" />
      ) : null}
      {candidates?.length ? (
        <ul aria-label="样片任务" className="space-y-2">
          {candidates.map((candidate) => (
            <li key={candidate.id}>
              <Button
                aria-pressed={selectedTask === candidate.id}
                variant={
                  selectedTask === candidate.id ? "secondary" : "outline"
                }
                size="sm"
                className="h-auto min-h-9 w-full flex-wrap justify-start py-1 text-left"
                onClick={() => {
                  setSelectedTask(candidate.id!);
                  setPreviewError("");
                }}
              >
                {candidate.model_name || "视频样片"} ·{" "}
                {generationStatusLabel(candidate) || "正在同步"}
                {candidate.source?.shot_revision !== shot.revision
                  ? " · 旧稿"
                  : ""}
                {candidate.create_time ? (
                  <time
                    className="text-xs text-muted-foreground"
                    dateTime={candidate.create_time}
                  >
                    {new Date(candidate.create_time).toLocaleString()}
                  </time>
                ) : null}
              </Button>
            </li>
          ))}
        </ul>
      ) : null}
      {previewError ? (
        <FeedbackAlert title={previewError} tone="error" />
      ) : null}
      {task ? (
        <div className="space-y-2">
          <p className="text-muted-foreground">
            {generationStatusLabel(task) || "正在同步任务"}
          </p>
          {outputs?.map((output) => {
            const adopted =
              shot.adopted?.operation_id === task.id &&
              shot.adopted.output_id === output.id;
            return (
              <div key={output.id} className="space-y-2">
                <div className="relative aspect-video max-h-60 overflow-hidden rounded-md bg-muted">
                  <ProjectMediaPreview
                    projectID={projectID}
                    id={output.media_asset_id!}
                    kind="video"
                    interactive
                    fit="contain"
                  />
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={
                    !writable ||
                    !!busy ||
                    adopted ||
                    oldCandidate ||
                    !!adoptPending
                  }
                  onClick={() =>
                    adopt({
                      key: crypto.randomUUID(),
                      revision,
                      shotRevision: shot.revision,
                      operationID: task.id,
                      outputID: output.id!,
                    })
                  }
                >
                  {adopted
                    ? "已采用"
                    : oldCandidate
                      ? "候选来自旧稿"
                      : "采用此视频"}
                </Button>
              </div>
            );
          })}
        </div>
      ) : null}
      {adoptPending ? (
        <Button
          variant="outline"
          size="sm"
          disabled={!writable || !!busy}
          onClick={() => adopt(adoptPending)}
        >
          重试原采用
        </Button>
      ) : null}
    </section>
  );
}
