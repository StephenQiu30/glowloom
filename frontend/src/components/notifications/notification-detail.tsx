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
import { PageLoading } from "@/components/layout/loading-state";
import { FeedbackAlert } from "@/components/feedback/feedback-alert";
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
  const retryDetail = (
    <Button
      variant="outline"
      size="feedback"
      onClick={() => void detail.mutate().catch(() => {})}
    >
      重试
    </Button>
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
        {...(!item && { "aria-describedby": undefined })}
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
          {item ? (
            <DialogDescription>
              {categoryLabel(item.category)} ·{" "}
              {notificationDate(item.occurred_at)}
            </DialogDescription>
          ) : null}
        </DialogHeader>
        {!item && detail.isValidating ? (
          <PageLoading layout="detail" message="正在读取通知" />
        ) : !item && detail.error ? (
          <ContentState
            className="min-h-32"
            state="error"
            title="通知暂时无法查看"
            actions={retryDetail}
          />
        ) : !item ? (
          <PageLoading layout="detail" message="正在读取通知" />
        ) : (
          <>
            {detail.error && !detail.isValidating ? (
              <FeedbackAlert
                tone="error"
                title="通知刷新失败"
                actions={retryDetail}
              />
            ) : null}
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
