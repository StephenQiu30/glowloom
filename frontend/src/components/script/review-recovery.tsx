"use client";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import type { useReviewWriter } from "./use-review-writer";
export function ReviewRecovery({
  writer,
}: {
  writer: ReturnType<typeof useReviewWriter>;
}) {
  const recovery = useRef<HTMLButtonElement>(null);
  const [inspect, setInspect] = useState(false);
  const hasRecovery = Boolean(writer.intent || writer.storageError);
  useEffect(() => {
    if (!hasRecovery || writer.busy) return;
    const active = document.activeElement;
    if (
      (active instanceof HTMLButtonElement ||
        active instanceof HTMLInputElement ||
        active instanceof HTMLTextAreaElement ||
        active instanceof HTMLSelectElement) &&
      active.matches(":disabled")
    )
      recovery.current?.focus();
  }, [hasRecovery, writer.busy, writer.intent?.key, writer.storageError]);
  if (!hasRecovery)
    return writer.error && !writer.rejected ? (
      <p role="alert">{writer.error}</p>
    ) : null;
  return (
    <section
      aria-label="未确认的分集与结构审核"
      className="space-y-3 rounded-lg border border-amber-500 p-4"
    >
      <p>
        这次分集或结构审核尚未确认。原键与完整原正文已保留；核验前不替换版本或提交新审核。
      </p>
      {writer.intent && (
        <>
          <p className="text-sm break-all">
            {writer.intent.action} · 原剧本版本{" "}
            {writer.intent.body.expected_revision} · 原键 {writer.intent.key}
          </p>
          <details onToggle={(event) => setInspect(event.currentTarget.open)}>
            <summary className="cursor-pointer">查看完整原审核正文</summary>
            {inspect && (
              <pre className="max-h-64 overflow-auto text-xs break-all whitespace-pre-wrap">
                {JSON.stringify(writer.intent.body, null, 2)}
              </pre>
            )}
          </details>
        </>
      )}
      {(writer.error || writer.storageError) && (
        <p role="alert">{writer.storageError ?? writer.error}</p>
      )}
      <Button
        ref={recovery}
        type="button"
        disabled={writer.busy}
        onClick={() =>
          void (writer.storageError ? writer.restoreStorage() : writer.replay())
        }
      >
        {writer.storageError ? "恢复原审核存储" : "人工使用原键核验审核"}
      </Button>
    </section>
  );
}
