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
  type DraftTextReference,
  paragraphTextReference,
} from "./conversation-composer";
import { ConversationWelcome } from "./conversation-welcome";
import {
  isSameSkillVersion,
  isVersionedSkill,
  selectSkillVersion,
} from "./conversation-state";
import { cn } from "cn";
import { ConversationThread } from "./conversation-thread";
import { useConversation, type ConversationOptions } from "./use-conversation";
import { CircleAlert } from "lucide-react";
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
  replyAction,
  textReference,
  useReplyLabel,
  embedded = false,
  panel = false,
  panelEmptyMessage,
  compactEntry = false,
  homeGeneration = false,
  independent = false,
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
  replyAction?: (text: string, input?: API.AuthorizedInput) => ReactNode;
  textReference?: DraftTextReference;
  useReplyLabel?: string;
  embedded?: boolean;
  panel?: boolean;
  panelEmptyMessage?: string | null;
  compactEntry?: boolean;
  homeGeneration?: boolean;
}) {
  const c = useConversation({ ...options, independent });
  useEffect(() => {
    onLock?.(c.phase !== "idle");
    return () => onLock?.(false);
  }, [c.phase, onLock]);
  const identity = `${options.ownerKey}/${options.projectID}/${options.sessionID ?? options.canvasID ?? "new"}`;
  const hasMessages = c.summaries.length > 0;
  const welcome = !embedded && !panel && !options.sessionID && !hasMessages;
  const authorized = options.enabled && options.writable !== false && !c.denied;
  const suggestedSkills =
    homeGeneration && authorized && !c.loading
      ? c.skills
          .filter(isVersionedSkill)
          .filter((skill) => skill.enabled)
          .slice(0, 4)
      : [];
  const [memory, setMemory] = useState<{
    project: string;
    statement: string;
  } | null>(null);
  const openMemory = (statement: string) =>
    setMemory({ project: options.projectID, statement });
  const memoryAction =
    authorized && options.projectID && !independent ? (
      <Button
        variant="ghost"
        size="toolbar"
        aria-label="项目记忆"
        title="项目记忆"
        onClick={() => openMemory("")}
      >
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
      const entry = textReference?.(text);
      const content = entry
        ? c.draft.document.content.filter(
            (paragraph) =>
              paragraphTextReference(paragraph, textReference)?.key !==
              entry.key,
          )
        : c.draft.document.content;
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
          replyAction={authorized ? replyAction : undefined}
          displayInput={
            textReference
              ? (text) =>
                  text
                    .split("\n")
                    .map((line) => textReference(line)?.label ?? line)
                    .join("\n")
              : undefined
          }
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
        panel && panelEmptyMessage === null ? (
          <div className="flex-1" />
        ) : panel && authorized && panelEmptyMessage ? (
          <p className="flex-1 text-sm text-muted-foreground">
            {panelEmptyMessage}
          </p>
        ) : panel && authorized && textReference ? (
          <div className="flex-1 space-y-1 overflow-y-auto py-2">
            <h3 className="text-base font-medium">想拍什么？</h3>
            <p className="text-sm text-muted-foreground">
              说说创意，或引用镜头和素材。
            </p>
          </div>
        ) : panel && authorized ? (
          <div className="flex flex-1 items-center justify-center px-4 text-center text-muted-foreground">
            聊聊你想创作什么。
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
          welcome ? "max-w-[800px]" : "max-w-[1014px]",
        )}
      >
        {c.modelCatalogError && !alternateComposer && (
          <div
            className="mb-2 flex items-center gap-2 px-2 text-sm text-muted-foreground"
            role="status"
          >
            <CircleAlert
              className="size-4 shrink-0 text-destructive"
              aria-hidden="true"
            />
            <span className="min-w-0 flex-1">无法读取对话模型</span>
            <Button
              type="button"
              variant="ghost"
              size="toolbar"
              disabled={c.loading || c.phase !== "idle"}
              onClick={c.refresh}
            >
              {c.loading ? "正在重试…" : "重试"}
            </Button>
          </div>
        )}
        {options.writable === false ? (
          <p className="px-4 pb-4 text-sm text-muted-foreground">
            {panel ? "当前画布为只读。" : "当前空间为只读。"}
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
                  {!textReference &&
                    !compactEntry &&
                    !homeGeneration &&
                    memoryAction}
                </>
              }
              projectControl={projectControl}
              projectSetting={
                textReference || compactEntry || homeGeneration ? (
                  <>
                    {projectSetting}
                    {memoryAction}
                  </>
                ) : (
                  projectSetting
                )
              }
              welcome={welcome}
              compactEntry={compactEntry}
              canvasPanel={panel && !!options.canvasID}
              homeGeneration={homeGeneration || (!panel && !embedded)}
              textReference={textReference}
            />
          ))
        )}
        {suggestedSkills.length > 0 && (
          <div
            role="group"
            aria-label="推荐技能"
            className="mt-3 flex flex-wrap gap-2"
          >
            {suggestedSkills.map((skill) => {
              const selected = c.settings.skills.some((pin) =>
                isSameSkillVersion(pin, skill),
              );
              return (
                <Button
                  key={`${skill.skill_id}/${skill.version}`}
                  variant="outline"
                  size="skill"
                  className="max-md:min-h-11"
                  aria-pressed={selected}
                  disabled={c.active || c.phase !== "idle"}
                  onClick={() => {
                    c.changeSettings({
                      ...c.settings,
                      skills: selected
                        ? c.settings.skills.filter(
                            (pin) => !isSameSkillVersion(pin, skill),
                          )
                        : selectSkillVersion(c.settings.skills, skill),
                    });
                    composer.current?.focusPrompt();
                  }}
                >
                  {skill.name || "创作技能"}
                </Button>
              );
            })}
          </div>
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
