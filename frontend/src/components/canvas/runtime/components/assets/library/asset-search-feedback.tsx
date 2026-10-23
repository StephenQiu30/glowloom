"use client";
import { Button } from "@/components/ui/button";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
/** Search availability is separate from the result of a source write. */
export function AssetSearchFeedback({
  state,
  retry,
}: {
  state: "SYNCING" | "UNAVAILABLE";
  retry: () => void;
}) {
  return (
    <Empty role="status" aria-live="polite">
      <EmptyHeader>
        <EmptyTitle>
          {state === "SYNCING" ? "素材正在同步" : "素材暂不可读取"}
        </EmptyTitle>
        <EmptyDescription>
          {state === "SYNCING"
            ? "列表尚未同步完成。自动等待已结束，可稍后重新读取；已完成的操作不受影响。"
            : "暂时无法读取素材列表，请稍后重试。筛选条件已保留。"}
        </EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        <Button variant="outline" onClick={retry}>
          重新读取
        </Button>
      </EmptyContent>
    </Empty>
  );
}
