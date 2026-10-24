"use client";

import { Button } from "@/components/ui/button";
import { ContentState } from "@/components/layout/content-state";

export function ProjectReadEmpty({
  onRetry,
  retrying,
  canvas = false,
}: {
  onRetry: () => void;
  retrying: boolean;
  canvas?: boolean;
}) {
  return (
    <ContentState
      className="min-h-40 py-6"
      state={retrying ? "loading" : "error"}
      title={
        retrying ? "正在读取" : canvas ? "画布暂时无法读取" : "项目暂时无法读取"
      }
      description="请重新读取，或稍后再试。"
      actions={
        <Button variant="outline" disabled={retrying} onClick={onRetry}>
          {retrying ? "正在读取" : "重新读取"}
        </Button>
      }
    />
  );
}

export function CommandRecovery({
  busy,
  onQuery,
  onRetry,
}: {
  busy: boolean;
  onQuery: () => void;
  onRetry: () => void;
}) {
  return (
    <div className="relative z-10 flex flex-wrap gap-2">
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={busy}
        onClick={onQuery}
      >
        查询操作结果
      </Button>
      <Button type="button" size="sm" disabled={busy} onClick={onRetry}>
        重试原操作
      </Button>
    </div>
  );
}
