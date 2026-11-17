"use client";

import { Bell } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useNotificationCenter } from "./notification-provider";

export { ReadAllButton } from "./read-all-button";

export function NotificationBell({ onLogin }: { onLogin: () => void }) {
  const center = useNotificationCenter();
  const count = center?.summary?.unread_count;
  return (
    <Button
      variant="ghost"
      size="toolbar-icon"
      className="relative"
      data-notification-trigger
      aria-label={
        !center?.signedIn
          ? "消息中心"
          : count === undefined
            ? "消息中心，未读数量暂不可用"
            : `消息中心，${count}条未读`
      }
      aria-expanded={center?.inboxOpen ?? false}
      aria-haspopup="dialog"
      onClick={() =>
        center?.signedIn ? center.setInboxOpen(!center.inboxOpen) : onLogin()
      }
    >
      <Bell data-icon="inline-start" aria-hidden="true" />
      {count ? (
        <Badge variant="secondary" className="absolute -top-1 -right-1 px-1">
          {count > 99 ? "99+" : count}
        </Badge>
      ) : null}
    </Button>
  );
}
