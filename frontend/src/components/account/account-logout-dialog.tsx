"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { revokeSession, useOptionalSession } from "@/components/auth/session";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";

export function AccountLogoutDialog({
  open,
  onOpenChange,
  disabled = false,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  disabled?: boolean;
}) {
  const session = useOptionalSession();
  const router = useRouter();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  async function leave() {
    if (disabled || pending || !session) return;
    setPending(true);
    setError("");
    try {
      await revokeSession(session);
      router.replace("/login");
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "退出结果未确认，请重试。",
      );
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        if (!pending) onOpenChange(value);
      }}
    >
      <DialogContent>
        <DialogTitle>退出登录？</DialogTitle>
        <DialogDescription>只退出当前设备。</DialogDescription>
        {error && <FeedbackToast message={error} />}
        <DialogFooter>
          <Button
            variant="outline"
            disabled={pending}
            onClick={() => onOpenChange(false)}
          >
            取消
          </Button>
          <Button disabled={disabled || pending} onClick={() => void leave()}>
            {pending ? "正在退出…" : "退出登录"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
