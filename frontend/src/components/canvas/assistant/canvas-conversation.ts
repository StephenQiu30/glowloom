import { listAgentSessions } from "@/api/agent";

export type CanvasConversationScope = {
  projectID: string;
  scopeID: string;
  actorID: string;
  canvasID: string;
};

export async function findCanvasConversation(
  { projectID, scopeID, actorID, canvasID }: CanvasConversationScope,
  signal: AbortSignal,
) {
  const seen = new Set<string>();
  let cursor: string | undefined;
  do {
    const page = await listAgentSessions(
      { pid: projectID, limit: 50, cursor },
      { signal },
    );
    const session = page.items.find(
      (item) =>
        item.id &&
        item.actor_id === actorID &&
        item.scope_id === scopeID &&
        item.project_id === projectID &&
        item.canvas_id === canvasID,
    );
    if (session) return session.id;
    cursor = page.next_cursor;
    if (cursor && seen.has(cursor))
      throw new Error("invalid_agent_session_cursor");
    if (cursor) seen.add(cursor);
  } while (cursor && !signal.aborted);
  return null;
}
