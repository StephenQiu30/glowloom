"use client";

import { useEffect, useId, useRef } from "react";
import type { Action, ToastT } from "sonner";
import { toast } from "./notification";

export function FeedbackToast({
  message,
  tone = "error",
  action,
  attempt = 0,
  onDismiss,
}: {
  message: string;
  tone?: "error" | "info" | "success" | "warning" | "loading";
  action?: Action;
  attempt?: number;
  onDismiss?: (toast: ToastT) => void;
}) {
  const ownerID = useId();
  const id = `${ownerID}:${attempt}`;
  const latestAction = useRef(action);
  const latestDismiss = useRef(onDismiss);
  const label = action?.label;
  useEffect(() => {
    latestAction.current = action;
    latestDismiss.current = onDismiss;
  }, [action, onDismiss]);
  useEffect(() => {
    if (!message) return;
    toast[tone](message, {
      id,
      onDismiss: (toast) => latestDismiss.current?.(toast),
      onAutoClose: (toast) => latestDismiss.current?.(toast),
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
  }, [id, message, tone, label, attempt]);
  return null;
}
