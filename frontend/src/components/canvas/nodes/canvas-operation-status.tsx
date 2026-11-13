"use client";

import { useEffect, useRef } from "react";
import useSWR from "swr";
import { GenerationPlaceholder } from "@/components/generation/generation-placeholder";
import { getOperation } from "@/api/operations";
import {
  activeTask,
  requireTask,
  type Task,
} from "@/components/generation/contracts";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { Skeleton } from "@/components/ui/skeleton";
import { RequestError } from "@/lib/request";

export function CanvasOperationStatus(props: {
  projectID: string;
  canvasID: string;
  nodeID: string;
  operationID: string;
  identity: string;
  frozen?: boolean;
  onCompleted?: (
    nodeID: string,
    operationID: string,
    task: Task,
  ) => void | Promise<void>;
}) {
  return (
    <OperationStatus
      key={`${props.identity}/${props.projectID}/${props.operationID}`}
      {...props}
    />
  );
}

function OperationStatus({
  projectID,
  canvasID,
  nodeID,
  operationID,
  identity,
  frozen,
  onCompleted,
}: Parameters<typeof CanvasOperationStatus>[0]) {
  const query = useSWR(
    ["operation", identity, projectID, operationID],
    async () => {
      const task = requireTask(
        await getOperation({ id: operationID, project_id: projectID }),
        projectID,
      );
      if (
        task.id !== operationID ||
        task.source?.canvas_id !== canvasID ||
        task.source.node_id !== nodeID
      )
        throw new RequestError("invalid_receipt");
      return task;
    },
    {
      shouldRetryOnError: false,
      refreshInterval: (task) => (activeTask(task?.status) ? 3000 : 0),
    },
  );
  const completed = useRef(false);
  const task = query.data;
  useEffect(() => {
    if (
      task?.status !== "completed" ||
      frozen ||
      completed.current ||
      !onCompleted
    )
      return;
    completed.current = true;
    void Promise.resolve(onCompleted(nodeID, operationID, task)).catch(() => {
      completed.current = false;
      void query.mutate();
    });
  }, [task, frozen, nodeID, operationID, onCompleted, query]);
  const problem = query.error
    ? "生成状态暂时无法读取"
    : task?.status === "failed"
      ? "生成失败"
      : task?.status === "cancelled"
        ? "生成已取消"
        : ["unknown", "manual"].includes(task?.status ?? "")
          ? "生成状态待确认"
          : "";
  return (
    <>
      {problem && (
        <FeedbackToast
          message={problem}
          action={{ label: "重试", onClick: () => void query.mutate() }}
        />
      )}
      {problem ? (
        <Skeleton className="size-full animate-none rounded-[inherit]" />
      ) : (
        <GenerationPlaceholder />
      )}
    </>
  );
}
