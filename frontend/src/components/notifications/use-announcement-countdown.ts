"use client";

import { useEffect, useEffectEvent, useRef, useState } from "react";

/** One mounted announcement owns one clock; pausing preserves elapsed time. */
export function useAnnouncementCountdown(
  durationSeconds: number,
  paused: boolean,
  onElapsed: () => void,
) {
  const remaining = useRef(durationSeconds * 1000);
  const finished = useRef(false);
  const [seconds, setSeconds] = useState(durationSeconds);
  const elapsed = useEffectEvent(onElapsed);

  useEffect(() => {
    if (paused || finished.current) return;
    const started = Date.now();
    const before = remaining.current;
    const interval = setInterval(() => {
      setSeconds(
        Math.ceil(Math.max(0, before - (Date.now() - started)) / 1000),
      );
    }, 250);
    const timeout = setTimeout(() => {
      finished.current = true;
      clearInterval(interval);
      setSeconds(0);
      elapsed();
    }, before);
    return () => {
      clearTimeout(timeout);
      clearInterval(interval);
      remaining.current = Math.max(0, before - (Date.now() - started));
    };
  }, [paused]);

  return seconds;
}

/** Long validity windows must not overflow the browser's 32-bit timeout. */
export function useAnnouncementExpiry(endsAt: string, onExpired: () => void) {
  const expire = useEffectEvent(onExpired);
  useEffect(() => {
    const end = Date.parse(endsAt);
    if (!Number.isFinite(end)) return;
    let timer: ReturnType<typeof setTimeout>;
    function check() {
      const remaining = end - Date.now();
      if (remaining <= 0) {
        expire();
        return;
      }
      timer = setTimeout(check, Math.min(remaining, 86400000));
    }
    timer = setTimeout(
      check,
      Math.min(Math.max(0, end - Date.now()), 86400000),
    );
    return () => clearTimeout(timer);
  }, [endsAt]);
}
