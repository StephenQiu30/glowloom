import { CircleAlert } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { cn } from "@/lib/utils";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  generationFailurePresentation,
  type GenerationFailure,
} from "@/components/canvas/runtime/components/generation/lib/generation-failure";
import type { canvasMediaState } from "../lib/canvas-generation-projection";

export function CanvasMediaStatus({
  state,
  color,
  run,
  frozen = false,
  onEditInput,
  onRefresh,
  onRetry,
}: {
  state: NonNullable<ReturnType<typeof canvasMediaState>>;
  color: string;
  run?: GenerationFailure & { taskId?: number | string };
  frozen?: boolean;
  onEditInput?: () => void;
  onRefresh?: () => void;
  onRetry?: () => void;
}) {
  const [detailsOpen, setDetailsOpen] = useState(false);
  const busy = state === "generating" || state === "loading";
  const failure = generationFailurePresentation(run);
  const label = {
    generating: "生成中",
    loading: "加载中",
    "preview-error": "加载失败",
    "archive-error": "保存失败",
    unconfirmed: "状态待确认",
    error: failure.reason,
  }[state];
  const action = state === "error" ? failure.action : "query";
  const disabled =
    (frozen && (action === "edit" || action === "retry")) ||
    (action === "edit" && !onEditInput) ||
    (action === "query" && !onRefresh) ||
    (action === "retry" && !onRetry);
  const diagnostic = run?.failureDiagnostic;
  return (
    <div
      className={cn(
        "flex h-full w-full flex-col items-center justify-center text-center",
        busy ? "gap-3 p-4" : "gap-2 overflow-y-auto p-2",
      )}
      style={{ color }}
      aria-busy={busy}
      data-canvas-no-zoom
      onPointerDown={(event) => event.stopPropagation()}
      onClick={(event) => event.stopPropagation()}
      onKeyDown={(event) => event.stopPropagation()}
      onWheel={(event) => event.stopPropagation()}
    >
      {busy ? (
        <Spinner
          className="size-6 motion-reduce:animate-none"
          aria-hidden="true"
        />
      ) : (
        <CircleAlert className="size-6" aria-hidden="true" />
      )}
      <span role="status" className="text-sm leading-5">
        {label}
      </span>
      {!busy && (
        <Button
          variant="outline"
          size="sm"
          disabled={disabled}
          onClick={(event) => {
            event.stopPropagation();
            if (disabled) return;
            if (action === "edit") onEditInput?.();
            else if (action === "retry") onRetry?.();
            else if (action === "details") setDetailsOpen(true);
            else onRefresh?.();
          }}
        >
          {action === "edit"
            ? "修改输入"
            : action === "details"
              ? "查看原因"
              : action === "retry"
                ? "稍后重试"
                : state === "unconfirmed" || state === "error"
                  ? "确认状态"
                  : "重新加载"}
        </Button>
      )}
      {!busy && run && (
        <Collapsible
          className="w-full text-xs leading-5"
          open={detailsOpen}
          onOpenChange={setDetailsOpen}
        >
          <CollapsibleTrigger asChild>
            <Button variant="ghost" size="sm">
              任务详情
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <dl className="max-h-24 overflow-y-auto text-left break-all">
              {run.taskId != null && (
                <>
                  <dt>任务编号</dt>
                  <dd>{String(run.taskId)}</dd>
                </>
              )}
              {diagnostic?.providerCode && (
                <>
                  <dt>错误码</dt>
                  <dd>{diagnostic.providerCode}</dd>
                </>
              )}
              {diagnostic?.httpStatus != null && (
                <>
                  <dt>HTTP 状态</dt>
                  <dd>{diagnostic.httpStatus}</dd>
                </>
              )}
              {diagnostic?.providerRequestId && (
                <>
                  <dt>供应商请求编号</dt>
                  <dd>{diagnostic.providerRequestId}</dd>
                </>
              )}
              {!diagnostic && (
                <>
                  <dt>诊断</dt>
                  <dd>未保存具体原因</dd>
                </>
              )}
            </dl>
          </CollapsibleContent>
        </Collapsible>
      )}
    </div>
  );
}
