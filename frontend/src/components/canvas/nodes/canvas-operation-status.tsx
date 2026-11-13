"use client";

import { useEffect, useRef, useState } from "react";
import useSWR from "swr";
import { GenerationPlaceholder } from "@/components/generation/generation-placeholder";
import { getOperation } from "@/api/operations";
import {
  activeTask,
  generationStatusLabel,
  requireTask,
  type Task,
} from "@/components/generation/contracts";
import { toast } from "@/components/feedback/notification";
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
  onStopped?: () => void;
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
  onStopped,
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
      !onStopped ||
      !task ||
      !["manual", "failed", "cancelled"].includes(task.status)
    )
      return;
    const message = generationStatusLabel(task);
    if (
      task.status === "cancelled" ||
      (task.status === "manual" && message !== "生成失败")
    )
      toast.info(message, { id: `canvas-operation:${task.id}` });
    else toast.error(message, { id: `canvas-operation:${task.id}` });
    onStopped();
  }, [onStopped, task]);
  const [deliveryError, setDeliveryError] = useState("");
  useEffect(() => {
    if (
      task?.status !== "completed" ||
      frozen ||
      deliveryError ||
      completed.current ||
      !onCompleted
    )
      return;
    completed.current = true;
    void Promise.resolve()
      .then(() => onCompleted(nodeID, operationID, task))
      .catch((cause: unknown) => {
        completed.current = false;
        setDeliveryError(
          cause instanceof Error ? cause.message : "生成结果暂时无法显示",
        );
      });
  }, [task, frozen, nodeID, operationID, onCompleted, deliveryError]);
  const problem =
    deliveryError ||
    (query.error
      ? "生成状态暂时无法读取"
      : onStopped &&
          task &&
          ["manual", "failed", "cancelled"].includes(task.status)
        ? ""
        : task?.status === "failed"
          ? "生成失败"
          : task?.status === "cancelled"
            ? "生成已取消"
            : task?.status === "manual"
              ? generationStatusLabel(task)
              : "");
  return (
    <>
      {problem && (
        <FeedbackToast
          message={problem}
          action={{
            label: "重试",
            onClick: () =>
              deliveryError ? setDeliveryError("") : void query.mutate(),
          }}
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
