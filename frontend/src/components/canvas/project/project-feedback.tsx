"use client";
import { FolderOpen } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";

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
    <Empty className="min-h-40 py-6" aria-busy={retrying}>
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <FolderOpen />
        </EmptyMedia>
        <EmptyTitle>
          {canvas ? "暂无可展示的画布" : "暂无可展示的项目"}
        </EmptyTitle>
        <EmptyDescription>
          重新读取{canvas ? "画布" : "项目"}，或稍后再试。
        </EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        <Button variant="outline" disabled={retrying} onClick={onRetry}>
          {retrying ? "正在读取" : "重新读取"}
        </Button>
      </EmptyContent>
    </Empty>
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
