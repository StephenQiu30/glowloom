"use client";

import { useEffect, useRef, useState, type CSSProperties } from "react";
import { ArrowLeft, History, PanelRight, UserRound, X } from "lucide-react";
import Link from "next/link";
import { cn } from "cn";
import {
  ConversationWorkspace,
  type ConversationWorkspaceHandle,
} from "@/components/conversations/conversation-workspace";
import { FeedbackAlert } from "@/components/feedback/feedback-alert";
import { mediaBinding } from "@/components/generation/reference-controls";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { readErrorMessage } from "@/lib/read-error";
import {
  storyboardActionProposal,
  storyboardDirections,
  storyboardReferenceLabel,
  type StoryboardActionProposal,
} from "./canvas-storyboard-state";
import { CanvasStoryboard } from "./canvas-storyboard";
import { CanvasStoryboardMaterials } from "./canvas-storyboard-materials";
import {
  findCanvasConversation,
  type CanvasConversationScope,
} from "./canvas-conversation";

const minAssistantWidth = 320;
const maxAssistantWidth = 460;

function clampAssistantWidth(width: number) {
  return Math.round(
    Math.max(minAssistantWidth, Math.min(maxAssistantWidth, width)),
  );
}

export type CanvasStoryboardTab = "shots" | "materials" | "conversation";

export function CanvasStoryboardWorkspace({
  actorID,
  scopeID,
  projectID,
  canvasID,
  commitSeq,
  writable,
  boardWritable,
  title,
  saveStatus,
  canNavigate,
  open,
  tab,
  assistantOnly = false,
  onTabChange,
  onClose,
  onOpenLegacy,
}: CanvasConversationScope & {
  commitSeq: string;
  writable: boolean;
  boardWritable: boolean;
  title: string;
  saveStatus: string;
  canNavigate: boolean;
  open: boolean;
  tab: CanvasStoryboardTab;
  assistantOnly?: boolean;
  onTabChange: (tab: CanvasStoryboardTab) => void;
  onClose: () => void;
  onOpenLegacy: () => void;
}) {
  const conversation = useRef<ConversationWorkspaceHandle>(null);
  const [assistantWidth, setAssistantWidth] = useState(400);
  const heading = useRef<HTMLHeadingElement>(null);
  const assistantHeading = useRef<HTMLHeadingElement>(null);
  const [proposal, setProposal] = useState<StoryboardActionProposal>();
  const [lastBoardTab, setLastBoardTab] = useState<"shots" | "materials">(
    "shots",
  );
  const [assistantHidden, setAssistantHidden] = useState(false);
  const boardTab = tab === "conversation" ? lastBoardTab : tab;
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
      (assistantOnly || !heading.current?.getClientRects().length
        ? assistantHeading.current
        : heading.current
      )?.focus();
  }, [assistantOnly, open]);

  function appendInput(
    append: (workspace: ConversationWorkspaceHandle) => boolean,
  ) {
    setInputError("");
    showConversation();
    try {
      if (!conversation.current || !append(conversation.current))
        throw new Error("暂时无法加入对话，请重试。");
      requestAnimationFrame(() => conversation.current?.focusPrompt());
    } catch (error) {
      setInputError(readErrorMessage(error));
    }
  }

  function showConversation() {
    setLastBoardTab(boardTab);
    setAssistantHidden(false);
    onTabChange("conversation");
  }

  function showBoard(next = boardTab) {
    setLastBoardTab(next);
    onTabChange(next);
  }

  const showAssistant = !assistantHidden || tab === "conversation";
  const canReference = writable && session !== undefined;
  return (
    <section
      hidden={!open}
      aria-label={assistantOnly ? "画布对话" : "画布创作工作台"}
      className={cn(
        "absolute z-40 bg-background",
        assistantOnly
          ? "inset-x-0 top-[60px] bottom-0 border-t md:left-auto md:w-(--canvas-conversation-width) md:border-l"
          : "inset-0",
      )}
      style={
        assistantOnly
          ? ({
              "--canvas-conversation-width": `${assistantWidth}px`,
            } as CSSProperties)
          : undefined
      }
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
        if (tab === "conversation" && !assistantOnly) showBoard();
        else onClose();
      }}
    >
      {assistantOnly && (
        <div
          role="separator"
          tabIndex={0}
          aria-label="调整对话宽度"
          aria-orientation="vertical"
          aria-controls="canvas-storyboard-assistant"
          aria-valuemin={minAssistantWidth}
          aria-valuemax={maxAssistantWidth}
          aria-valuenow={assistantWidth}
          aria-valuetext={`${assistantWidth} 像素`}
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
            setAssistantWidth((width) => clampAssistantWidth(width + delta));
          }}
          onPointerDown={(event) => {
            if (event.button !== 0) return;
            event.currentTarget.setPointerCapture(event.pointerId);
          }}
          onPointerMove={(event) => {
            const panel = event.currentTarget.parentElement;
            if (panel && event.currentTarget.hasPointerCapture(event.pointerId))
              setAssistantWidth(
                clampAssistantWidth(
                  panel.getBoundingClientRect().right - event.clientX,
                ),
              );
          }}
        />
      )}
      <div className="flex h-full min-h-0 flex-col">
        <header
          className={cn(
            "h-[60px] shrink-0 items-center gap-1 border-b px-2 sm:gap-3 sm:px-5",
            assistantOnly ? "hidden" : "flex",
          )}
        >
          <Button
            variant="ghost"
            size="icon-sm"
            className="max-sm:size-11"
            aria-label="返回工作流"
            onClick={onClose}
          >
            <ArrowLeft />
          </Button>
          <span
            className="min-w-0 truncate text-sm font-medium sm:max-w-52"
            title={title}
          >
            {title}
          </span>
          <span
            className="hidden text-xs text-muted-foreground sm:block"
            role="status"
          >
            {saveStatus}
          </span>
          <nav
            aria-label="画布视图"
            className="ml-auto flex shrink-0 gap-1 sm:ml-3"
          >
            <Button
              variant="ghost"
              size="sm"
              className="max-sm:h-11"
              onClick={onClose}
            >
              工作流
            </Button>
            <Button
              variant="secondary"
              size="sm"
              className="max-sm:h-11"
              aria-current="page"
              onClick={() => showBoard()}
            >
              故事板
            </Button>
          </nav>
          <div className="flex shrink-0 items-center gap-1 sm:ml-auto">
            <Button
              variant={showAssistant ? "secondary" : "ghost"}
              size="sm"
              className="hidden lg:inline-flex"
              aria-controls="canvas-storyboard-assistant"
              aria-expanded={showAssistant}
              onClick={() => {
                if (showAssistant) {
                  setAssistantHidden(true);
                  showBoard();
                } else {
                  showConversation();
                  requestAnimationFrame(() =>
                    conversation.current?.focusPrompt(),
                  );
                }
              }}
            >
              <PanelRight data-icon="inline-start" /> AI 对话
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              className="max-sm:size-11"
              aria-label="账号设置"
              disabled={!canNavigate}
              asChild={canNavigate}
            >
              {canNavigate ? (
                <Link href="/account">
                  <UserRound />
                </Link>
              ) : (
                <UserRound />
              )}
            </Button>
          </div>
        </header>
        <div className="flex min-h-0 flex-1">
          <main
            className={cn(
              "min-w-0 flex-1 overflow-y-auto overscroll-contain",
              assistantOnly
                ? "hidden"
                : tab === "conversation" && "hidden lg:block",
            )}
          >
            <div className="mx-auto max-w-[1280px] px-4 py-3 lg:px-8 lg:py-4">
              <h1
                ref={heading}
                tabIndex={-1}
                className="mb-2 text-xl font-semibold outline-none lg:text-2xl"
              >
                故事板
              </h1>
              <Tabs
                value={boardTab}
                onValueChange={(next) =>
                  showBoard(next === "materials" ? "materials" : "shots")
                }
                className="relative min-w-0 gap-3"
              >
                <TabsList
                  variant="navigation"
                  aria-label="故事板内容"
                  className="max-lg:h-11 lg:absolute lg:top-0 lg:left-0"
                >
                  <TabsTrigger value="shots" className="max-lg:min-h-11">
                    镜头
                  </TabsTrigger>
                  <TabsTrigger value="materials" className="max-lg:min-h-11">
                    项目素材
                  </TabsTrigger>
                </TabsList>
                <TabsContent
                  value="shots"
                  forceMount
                  className="min-w-0 data-[state=inactive]:hidden"
                >
                  <CanvasStoryboard
                    active={open && !assistantOnly}
                    canvasID={canvasID}
                    projectID={projectID}
                    commitSeq={commitSeq}
                    writable={boardWritable}
                    canReference={canReference}
                    proposal={proposal}
                    onCloseProposal={() => setProposal(undefined)}
                    onDiscuss={() => {
                      showConversation();
                      requestAnimationFrame(() =>
                        conversation.current?.focusPrompt(),
                      );
                    }}
                    onReference={(reference) =>
                      appendInput((current) => current.appendText(reference))
                    }
                  />
                </TabsContent>
                <TabsContent
                  value="materials"
                  forceMount
                  className="min-w-0 data-[state=inactive]:hidden"
                >
                  <CanvasStoryboardMaterials
                    active={open && !assistantOnly && boardTab === "materials"}
                    actorID={actorID}
                    projectID={projectID}
                    canReference={canReference}
                    writable={writable}
                    onReference={(asset) =>
                      appendInput((current) =>
                        current.attachReference(
                          mediaBinding(asset, `reference_${asset.kind}`),
                        ),
                      )
                    }
                  />
                </TabsContent>
              </Tabs>
            </div>
          </main>
          <aside
            id="canvas-storyboard-assistant"
            aria-label="AI 对话"
            className={cn(
              "min-h-0 min-w-0 flex-col bg-popover",
              assistantOnly
                ? "flex flex-1"
                : [
                    "lg:w-[clamp(20rem,29vw,25.5rem)] lg:shrink-0 lg:border-l",
                    tab === "conversation"
                      ? "flex flex-1 lg:flex-none"
                      : "hidden",
                    showAssistant && "lg:flex",
                  ],
            )}
          >
            <div className="flex h-14 shrink-0 items-center justify-between border-b px-4 sm:px-5">
              <h2
                ref={assistantHeading}
                tabIndex={-1}
                className="text-base font-semibold outline-none"
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
                  onClick={() => {
                    if (assistantOnly) onClose();
                    else {
                      setAssistantHidden(true);
                      showBoard();
                    }
                  }}
                >
                  <X />
                </Button>
              </div>
            </div>
            <div
              className={cn(
                "flex min-h-0 flex-1 flex-col gap-2 pt-4",
                assistantOnly
                  ? "px-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]"
                  : "px-4 pb-3 lg:px-5 lg:pb-5",
              )}
            >
              {inputError && <FeedbackAlert tone="error" title={inputError} />}
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
                  <p role="status" className="text-sm text-muted-foreground">
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
                  compactEntry={assistantOnly}
                  panelEmptyMessage={
                    assistantOnly ? "输入想法，或添加参考素材。" : undefined
                  }
                  textReference={storyboardReferenceLabel}
                  replyAction={(text, input) => {
                    const suggested = storyboardActionProposal(text, input);
                    if (suggested)
                      return (
                        <Button
                          variant="outline"
                          size="sm"
                          className="max-lg:h-11"
                          disabled={!boardWritable}
                          onClick={() => {
                            showBoard("shots");
                            setProposal(suggested);
                          }}
                        >
                          查看镜头建议
                        </Button>
                      );
                    if (!input || input.storyboard_targets?.length) return;
                    return storyboardDirections(text).map((direction) => (
                      <Button
                        key={direction}
                        variant="outline"
                        size="sm"
                        className="h-auto min-h-8 max-w-full py-1.5 whitespace-normal max-lg:min-h-11"
                        onClick={() => {
                          appendInput((current) =>
                            current.appendText(
                              `按「${direction}」方向写分镜。`,
                            ),
                          );
                        }}
                      >
                        选用「{direction}」
                      </Button>
                    ));
                  }}
                  onChooseProject={() => {}}
                />
              )}
            </div>
          </aside>
        </div>
        <nav
          aria-label="故事板工作区"
          className={cn(
            "shrink-0 gap-2 border-t bg-popover px-4 pt-2 pb-[max(0.5rem,env(safe-area-inset-bottom))] lg:hidden",
            assistantOnly ? "hidden" : "flex",
          )}
        >
          <Button
            variant={tab !== "conversation" ? "secondary" : "ghost"}
            size="sm"
            className="h-11 px-5"
            aria-pressed={tab !== "conversation"}
            onClick={() => showBoard()}
          >
            故事板
          </Button>
          <Button
            variant={tab === "conversation" ? "secondary" : "ghost"}
            size="sm"
            className="h-11 px-5"
            aria-pressed={tab === "conversation"}
            onClick={showConversation}
          >
            AI 对话
          </Button>
        </nav>
      </div>
    </section>
  );
}
