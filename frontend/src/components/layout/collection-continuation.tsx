"use client";

import { useEffect, useEffectEvent, useRef } from "react";
import { inView } from "motion";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { ContentState } from "./content-state";

export function CollectionContinuation({
  cursor,
  loading,
  disabled,
  error,
  onContinue,
  onRetry,
  onRestart,
}: {
  cursor?: string | null;
  loading: boolean;
  disabled: boolean;
  error?: string;
  onContinue: () => void;
  onRetry: () => void;
  onRestart?: () => void;
}) {
  const tail = useRef<HTMLDivElement>(null);
  const continueRead = useEffectEvent(onContinue);
  useEffect(() => {
    if (
      !cursor ||
      loading ||
      disabled ||
      error ||
      !tail.current ||
      typeof IntersectionObserver === "undefined"
    )
      return;
    let active = true;
    const stop = inView(tail.current, () => {
      if (!active) return;
      active = false;
      continueRead();
    });
    return () => {
      active = false;
      stop();
    };
  }, [cursor, loading, disabled, error]);

  if (!cursor) return null;
  return (
    <div ref={tail} className="min-h-1" data-slot="collection-continuation">
      {loading && (
        <p
          role="status"
          className="flex items-center justify-center gap-2 text-muted-foreground"
        >
          <Spinner aria-hidden="true" />
          正在加载更多记录
        </p>
      )}
      {error && (
        <ContentState
          state="error"
          title="暂时无法加载更多记录"
          description={error}
          actions={
            <Button
              variant="outline"
              disabled={disabled}
              onClick={onRestart ?? onRetry}
            >
              {onRestart ? "重新读取" : "重试加载更多"}
            </Button>
          }
        />
      )}
    </div>
  );
}
