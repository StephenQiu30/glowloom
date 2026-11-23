"use client";

import { useEffect, useRef, useState } from "react";
import { History, X } from "lucide-react";
import {
  ConversationWorkspace,
  type ConversationWorkspaceHandle,
} from "@/components/conversations/conversation-workspace";
import { FeedbackAlert } from "@/components/feedback/feedback-alert";
import { mediaBinding } from "@/components/generation/reference-controls";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { readErrorMessage } from "@/lib/read-error";
import { CanvasStoryboard } from "./canvas-storyboard";
import { CanvasStoryboardMaterials } from "./canvas-storyboard-materials";
import {
  findCanvasConversation,
  type CanvasConversationScope,
} from "./canvas-conversation";

export type CanvasStoryboardTab = "shots" | "materials" | "conversation";

export function CanvasStoryboardWorkspace({
  actorID,
  scopeID,
  projectID,
  canvasID,
  commitSeq,
  writable,
  boardWritable,
  open,
  tab,
  onTabChange,
  onClose,
  onOpenLegacy,
}: CanvasConversationScope & {
  commitSeq: string;
  writable: boolean;
  boardWritable: boolean;
  open: boolean;
  tab: CanvasStoryboardTab;
  onTabChange: (tab: CanvasStoryboardTab) => void;
  onClose: () => void;
  onOpenLegacy: () => void;
}) {
  const conversation = useRef<ConversationWorkspaceHandle>(null);
  const tabs = useRef<HTMLDivElement>(null);
  const [session, setSession] = useState<string | null>();
  const [sessionError, setSessionError] = useState("");
  const [lookup, setLookup] = useState(0);
  const [inputError, setInputError] = useState("");
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
  }, [actorID, scopeID, projectID, canvasID, lookup]);
  useEffect(() => {
    if (open)
      tabs.current
        ?.querySelector<HTMLElement>('[aria-selected="true"]')
        ?.focus();
  }, [open]);

  function appendInput(
    append: (workspace: ConversationWorkspaceHandle) => boolean,
  ) {
    setInputError("");
    onTabChange("conversation");
    try {
      if (!conversation.current || !append(conversation.current))
        throw new Error("暂时无法加入对话，请重试。");
      requestAnimationFrame(() => conversation.current?.focusPrompt());
    } catch (error) {
      setInputError(readErrorMessage(error));
    }
  }

  const canReference = writable && session !== undefined;
  return (
    <aside
      hidden={!open}
      aria-label="画布创作工作台"
      className="absolute inset-y-0 right-0 z-40 w-full border-l bg-popover shadow-lg sm:max-w-[36rem]"
      data-canvas-no-zoom
      onKeyDown={(event) => {
        if (
          event.key !== "Escape" ||
          event.defaultPrevented ||
          !event.currentTarget.contains(event.target as Node)
        )
          return;
        event.stopPropagation();
        onClose();
      }}
    >
      <Tabs
        value={tab}
        onValueChange={(next) => onTabChange(next as CanvasStoryboardTab)}
        className="h-full min-h-0 min-w-0 gap-0"
      >
        <header className="flex shrink-0 items-center justify-between gap-1 border-b px-2 py-2 sm:px-4">
          <TabsList ref={tabs} aria-label="画布工作台" className="h-auto">
            <TabsTrigger value="shots" className="min-h-9 px-2">
              故事板
            </TabsTrigger>
            <TabsTrigger value="materials" className="min-h-9 px-2">
              项目素材
            </TabsTrigger>
            <TabsTrigger value="conversation" className="min-h-9 px-2">
              AI 对话
            </TabsTrigger>
          </TabsList>
          <div className="flex shrink-0 items-center">
            <Button
              variant="ghost"
              size="icon"
              aria-label="旧助手历史"
              title="旧助手历史"
              onClick={onOpenLegacy}
            >
              <History />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label="关闭画布创作工作台"
              onClick={onClose}
            >
              <X />
            </Button>
          </div>
        </header>
        <TabsContent
          value="shots"
          forceMount
          className="min-h-0 overflow-y-auto overscroll-contain data-[state=inactive]:hidden"
        >
          <CanvasStoryboard
            active={open}
            canvasID={canvasID}
            projectID={projectID}
            commitSeq={commitSeq}
            writable={boardWritable}
            canReference={canReference}
            onDiscuss={() => {
              onTabChange("conversation");
              requestAnimationFrame(() => conversation.current?.focusPrompt());
            }}
            onReference={(reference) =>
              appendInput((current) => current.appendText(reference))
            }
          />
        </TabsContent>
        <TabsContent
          value="materials"
          forceMount
          className="min-h-0 overflow-y-auto overscroll-contain data-[state=inactive]:hidden"
        >
          <CanvasStoryboardMaterials
            active={open && tab === "materials"}
            actorID={actorID}
            projectID={projectID}
            canReference={canReference}
            onReference={(asset) =>
              appendInput((current) =>
                current.attachReference(
                  mediaBinding(asset, `reference_${asset.kind}`),
                ),
              )
            }
          />
        </TabsContent>
        <TabsContent
          value="conversation"
          forceMount
          className="flex min-h-0 flex-col gap-2 px-2 pt-3 pb-2 data-[state=inactive]:hidden sm:px-4"
        >
          {inputError ? (
            <FeedbackAlert tone="error" title={inputError} />
          ) : null}
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
                      setLookup((previous) => previous + 1);
                    }}
                  >
                    重新读取
                  </Button>
                }
              />
            ) : (
              <p role="status" className="p-3 text-muted-foreground">
                正在查找画布对话…
              </p>
            )
          ) : (
            <ConversationWorkspace
              ref={conversation}
              ownerKey={`${actorID}/${scopeID}`}
              projectID={projectID}
              canvasID={canvasID}
              sessionID={session ?? undefined}
              enabled
              writable={writable}
              panel
              onChooseProject={() => {}}
            />
          )}
        </TabsContent>
      </Tabs>
    </aside>
  );
}
