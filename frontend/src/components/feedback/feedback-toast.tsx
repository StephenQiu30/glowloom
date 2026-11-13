"use client";

import { useEffect, useId, useRef } from "react";
import type { Action, ToastT } from "sonner";
import { toast } from "./notification";

export function FeedbackToast({
  message,
  tone = "error",
  action,
  cancel,
  attempt = 0,
  onDismiss,
}: {
  message: string;
  tone?: "error" | "info" | "success" | "warning" | "loading";
  action?: Action;
  cancel?: Action;
  attempt?: number;
  onDismiss?: (toast: ToastT) => void;
}) {
  const ownerID = useId();
  const id = `${ownerID}:${attempt}`;
  const latestAction = useRef(action);
  const latestCancel = useRef(cancel);
  const latestDismiss = useRef(onDismiss);
  const label = action?.label;
  const cancelLabel = cancel?.label;
  useEffect(() => {
    latestAction.current = action;
    latestCancel.current = cancel;
    latestDismiss.current = onDismiss;
  }, [action, cancel, onDismiss]);
  useEffect(() => {
    if (!message) return;
    toast[tone](message, {
      id,
      onDismiss: (toast) => latestDismiss.current?.(toast),
      onAutoClose: (toast) => latestDismiss.current?.(toast),
      cancel: cancelLabel
        ? {
            label: cancelLabel,
            onClick: (event) => latestCancel.current?.onClick(event),
          }
        : undefined,
      action: label
        ? {
            label,
            onClick: (event) => latestAction.current?.onClick(event),
          }
        : undefined,
    });
    return () => {
      toast.dismiss(id);
    };
  }, [id, message, tone, label, cancelLabel, attempt]);
  return null;
}
