"use client";

import Link from "next/link";
import {
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type RefObject,
} from "react";
import useSWR from "swr";
import { getNotification, readNotification } from "@/api/notifications";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ContentState } from "@/components/layout/content-state";
import {
  requireInboxNotification,
  safeNotificationAction,
  categoryLabel,
  notificationDate,
} from "./contracts";
import { useNotificationCenter } from "./notification-provider";

export function NotificationDetail({
  id,
  onClose,
  returnFocus,
  fallbackFocus,
}: {
  id: string;
  onClose: () => void;
  returnFocus: RefObject<HTMLElement | null>;
  fallbackFocus: RefObject<HTMLElement | null>;
}) {
  const center = useNotificationCenter();
  const detail = useSWR(
    ["notification-detail", center?.identity, id],
    async () => requireInboxNotification(await getNotification({ id })),
    { shouldRetryOnError: false },
  );
  const command = useRef(crypto.randomUUID());
  const attempted = useRef(false);
  const reading = useRef(false);
  const [readFailed, setReadFailed] = useState(false);
  const [pending, setPending] = useState(false);
  async function markRead() {
    if (reading.current) return;
    reading.current = true;
    setPending(true);
    try {
      const next = await readNotification(
        { id },
        { command_id: command.current },
      );
      center?.synchronize(next);
      await detail.mutate(
        (current) => (current ? { ...current, read: true } : current),
        false,
      );
      setReadFailed(false);
    } catch {
      setReadFailed(true);
    } finally {
      reading.current = false;
      setPending(false);
    }
  }
  const automaticRead = useEffectEvent(markRead);
  useEffect(() => {
    if (detail.data && !detail.data.read && !attempted.current) {
      attempted.current = true;
      void automaticRead();
    }
  }, [detail.data]);
  const item = detail.data;
  const action = safeNotificationAction(item?.action_path);
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        className="notification-surface sm:max-w-[560px] [&_[data-slot=dialog-close]]:size-11"
        onCloseAutoFocus={(event) => {
          const replacement = Array.from(
            fallbackFocus.current?.querySelectorAll<HTMLElement>(
              "[data-notification-id]",
            ) ?? [],
          ).find((node) => node.dataset.notificationId === id);
          const target = returnFocus.current?.isConnected
            ? returnFocus.current
            : (replacement ?? fallbackFocus.current);
          if (target) {
            event.preventDefault();
            target.focus();
          }
        }}
      >
        <DialogHeader>
          <DialogTitle size="lg" className="pr-10">
            {item?.title ?? "通知详情"}
          </DialogTitle>
          <DialogDescription>
            {item
              ? `${categoryLabel(item.category)} · ${notificationDate(item.occurred_at)}`
              : "正在读取通知内容"}
          </DialogDescription>
        </DialogHeader>
        {detail.error ? (
          <ContentState
            className="min-h-32"
            state="error"
            title="通知暂时无法查看"
            actions={
              <Button variant="outline" onClick={() => void detail.mutate()}>
                重试
              </Button>
            }
          />
        ) : !item ? (
          <ContentState
            className="min-h-32"
            state="loading"
            title="正在读取通知"
          />
        ) : (
          <>
            {item.status === "disabled" || item.status === "expired" ? (
              <p className="text-sm text-muted-foreground">
                {item.status === "expired" ? "此通知已结束" : "此通知已下线"}
              </p>
            ) : null}
            <p className="break-words whitespace-pre-wrap">{item.body}</p>
            <DialogFooter>
              {readFailed ? (
                <Button
                  variant="outline"
                  disabled={pending}
                  onClick={() => void markRead()}
                >
                  重试标记已读
                </Button>
              ) : null}
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
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
