"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import type { SourceIntent } from "./source-intent";

export function SourceIntentRecovery({
  intent,
  busy,
  storageError,
  error,
  onReplay,
  onStorageCheck,
  onReviewPending,
}: {
  intent: SourceIntent | null;
  busy: boolean;
  storageError?: string;
  error?: string;
  onReplay: () => void;
  onStorageCheck: () => void;
  onReviewPending?: () => void;
}) {
  const recovery = useRef<HTMLButtonElement>(null);
  const [inspect, setInspect] = useState(false);
  const hasRecovery = Boolean(intent || storageError);
  useEffect(() => {
    if (busy || !hasRecovery) return;
    const active = document.activeElement;
    if (
      active instanceof HTMLButtonElement ||
      active instanceof HTMLInputElement ||
      active instanceof HTMLTextAreaElement ||
      active instanceof HTMLSelectElement
    ) {
      if (active.disabled) recovery.current?.focus();
    }
  }, [intent?.key, busy, storageError, hasRecovery]);
  if (!hasRecovery) return null;
  const action =
    intent?.action === "create"
      ? "创建来源"
      : intent?.action === "update"
        ? "保存来源"
        : intent?.action === "delete"
          ? "删除来源"
          : intent?.action === "import"
            ? "批量导入"
            : "重排来源";
  return (
    <section
      aria-label="未确认的剧本保存"
      className="space-y-3 rounded-lg border border-amber-500 p-4"
    >
      <p className="font-medium">这次保存尚未确认，原请求已保留。</p>
      <p className="text-sm">
        核验完成前，编辑、新提交和关闭保持锁定。请人工使用同一原键与完整原正文核验；不会自动替换版本或重试。
      </p>
      {intent && (
        <>
          <p className="text-sm break-all">
            {action} · 原脚本版本 {intent.body.expected_revision} · 原键{" "}
            {intent.key}
          </p>
          {intent.action === "import" && (
            <p className="text-sm">
              原批次包含 {intent.body.sources.length} 个章节。
            </p>
          )}
          <details onToggle={(event) => setInspect(event.currentTarget.open)}>
            <summary className="cursor-pointer">查看完整原请求正文</summary>
            {inspect && (
              <pre
                aria-label="完整原请求正文"
                className="max-h-64 overflow-auto text-xs break-all whitespace-pre-wrap"
              >
                {JSON.stringify(intent.body, null, 2)}
              </pre>
            )}
          </details>
        </>
      )}
      {storageError && (
        <p role="alert" className="text-sm text-destructive">
          {storageError}
        </p>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {storageError ? (
        <Button
          ref={recovery}
          type="button"
          disabled={busy}
          onClick={onStorageCheck}
        >
          恢复存储并读取原意图
        </Button>
      ) : (
        <Button
          ref={recovery}
          type="button"
          disabled={busy || !intent}
          onClick={onReplay}
        >
          {busy ? "正在核验原保存…" : "人工使用原键核验保存"}
        </Button>
      )}
      {onReviewPending && (
        <Button type="button" variant="outline" onClick={onReviewPending}>
          查看未完成保存与核验记录
        </Button>
      )}
    </section>
  );
}
