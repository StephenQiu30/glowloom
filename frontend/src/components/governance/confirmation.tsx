"use client";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { FieldLabel } from "@/components/ui/field";
import { useEffect, useRef, useState } from "react";
import { reauthenticate } from "@/api/identity";
import { RequestError } from "@/lib/request";
import { authErrorMessage } from "@/components/auth/contracts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
export type ConfirmAction = {
  title: string;
  effect: string;
  reason?: boolean;
  password?: boolean;
  confirmLabel?: string;
  destructive?: boolean;
  run: (
    reason: string,
    signal: AbortSignal,
    requestID: string,
  ) => Promise<unknown>;
};
export function Confirmation({
  action,
  onClose,
  onDone,
}: {
  action: ConfirmAction | null;
  onClose: () => void;
  onDone: () => Promise<void> | void;
}) {
  const [password, setPassword] = useState("");
  const [reason, setReason] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [conflicted, setConflicted] = useState(false);
  const flight = useRef<AbortController | null>(null);
  const locked = useRef(false);
  const requestID = useRef(crypto.randomUUID());
  useEffect(
    () => () => {
      flight.current?.abort();
    },
    [],
  );
  async function submit() {
    if (!action || locked.current || conflicted) return;
    locked.current = true;
    setPending(true);
    setError("");
    const c = new AbortController();
    flight.current = c;
    try {
      if (action.password !== false)
        await reauthenticate({ password }, { signal: c.signal });
      setPassword("");
      await action.run(reason.trim(), c.signal, requestID.current);
      if (!c.signal.aborted) {
        await onDone();
        onClose();
      }
    } catch (e) {
      if (!c.signal.aborted) {
        setError(
          e instanceof RequestError &&
            e.status === 409 &&
            e.code === "revision_conflict"
            ? "资料已经变化，已重新读取。请关闭后核对最新状态再操作。"
            : authErrorMessage(e),
        );
        if (e instanceof RequestError && e.status === 409) {
          setConflicted(true);
          await onDone();
        }
      }
    } finally {
      locked.current = false;
      if (!c.signal.aborted) setPending(false);
    }
  }
  return (
    <Dialog
      open={Boolean(action)}
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
    >
      <DialogContent className="max-h-[85dvh] overflow-y-auto">
        <DialogTitle>{action?.title}</DialogTitle>
        <DialogDescription>{action?.effect}</DialogDescription>
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          {action?.reason ? (
            <FieldLabel className="grid w-full gap-2">
              操作原因
              <Textarea
                required
                minLength={5}
                maxLength={500}
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                disabled={pending}
              />
            </FieldLabel>
          ) : null}
          {action?.password !== false ? (
            <FieldLabel className="grid w-full gap-2">
              当前密码
              <Input
                type="password"
                required
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                disabled={pending}
              />
            </FieldLabel>
          ) : null}
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          <div className="flex flex-wrap justify-end gap-2">
            <Button
              type="button"
              variant="secondary"
              disabled={pending}
              onClick={onClose}
            >
              取消
            </Button>
            <Button
              variant={action?.destructive ? "destructive" : "default"}
              disabled={
                conflicted ||
                pending ||
                (action?.reason && reason.trim().length < 5) ||
                (!password && action?.password !== false)
              }
            >
              {pending ? "正在处理…" : (action?.confirmLabel ?? "确认操作")}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
