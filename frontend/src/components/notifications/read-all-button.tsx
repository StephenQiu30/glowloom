"use client";

import { Button } from "@/components/ui/button";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { useNotificationCenter } from "./notification-provider";

export function ReadAllButton() {
  const center = useNotificationCenter();
  if (!center) return null;
  return (
    <div className="flex flex-col items-end gap-2">
      <Button
        variant="ghost"
        size="sm"
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
      {center.readAllError ? (
        <Alert>
          <AlertTitle>标记失败，未读消息已保留</AlertTitle>
        </Alert>
      ) : null}
    </div>
  );
}
