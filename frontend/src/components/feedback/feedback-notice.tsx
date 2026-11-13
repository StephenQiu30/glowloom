"use client";

import { useState } from "react";
import type { Action } from "sonner";
import { Button } from "@/components/ui/button";
import { FeedbackToast } from "./feedback-toast";
import { FeedbackAlert } from "./feedback-alert";

export function FeedbackNotice({
  message,
  description,
  tone = "info",
  action,
}: {
  message: string;
  description?: string;
  tone?: "info" | "error";
  action: Action;
}) {
  const [dismissed, setDismissed] = useState(false);
  return dismissed ? (
    <FeedbackAlert
      title={message}
      description={description}
      tone={tone}
      actions={
        <Button variant="outline" size="feedback" onClick={action.onClick}>
          {action.label}
        </Button>
      }
    />
  ) : (
    <FeedbackToast
      message={message}
      tone={tone}
      action={{
        ...action,
        onClick: (event) => {
          setDismissed(true);
          action.onClick(event);
        },
      }}
      onDismiss={() => setDismissed(true)}
    />
  );
}
