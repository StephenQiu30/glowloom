"use client";
import {
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
  type ReactNode,
  type Ref,
} from "react";
import {
  ConversationComposer,
  type ConversationComposerHandle,
} from "./conversation-composer";
import { ConversationWelcome } from "./conversation-welcome";
import { isSameSkillVersion, selectSkillVersion } from "./conversation-state";
import { cn } from "cn";
import { ConversationThread } from "./conversation-thread";
import { useConversation, type ConversationOptions } from "./use-conversation";
import { FeedbackNotice } from "@/components/feedback/feedback-notice";
import { ConversationMessageSkeleton } from "@/components/layout/loading-state";
import { Button } from "@/components/ui/button";
import { ProjectMemoryDialog } from "./project-memory-dialog";
import type { ReferenceBinding } from "@/components/generation/prompt-document";

export type ConversationWorkspaceHandle = ConversationComposerHandle & {
  appendText: (text: string) => boolean;
};

export function ConversationWorkspace({
  ref,
  onChooseProject,
  headerActions,
  projectControl,
  projectSetting,
  alternateComposer,
  onLock,
  onUseReply,
  useReplyLabel,
  embedded = false,
  panel = false,
  ...options
}: ConversationOptions & {
  ref?: Ref<ConversationWorkspaceHandle>;
  onChooseProject: () => void;
  headerActions?: ReactNode;
  projectControl?: ReactNode;
  projectSetting?: ReactNode;
  alternateComposer?: ReactNode;
  onLock?: (locked: boolean) => void;
  onUseReply?: (text: string) => void;
  useReplyLabel?: string;
  embedded?: boolean;
  panel?: boolean;
}) {
  const c = useConversation(options);
  useEffect(() => {
    onLock?.(c.phase !== "idle");
    return () => onLock?.(false);
  }, [c.phase, onLock]);
  const identity = `${options.ownerKey}/${options.projectID}/${options.sessionID ?? options.canvasID ?? "new"}`;
  const hasMessages = c.summaries.length > 0;
  const welcome = !embedded && !panel && !options.sessionID && !hasMessages;
  const authorized = options.enabled && options.writable !== false && !c.denied;
  const [memory, setMemory] = useState<{
    project: string;
    statement: string;
  } | null>(null);
  const openMemory = (statement: string) =>
    setMemory({ project: options.projectID, statement });
  const memoryAction =
    authorized && options.projectID ? (
      <Button variant="ghost" size="sm" onClick={() => openMemory("")}>
        项目记忆
      </Button>
    ) : null;
  const composer = useRef<ConversationComposerHandle>(null);
  useImperativeHandle(ref, () => ({
    focusPrompt: () => composer.current?.focusPrompt(),
    attachReference: (reference: ReferenceBinding) =>
      authorized && (composer.current?.attachReference(reference) ?? false),
    appendText: (text) => {
      if (!authorized || !text.trim()) return false;
      const content = c.draft.document.content;
      if (
        !content.some(
          (paragraph) =>
            paragraph.content?.length === 1 &&
            paragraph.content[0]?.type === "text" &&
            paragraph.content[0].text === text,
        )
      )
        c.changeDraft({
          ...c.draft,
          document: {
            ...c.draft.document,
            content: [
              ...(content.some((paragraph) => paragraph.content?.length)
                ? content
                : []),
              { type: "paragraph", content: [{ type: "text", text }] },
            ],
          },
        });
      return true;
    },
  }));
  return (
    <div
      data-conversation-welcome={welcome || undefined}
      className={cn(
        "flex min-h-0 min-w-0 flex-col",
        embedded ? hasMessages && "h-[min(80dvh,900px)]" : "flex-1",
      )}
    >
      {c.loading && !welcome ? (
        <span role="status" className="sr-only">
          正在读取对话…
        </span>
      ) : null}
      {hasMessages ? (
        <ConversationThread
          conversation={c}
          writable={authorized}
          projectID={options.projectID}
          onUseReply={onUseReply}
          onDraftProjectMemory={memoryAction ? openMemory : undefined}
          useReplyLabel={useReplyLabel}
        />
      ) : welcome ? (
        <ConversationWelcome
          skills={
            authorized && !c.loading && !alternateComposer ? c.skills : []
          }
          showStarters={!alternateComposer}
          selected={c.settings.skills}
          disabled={c.active || c.phase !== "idle"}
          onSelect={(prompt, skill) => {
            if (skill) {
              if (
                !c.settings.skills.some((pin) => isSameSkillVersion(pin, skill))
              ) {
                c.changeSettings({
                  ...c.settings,
                  skills: selectSkillVersion(c.settings.skills, skill),
                });
              }
            } else {
              const content = c.draft.document.content;
              const hasStarter = content.some(
                (paragraph) =>
                  paragraph.content?.length === 1 &&
                  paragraph.content[0]?.type === "text" &&
                  paragraph.content[0].text === prompt,
              );
              if (!hasStarter) {
                c.changeDraft({
                  ...c.draft,
                  document: {
                    ...c.draft.document,
                    content: [
                      ...(content.some((paragraph) => paragraph.content?.length)
                        ? content
                        : []),
                      {
                        type: "paragraph",
                        content: [{ type: "text", text: prompt }],
                      },
                    ],
                  },
                });
              }
            }
            composer.current?.focusPrompt();
          }}
        />
      ) : c.loading ? (
        <ConversationMessageSkeleton className="mx-auto w-full max-w-[800px] flex-1 px-4 py-6" />
      ) : !embedded ? (
        panel && authorized ? (
          <div className="flex flex-1 items-center justify-center px-4 text-center text-muted-foreground">
            聊聊你想拍什么，或直接请 AI 写分镜。
          </div>
        ) : (
          <div className="flex-1" />
        )
      ) : null}
      {c.phase === "unknown" ||
      c.error ||
      c.connection !== "connected" ||
      c.unresolved.length > 0 ? (
        <div className="mx-auto mb-3 w-full max-w-[800px] empty:hidden">
          <FeedbackNotice
            key={`${identity}/${c.phase === "unknown" ? "unknown" : c.error || c.connection}`}
            message={
              c.phase === "unknown"
                ? "发送结果待确认"
                : c.error ||
                  (c.unresolved.length
                    ? "任务结果待确认"
                    : c.connection === "recovering"
                      ? "正在恢复消息连接…"
                      : "消息连接已断开")
            }
            description={
              c.phase === "unknown"
                ? c.canRetrySubmission
                  ? "草稿已保留，可重试发送。"
                  : "草稿已保留，请先查看状态。"
                : undefined
            }
            tone={
              c.phase === "unknown" ||
              c.connection !== "connected" ||
              c.unresolved.length
                ? "info"
                : "error"
            }
            action={{
              label:
                c.phase === "unknown" && c.canRetrySubmission
                  ? "重试发送"
                  : "查看状态",
              onClick: () => {
                if (c.phase === "unknown" && c.canRetrySubmission)
                  void c.send();
                else c.refresh();
              },
            }}
          />
        </div>
      ) : null}
      <div
        className={cn(
          "mx-auto w-full shrink-0",
          alternateComposer && "conversation-direct-surface",
          welcome ? "max-w-[800px]" : "max-w-[960px]",
        )}
      >
        {options.writable === false ? (
          <p className="px-4 pb-4 text-sm text-muted-foreground">
            当前画布为只读。
          </p>
        ) : (
          (alternateComposer ?? (
            <ConversationComposer
              key={identity}
              ref={composer}
              conversation={c}
              contextKey={identity}
              projectID={options.projectID}
              authorized={authorized}
              onChooseProject={onChooseProject}
              headerActions={
                <>
                  {headerActions}
                  {memoryAction}
                </>
              }
              projectControl={projectControl}
              projectSetting={projectSetting}
              welcome={welcome}
            />
          ))
        )}
        {alternateComposer && memoryAction}
      </div>
      {authorized && memory && memory.project === options.projectID && (
        <ProjectMemoryDialog
          projectID={memory.project}
          initialStatement={memory.statement}
          onClose={() => setMemory(null)}
        />
      )}
    </div>
  );
}
