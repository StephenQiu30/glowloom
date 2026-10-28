"use client";

import { useLayoutEffect, useRef } from "react";

/** A mode change removes its trigger: move focus to the safe exit, then back. */
export function useBatchToolbarFocus(batch: boolean) {
  const toolbar = useRef<HTMLDivElement>(null);
  const previous = useRef(batch);
  useLayoutEffect(() => {
    if (previous.current !== batch) {
      toolbar.current
        ?.querySelector<HTMLButtonElement>("[data-batch-focus]")
        ?.focus();
      previous.current = batch;
    }
  }, [batch]);
  return toolbar;
}
