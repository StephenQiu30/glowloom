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
  const returnFocus = useRef<HTMLElement | null>(null);
  const title = useRef<HTMLHeadingElement>(null);
  useAnnouncementCountdown(
    announcement.duration_seconds,
    hovered || interacting,
    onClose,
    !announcement.manual_close,
  );
  const action = safeNotificationAction(announcement.action_path);
  useAnnouncementExpiry(
    announcement.endless ? "" : announcement.ends_at,
    onClose,
  );
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
          returnFocus.current =
            document.activeElement instanceof HTMLElement
              ? document.activeElement
              : null;
          event.preventDefault();
          title.current?.focus();
          openingFocus.current = false;
        }}
        onCloseAutoFocus={(event) => {
          if (returnFocus.current?.isConnected) {
            event.preventDefault();
            returnFocus.current.focus();
          }
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
          {!announcement.manual_close ? (
            <Button
              variant="ghost"
              onClick={() => setInteracting((value) => !value)}
            >
              {interacting ? "继续自动关闭" : "暂停自动关闭"}
            </Button>
          ) : null}
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
      </DialogContent>
    </Dialog>
  );
}
