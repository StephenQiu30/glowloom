"use client";

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { ApiError } from "@/components/canvas/runtime/lib/request";
import {
  executeWorkspaceCommand,
  type WorkspaceCommand,
} from "../lib/workspace-commands";

export function useWorkspaceCommand(isCurrentActor: () => boolean) {
  const { account, ready, signingOut } = useAccount();
  const identity =
    ready && account && !signingOut ? `USER:${account.id}` : null;
  const liveIdentity = useRef(identity);
  useLayoutEffect(() => {
    liveIdentity.current = identity;
  }, [identity]);
  const pending = useRef<{
    command: WorkspaceCommand;
    identity: string;
  } | null>(null);
  const lock = useRef(false);
  const [busy, setBusy] = useState(false);
  const [unknown, setUnknown] = useState(false);
  const [error, setError] = useState<ApiError | null>(null);
  useEffect(() => {
    if (!busy && !unknown) return;
    const protect = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", protect);
    return () => window.removeEventListener("beforeunload", protect);
  }, [busy, unknown]);
  const run = async (command?: WorkspaceCommand, queryOnly = false) => {
    if (
      lock.current ||
      !identity ||
      !isCurrentActor() ||
      (pending.current && pending.current.identity !== identity)
    )
      return false;
    if (!pending.current && command) pending.current = { command, identity };
    const original = pending.current;
    if (!original) return false;
    lock.current = true;
    setBusy(true);
    setError(null);
    try {
      await executeWorkspaceCommand(
        original.command,
        queryOnly,
        unknown,
        () => liveIdentity.current === original.identity && isCurrentActor(),
      );
      if (liveIdentity.current !== original.identity || !isCurrentActor()) {
        setUnknown(true);
        return false;
      }
      pending.current = null;
      setUnknown(false);
      return true;
    } catch (failure) {
      const value =
        failure instanceof ApiError
          ? failure
          : new ApiError("暂未确认结果，请核对原操作", 50300);
      const uncertain = value.code >= 50000;
      if (!uncertain) pending.current = null;
      setUnknown(uncertain);
      setError(value);
      if (liveIdentity.current === original.identity && isCurrentActor())
        notifyError(value, uncertain ? "操作结果待确认" : "操作未完成");
      return false;
    } finally {
      lock.current = false;
      setBusy(false);
    }
  };
  return {
    busy,
    unknown,
    error,
    run,
    clearError: () => {
      if (!pending.current) setError(null);
    },
  };
}
