"use client";

import { useEffect, useRef, useState } from "react";
import { RequestError } from "@/lib/request";
import { readErrorMessage } from "@/lib/read-error";

type Command = (options: { signal: AbortSignal }) => Promise<void>;

export function useScriptWrite() {
  const controller = useRef<AbortController | null>(null);
  const command = useRef<Command | null>(null);
  const [pending, setPending] = useState(false);
  const [unknown, setUnknown] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => () => controller.current?.abort(), []);

  async function submit(next?: Command) {
    if (controller.current) return;
    command.current ??= next ?? null;
    if (!command.current) return;
    const flight = new AbortController();
    controller.current = flight;
    setPending(true);
    setError("");
    try {
      await command.current({ signal: flight.signal });
      if (flight.signal.aborted) return;
      command.current = null;
      setUnknown(false);
    } catch (reason) {
      if (flight.signal.aborted) return;
      const rejected =
        reason instanceof RequestError &&
        [400, 401, 403, 404, 409, 413, 415, 422].includes(reason.status) &&
        reason.code !== "invalid_receipt";
      if (rejected) command.current = null;
      setUnknown(!rejected);
      setError(
        rejected
          ? readErrorMessage(reason)
          : "结果尚未确认，请核对并重试原操作。",
      );
    } finally {
      controller.current = null;
      if (!flight.signal.aborted) setPending(false);
    }
  }

  return { submit, pending, unknown, error, locked: pending || unknown };
}
