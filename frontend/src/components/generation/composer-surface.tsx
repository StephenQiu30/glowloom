"use client";

import { useCallback, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";

/** Move one portal host, so expanding never remounts the editor or upload owner. */
export function ComposerSurface({
  expanded,
  onExpandedChange,
  title,
  children,
}: {
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
  title: string;
  children: ReactNode;
}) {
  const inline = useRef<HTMLDivElement>(null);
  const modal = useRef<HTMLDivElement | null>(null);
  const [host, setHost] = useState<HTMLDivElement | null>(null);
  const mountInline = useCallback((container: HTMLDivElement | null) => {
    inline.current = container;
    if (!container) return;
    const element = document.createElement("div");
    element.className = "flex min-h-0 w-full flex-1 flex-col";
    (modal.current ?? inline.current)?.appendChild(element);
    setHost(element);
    return () => element.remove();
  }, []);
  const mountModal = useCallback(
    (element: HTMLDivElement | null) => {
      modal.current = element;
      if (host) (element ?? inline.current)?.appendChild(host);
    },
    [host],
  );

  return (
    <Dialog open={expanded} onOpenChange={onExpandedChange}>
      <div ref={mountInline} className="min-w-0" />
      <DialogContent
        showCloseButton={false}
        aria-describedby={undefined}
        className="flex h-[min(560px,calc(100dvh-32px))] flex-col overflow-hidden p-0 sm:max-w-5xl"
        data-canvas-no-zoom
        data-canvas-shortcuts-ignore
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          host
            ?.querySelector<HTMLElement>('[aria-label="收起编辑器"]')
            ?.focus();
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          host
            ?.querySelector<HTMLElement>('[aria-label="展开编辑器"]')
            ?.focus({ preventScroll: true });
        }}
        onEscapeKeyDown={(event) => {
          if (event.isComposing || event.keyCode === 229)
            event.preventDefault();
        }}
      >
        <DialogTitle className="sr-only">{title}</DialogTitle>
        <div ref={mountModal} className="flex min-h-0 flex-1 flex-col" />
      </DialogContent>
      {host && createPortal(children, host)}
    </Dialog>
  );
}
