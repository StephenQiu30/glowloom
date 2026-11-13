"use client";
import { FeedbackAlert } from "@/components/feedback/feedback-alert";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import type { useSubjectVoiceJob } from "./unified-voice-job";
type Owner = ReturnType<typeof useSubjectVoiceJob>;
export function UnifiedSubjectVoiceControls({ owner }: { owner: Owner }) {
  if (owner.phase === "idle" || owner.phase === "ready") return null;
  const recovering = ["unknown", "read_error", "unrecorded"].includes(
    owner.phase,
  );
  const loading = ["options", "submitting", "waiting"].includes(owner.phase);
  return (
    <div className="flex min-w-0 flex-1 flex-col gap-2">
      {loading ? (
        <Button
          type="button"
          variant="outline"
          size="feedback"
          disabled
          className="w-40 gap-2"
        >
          <Spinner aria-hidden="true" />
          音色加载中
        </Button>
      ) : null}
      {owner.error ? (
        <FeedbackAlert
          title={
            owner.phase === "import_pending"
              ? "音频尚未保存"
              : recovering
                ? "配音结果待确认"
                : owner.phase === "unavailable"
                  ? "智能配音暂不可用"
                  : "配音失败"
          }
          tone={
            recovering || owner.phase === "import_pending" ? "info" : "error"
          }
          description={
            owner.phase === "import_pending"
              ? "重试保存即可，无需重新生成。"
              : recovering
                ? "请先查看状态，避免重复生成。"
                : undefined
          }
          actions={
            <>
              {recovering ? (
                <Button
                  type="button"
                  variant="outline"
                  size="feedback"
                  onClick={() => void owner.recover()}
                >
                  查看状态
                </Button>
              ) : null}
              {owner.phase === "unrecorded" ? (
                <Button
                  type="button"
                  variant="outline"
                  size="feedback"
                  onClick={() => void owner.retryOriginal()}
                >
                  重试生成
                </Button>
              ) : null}
              {owner.phase === "import_pending" ? (
                <Button
                  type="button"
                  variant="outline"
                  size="feedback"
                  onClick={() => void owner.retryOriginal()}
                >
                  重试保存
                </Button>
              ) : null}
              {owner.phase === "failed" || owner.phase === "unavailable" ? (
                <Button
                  type="button"
                  variant="outline"
                  size="feedback"
                  onClick={() => void owner.begin()}
                >
                  {owner.phase === "failed" ? "重新生成" : "重试"}
                </Button>
              ) : null}
            </>
          }
        />
      ) : null}
      {owner.active && owner.phase !== "options" ? (
        <p className="text-xs text-muted-foreground">
          关闭设置不会取消已提交的配音任务。
        </p>
      ) : null}
    </div>
  );
}
