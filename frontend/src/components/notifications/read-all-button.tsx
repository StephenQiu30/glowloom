"use client";

import { Button } from "@/components/ui/button";
import { useNotificationCenter } from "./notification-provider";

export function ReadAllButton() {
  const center = useNotificationCenter();
  if (!center) return null;
  return (
    <Button
      variant="ghost"
      size="feedback"
      disabled={
        center.readAllPending ||
        !center.summary ||
        (!center.readAllError && center.summary.unread_count === 0)
      }
      onClick={() => void center.readAll()}
    >
      {center.readAllPending
        ? "标记中…"
        : center.readAllError
          ? "重试全部已读"
          : "全部已读"}
    </Button>
  );
}
