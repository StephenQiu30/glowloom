"use client";
import dynamic from "next/dynamic";
import type { PromptDraft } from "@/components/generation/prompt-document";
import { validatePromptDraft } from "@/components/generation/prompt-document";
const TaskDetail = dynamic(
  () =>
    import("@/components/generation/generation-dialog").then(
      (m) => m.TaskDetail,
    ),
  {
    loading: () => (
      <p role="status" className="text-sm text-muted-foreground">
        正在读取原任务…
      </p>
    ),
  },
);
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export function ConversationResults({
  projectID,
  messages = [],
  operationID,
  onReuse,
}: {
  projectID: string;
  messages?: API.PublicMessage[];
  operationID?: string;
  onReuse?: (draft: PromptDraft) => void;
}) {
  const ids = [
    ...new Set([
      ...messages.filter((p) => p.kind === "operation").map((p) => p.content),
      ...(operationID ? [operationID] : []),
    ]),
  ].filter((id) => uuid.test(id));
  return (
    <>
      {ids.map((id) => (
        <TaskDetail
          key={id}
          id={id}
          presentation="asset"
          projectID={projectID}
          onReuse={
            onReuse
              ? (task) => {
                  if (!task.editor_draft) return;
                  try {
                    validatePromptDraft(task.editor_draft);
                    onReuse(task.editor_draft);
                  } catch {
                    /* Historical malformed drafts never replace current input. */
                  }
                }
              : undefined
          }
        />
      ))}
    </>
  );
}
