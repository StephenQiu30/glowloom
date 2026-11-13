import { GenerationPlaceholder } from "@/components/generation/generation-placeholder";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import {
  generationFailurePresentation,
  type GenerationFailure,
} from "@/components/canvas/runtime/components/generation/lib/generation-failure";
import type { canvasMediaState } from "../lib/canvas-generation-projection";

export function CanvasMediaStatus({
  state,
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
  if (busy) return <GenerationPlaceholder />;
  const retry =
    action === "edit"
      ? !frozen && onEditInput
      : action === "retry"
        ? !frozen && onRetry
        : action === "query"
          ? onRefresh
          : undefined;
  return (
    <FeedbackToast
      message={label}
      action={
        retry
          ? {
              label:
                action === "edit"
                  ? "修改输入"
                  : action === "retry"
                    ? "重新生成"
                    : "重新查询",
              onClick: retry,
            }
          : undefined
      }
    />
  );
}
