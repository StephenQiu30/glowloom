"use client";

import { useEffect, useRef, useState } from "react";
import { History, X } from "lucide-react";
import { ConversationWorkspace } from "@/components/conversations/conversation-workspace";
import { FeedbackAlert } from "@/components/feedback/feedback-alert";
import { Button } from "@/components/ui/button";
import { readErrorMessage } from "@/lib/read-error";
import {
  findCanvasConversation,
  type CanvasConversationScope,
} from "./canvas-conversation";

const minPanelWidth = 320;
const maxPanelWidth = 460;

function clampPanelWidth(width: number) {
  return Math.round(Math.max(minPanelWidth, Math.min(maxPanelWidth, width)));
}

export function CanvasConversationPanel({
  actorID,
  scopeID,
  projectID,
  canvasID,
  writable,
  open,
  width,
  onWidthChange,
  onClose,
  onOpenLegacy,
}: CanvasConversationScope & {
  writable: boolean;
  open: boolean;
  width: number;
  onWidthChange: (width: number) => void;
  onClose: () => void;
  onOpenLegacy: () => void;
}) {
  const heading = useRef<HTMLHeadingElement>(null);
  const [session, setSession] = useState<string | null>();
  const [sessionError, setSessionError] = useState("");
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    void findCanvasConversation(
      { actorID, scopeID, projectID, canvasID },
      controller.signal,
    )
      .then((id) => {
        if (!controller.signal.aborted) setSession(id);
      })
      .catch((error) => {
        if (!controller.signal.aborted)
          setSessionError(readErrorMessage(error));
      });
    return () => controller.abort();
  }, [actorID, scopeID, projectID, canvasID, attempt]);

  useEffect(() => {
    if (open) heading.current?.focus();
  }, [open]);

  return (
    <section
      id="canvas-conversation-panel"
      hidden={!open}
      aria-labelledby="canvas-conversation-heading"
      className="canvas-conversation-panel absolute inset-0 z-[60] bg-card text-card-foreground"
      data-canvas-no-zoom
      onKeyDown={(event) => {
        if (
          event.key !== "Escape" ||
          event.defaultPrevented ||
          event.nativeEvent.isComposing ||
          !event.currentTarget.contains(event.target as Node)
        )
          return;
        event.stopPropagation();
        onClose();
      }}
    >
      <div
        role="separator"
        tabIndex={0}
        aria-label="调整对话宽度"
        aria-orientation="vertical"
        aria-controls="canvas-conversation-panel"
        aria-valuemin={minPanelWidth}
        aria-valuemax={maxPanelWidth}
        aria-valuenow={width}
        aria-valuetext={`${width} 像素`}
        className="absolute inset-y-0 -left-1 z-10 hidden w-2 cursor-col-resize touch-none focus-visible:bg-ring focus-visible:outline-none md:block"
        onKeyDown={(event) => {
          const delta =
            event.key === "ArrowLeft"
              ? 20
              : event.key === "ArrowRight"
                ? -20
                : 0;
          if (!delta) return;
          event.preventDefault();
          onWidthChange(clampPanelWidth(width + delta));
        }}
        onPointerDown={(event) => {
          if (event.button === 0)
            event.currentTarget.setPointerCapture(event.pointerId);
        }}
        onPointerMove={(event) => {
          const panel = event.currentTarget.parentElement;
          if (panel && event.currentTarget.hasPointerCapture(event.pointerId))
            onWidthChange(
              clampPanelWidth(
                panel.getBoundingClientRect().right - event.clientX,
              ),
            );
        }}
      />
      <div className="flex h-full min-h-0 flex-col">
        <header className="flex h-14 shrink-0 items-center justify-between gap-2 px-4">
          <h2
            id="canvas-conversation-heading"
            ref={heading}
            tabIndex={-1}
            className="min-w-0 truncate text-sm font-medium outline-none"
          >
            AI 对话
          </h2>
          <div className="flex gap-2">
            <Button
              variant="ghost"
              size="icon-sm"
              className="max-lg:size-11"
              aria-label="旧助手历史"
              title="旧助手历史"
              onClick={onOpenLegacy}
            >
              <History />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              className="max-lg:size-11"
              aria-label="收起 AI 对话"
              onClick={onClose}
            >
              <X />
            </Button>
          </div>
        </header>
        <div className="flex min-h-0 flex-1 flex-col px-2 pb-[max(0.5rem,env(safe-area-inset-bottom))]">
          {session === undefined ? (
            sessionError ? (
              <FeedbackAlert
                tone="error"
                title={sessionError}
                actions={
                  <Button
                    variant="outline"
                    size="feedback"
                    onClick={() => {
                      setSessionError("");
                      setAttempt((value) => value + 1);
                    }}
                  >
                    重新读取
                  </Button>
                }
              />
            ) : (
              <p role="status" className="px-2 text-sm text-muted-foreground">
                正在查找画布对话…
              </p>
            )
          ) : (
            <ConversationWorkspace
              ownerKey={`${actorID}/${scopeID}`}
              projectID={projectID}
              canvasID={canvasID}
              sessionID={session ?? undefined}
              enabled
              writable={writable}
              panel
              compactEntry
              panelEmptyMessage={null}
              onChooseProject={() => {}}
            />
          )}
        </div>
      </div>
    </section>
  );
}
