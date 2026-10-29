"use client";
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
        <Button type="button" variant="outline" disabled className="w-40 gap-2">
          <Spinner aria-hidden="true" />
          音色加载中
        </Button>
      ) : null}
      {owner.error ? (
        <div role="alert" className="flex flex-wrap items-center gap-2 text-sm">
          <span className="text-destructive">{owner.error}</span>
          {recovering ? (
            <Button
              type="button"
              variant="ghost"
              onClick={() => void owner.recover()}
            >
              恢复读取
            </Button>
          ) : null}
          {owner.phase === "unrecorded" ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => void owner.retryOriginal()}
            >
              重试原生成请求
            </Button>
          ) : null}
          {owner.phase === "import_pending" ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => void owner.retryOriginal()}
            >
              重试保存音频
            </Button>
          ) : null}
          {owner.phase === "failed" ? (
            <Button
              type="button"
              variant="ghost"
              onClick={() => void owner.begin()}
            >
              重新生成
            </Button>
          ) : null}
          {owner.phase === "unavailable" ? (
            <>
              <Button
                type="button"
                variant="ghost"
                onClick={() => void owner.begin()}
              >
                重试
              </Button>
            </>
          ) : null}
        </div>
      ) : null}
      {owner.active && owner.phase !== "options" ? (
        <p className="text-xs text-muted-foreground">
          关闭设置不会取消已提交的配音任务。
        </p>
      ) : null}
    </div>
  );
}
