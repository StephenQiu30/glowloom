"use client";

import Link from "next/link";
import { useRef } from "react";
import { useAnnouncementExpiry } from "./use-announcement-countdown";
import { X } from "lucide-react";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { safeNotificationAction, type DisplayAnnouncement } from "./contracts";

export function GlobalAnnouncement({
  announcement,
  onDismiss,
  onView,
  onExpire,
}: {
  announcement?: DisplayAnnouncement;
  onDismiss: () => void;
  onView: () => void;
  onExpire: () => void;
}) {
  useAnnouncementExpiry(
    announcement?.endless ? "" : (announcement?.ends_at ?? ""),
    onExpire,
  );
  if (!announcement) return null;
  return (
    <Alert role="status" className="global-announcement" aria-label="全局公告">
      <AlertTitle className="min-w-0 truncate">{announcement.title}</AlertTitle>
      <div className="flex shrink-0 items-center gap-1">
        <Button variant="ghost" size="sm" onClick={onView}>
          查看
        </Button>
        <Button
          variant="ghost"
          size="icon"
          aria-label="关闭公告"
          onClick={onDismiss}
        >
          <X />
        </Button>
      </div>
    </Alert>
  );
}

export function AnnouncementDetail({
  announcement,
  onClose,
}: {
  announcement: DisplayAnnouncement;
  onClose: () => void;
}) {
  const returnFocus = useRef<HTMLElement | null>(null);
  const action = safeNotificationAction(announcement.action_path);
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        className="notification-surface sm:max-w-[560px] [&_[data-slot=dialog-close]]:size-11"
        onOpenAutoFocus={() => {
          returnFocus.current =
            document.activeElement instanceof HTMLElement
              ? document.activeElement
              : null;
        }}
        onCloseAutoFocus={(event) => {
          if (returnFocus.current?.isConnected) {
            event.preventDefault();
            returnFocus.current.focus();
          }
        }}
      >
        <DialogHeader>
          <DialogTitle size="lg" className="pr-10">
            {announcement.title}
          </DialogTitle>
          <DialogDescription>系统公告</DialogDescription>
        </DialogHeader>
        <p className="break-words whitespace-pre-wrap">{announcement.body}</p>
        <DialogFooter>
          {action ? (
            <Button asChild>
              <Link href={action} onClick={onClose}>
                查看
              </Link>
            </Button>
          ) : (
            <Button onClick={onClose}>我知道了</Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
