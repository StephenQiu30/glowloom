"use client";

import Link from "next/link";
import { useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { type DisplayAnnouncement, safeNotificationAction } from "./contracts";
import {
  useAnnouncementCountdown,
  useAnnouncementExpiry,
} from "./use-announcement-countdown";

export function TimedAnnouncementDialog({
  announcement,
  onClose,
}: {
  announcement: DisplayAnnouncement;
  onClose: () => void;
}) {
  const [hovered, setHovered] = useState(false);
  const [interacting, setInteracting] = useState(false);
  const openingFocus = useRef(true);
  const title = useRef<HTMLHeadingElement>(null);
  const seconds = useAnnouncementCountdown(
    announcement.duration_seconds,
    hovered || interacting,
    onClose,
  );
  const action = safeNotificationAction(announcement.action_path);
  useAnnouncementExpiry(announcement.ends_at, onClose);
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        className="notification-surface sm:max-w-[560px] [&_[data-slot=dialog-close]]:size-11"
        onPointerEnter={() => setHovered(true)}
        onPointerLeave={() => setHovered(false)}
        onKeyDownCapture={(event) => {
          if (event.key !== "Escape") setInteracting(true);
        }}
        onFocusCapture={() => {
          if (!openingFocus.current) setInteracting(true);
        }}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          title.current?.focus();
          openingFocus.current = false;
        }}
      >
        <DialogHeader>
          <DialogTitle ref={title} tabIndex={-1} size="lg" className="pr-10">
            {announcement.title}
          </DialogTitle>
          <DialogDescription className="sr-only">
            消息通知，可暂停自动关闭或稍后前往通知中心查看。
          </DialogDescription>
        </DialogHeader>
        <p className="max-h-64 overflow-y-auto break-words whitespace-pre-wrap">
          {announcement.body}
        </p>
        <DialogFooter>
          <Button
            variant="ghost"
            onClick={() => setInteracting((value) => !value)}
          >
            {interacting ? "继续自动关闭" : "暂停自动关闭"}
          </Button>
          <Button asChild>
            <Link
              href={
                announcement.notification_id
                  ? `/notifications?notification=${announcement.notification_id}`
                  : (action ?? "/notifications")
              }
              onClick={onClose}
            >
              查看详情
            </Link>
          </Button>
        </DialogFooter>
        <p className="text-xs text-muted-foreground" aria-live="off">
          {hovered || interacting ? "自动关闭已暂停" : `${seconds} 秒后关闭`}
        </p>
      </DialogContent>
    </Dialog>
  );
}
