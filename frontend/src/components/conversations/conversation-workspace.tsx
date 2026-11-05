"use client";
import { useEffect, type ReactNode } from "react";
import { ConversationComposer } from "./conversation-composer";
import { ConversationThread } from "./conversation-thread";
import { useConversation, type ConversationOptions } from "./use-conversation";
import { Button } from "@/components/ui/button";
export function ConversationWorkspace({
  onChooseProject,
  headerActions,
  projectControl,
  onLock,
  embedded = false,
  ...options
}: ConversationOptions & {
  onChooseProject: () => void;
  headerActions?: ReactNode;
  projectControl?: ReactNode;
  onLock?: (locked: boolean) => void;
  embedded?: boolean;
}) {
  const c = useConversation(options);
  useEffect(() => {
    onLock?.(c.phase !== "idle");
    return () => onLock?.(false);
  }, [c.phase, onLock]);
  const identity = `${options.ownerKey}/${options.projectID}/${options.sessionID ?? "new"}`;
  const hasMessages = c.summaries.length > 0;
  return (
    <div
      className={`flex min-h-0 min-w-0 flex-col ${embedded ? (hasMessages ? "h-[min(80dvh,900px)]" : "") : "h-[calc(100dvh-8rem)] min-h-[400px]"}`}
    >
      {c.loading ? (
        <p role="status" className="px-4 py-3 text-sm text-muted-foreground">
          正在读取对话…
        </p>
      ) : null}
      {hasMessages ? (
        <ConversationThread conversation={c} projectID={options.projectID} />
      ) : !embedded ? (
        <div className="flex min-h-32 flex-1 items-center justify-center">
          <h1 className="text-2xl font-medium">你好，想创作什么？</h1>
        </div>
      ) : null}
      {c.error || c.connection !== "connected" || c.unresolved.length > 0 ? (
        <div
          className="mx-auto mb-3 flex w-full max-w-[960px] flex-wrap items-center gap-2 rounded-xl border px-4 py-3 text-sm"
          role="status"
        >
          <p className="min-w-0 flex-1">
            {c.error ||
              (c.unresolved.length
                ? "原任务结果仍待核对，请先读取原记录。"
                : c.connection === "recovering"
                  ? "正在恢复消息连接…"
                  : "消息连接已断开，已受理的任务仍保留。")}
          </p>
          <Button variant="outline" size="sm" onClick={c.refresh}>
            刷新核对
          </Button>
          {c.phase === "unknown" ? (
            <Button variant="outline" size="sm" onClick={() => void c.send()}>
              核对原发送
            </Button>
          ) : null}
        </div>
      ) : null}
      <div className="mx-auto w-full max-w-[960px] shrink-0">
        <ConversationComposer
          key={identity}
          conversation={c}
          contextKey={identity}
          projectID={options.projectID}
          authorized={options.enabled && !c.denied}
          onChooseProject={onChooseProject}
          headerActions={headerActions}
          projectControl={projectControl}
        />
      </div>
    </div>
  );
}
