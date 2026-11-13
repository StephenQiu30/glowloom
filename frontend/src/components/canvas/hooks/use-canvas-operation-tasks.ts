"use client";

import { useCallback, useEffect } from "react";
import useSWR from "swr";
import { listProjectOperations } from "@/api/operations";
import { requireTask } from "@/components/generation/contracts";
import { toast } from "@/components/feedback/notification";
import { RequestError } from "@/lib/request";

export function useCanvasOperationTasks({
  projectID,
  canvasID,
  identity,
}: {
  projectID: string;
  canvasID: string;
  identity: string;
}) {
  const query = useSWR(
    ["canvas-operation-tasks", identity, projectID, canvasID],
    async () => {
      const tasks: Record<string, string> = {};
      const seenNodes = new Set<string>();
      const cursors = new Set<string>();
      let cursor: string | undefined;
      do {
        const page = await listProjectOperations({
          pid: projectID,
          canvas_id: canvasID,
          limit: 200,
          cursor,
        });
        if (!Array.isArray(page.items))
          throw new RequestError("invalid_receipt");
        for (const item of page.items) {
          const task = requireTask(item, projectID);
          const source = task.source;
          if (source?.canvas_id !== canvasID || !source.node_id)
            throw new RequestError("invalid_receipt");
          if (seenNodes.has(source.node_id)) continue;
          seenNodes.add(source.node_id);
          if (
            !["draft", "quoted", "expired", "completed"].includes(task.status)
          )
            tasks[source.node_id] = task.id;
        }
        if (
          page.next_cursor != null &&
          (typeof page.next_cursor !== "string" ||
            cursors.has(page.next_cursor))
        )
          throw new RequestError("invalid_receipt");
        cursor = page.next_cursor || undefined;
        if (cursor) cursors.add(cursor);
      } while (cursor);
      return tasks;
    },
    { shouldRetryOnError: false },
  );
  const { mutate } = query;
  const register = useCallback(
    (nodeID: string, operationID: string) => {
      void mutate(
        (previous) =>
          previous?.[nodeID] === operationID
            ? previous
            : { ...previous, [nodeID]: operationID },
        { revalidate: false },
      );
    },
    [mutate],
  );
  const complete = useCallback(
    (nodeID: string, operationID: string) => {
      void mutate(
        (previous) => {
          if (previous?.[nodeID] !== operationID) return previous;
          const next = { ...previous };
          delete next[nodeID];
          return next;
        },
        { revalidate: false },
      );
    },
    [mutate],
  );
  useEffect(() => {
    if (query.error)
      toast.error("生成状态暂时无法读取", {
        id: `canvas-operation-tasks:${identity}:${canvasID}`,
        action: { label: "重试", onClick: () => void mutate() },
      });
  }, [query.error, identity, canvasID, mutate]);
  return { tasks: query.data ?? {}, register, complete };
}
