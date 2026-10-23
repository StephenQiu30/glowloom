"use client";

import { useEffect, useRef } from "react";
import { toast } from "sonner";
import { ApiError } from "@/components/canvas/runtime/lib/request";
import { SyncPendingError } from "@/components/canvas/runtime/lib/read-recovery";

const notifications = new WeakMap<Error, string | number>();

export function notifyError(error: unknown, title = "操作未完成") {
  if (error instanceof Error && error.name === "AbortError") return;
  if (error instanceof SyncPendingError) return;
  if (error instanceof Error && notifications.has(error))
    return notifications.get(error);
  const reason =
    error instanceof Error
      ? error.message
      : typeof error === "string"
        ? error
        : "请稍后重试。";
  const requestId = error instanceof ApiError ? error.requestId : undefined;
  const id = toast.error(title, {
    description: requestId ? `${reason}（请求编号：${requestId}）` : reason,
  });
  if (error instanceof Error) notifications.set(error, id);
  return id;
}

/** Error boundaries synchronize a caught error with the shared notification surface. */
export function useErrorNotification(error: Error | undefined, title: string) {
  const notified = useRef<{ error: Error; id?: string | number }>(undefined);
  useEffect(() => {
    if (notified.current?.error === error) return;
    if (notified.current?.id !== undefined) toast.dismiss(notified.current.id);
    notified.current = error
      ? { error, id: notifyError(error, title) }
      : undefined;
  }, [error, title]);
}
