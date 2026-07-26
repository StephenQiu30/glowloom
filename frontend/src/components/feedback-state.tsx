"use client";

import { CircleAlertIcon, InboxIcon, LoaderCircleIcon } from "lucide-react";

import { Alert, AlertAction, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

interface FeedbackStateProps {
  state: "loading" | "empty" | "error";
  title: string;
  description: string;
  details?: string;
}

const icons = {
  loading: <LoaderCircleIcon aria-hidden className="animate-spin" />,
  empty: <InboxIcon aria-hidden />,
  error: <CircleAlertIcon aria-hidden />,
};

export function FeedbackState({ state, title, description, details }: FeedbackStateProps) {
  return (
    <Alert
      aria-live={state === "error" ? "assertive" : "polite"}
      role={state === "error" ? "alert" : "status"}
      variant={state === "error" ? "destructive" : "default"}
    >
      {icons[state]}
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>{description}</AlertDescription>
      {details ? (
        <AlertAction>
          <Dialog>
            <DialogTrigger asChild>
              <Button size="sm" variant="outline">
                查看错误详情
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>错误详情</DialogTitle>
                <DialogDescription>此信息可用于排查，服务端事实不会被改写。</DialogDescription>
              </DialogHeader>
              <code className="rounded-md bg-muted p-3 font-mono text-xs">{details}</code>
            </DialogContent>
          </Dialog>
        </AlertAction>
      ) : null}
    </Alert>
  );
}
