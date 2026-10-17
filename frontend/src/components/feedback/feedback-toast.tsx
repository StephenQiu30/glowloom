"use client";

import { useEffect, useId, useRef } from "react";
import { toast, type Action } from "sonner";

/** Present state-owned async feedback through the single root shadcn Toaster. */
export function FeedbackToast({
  message,
  tone = "error",
  action,
  attempt = 0,
}: {
  message: string;
  tone?: "error" | "info" | "success";
  action?: Action;
  attempt?: number;
}) {
  const ownerID = useId();
  const id = `${ownerID}:${attempt}`;
  const latestAction = useRef(action);
  const label = action?.label;
  useEffect(() => {
    latestAction.current = action;
  }, [action]);
  useEffect(() => {
    if (!message) return;
    toast[tone](message, {
      id,
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
