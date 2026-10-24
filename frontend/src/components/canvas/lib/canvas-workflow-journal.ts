import { pageBusinessMemory } from "@/lib/page-business-memory";
/** Page-local command identity; it is not a substitute for the server submission fence. */
export function readWorkflowCommand(
  key: string,
  canvasId: string,
): CanvasContract.WorkflowSubmitRequest | null {
  const raw = pageBusinessMemory.getItem(key);
  if (!raw) return null;
  if (raw.length > 8192) throw new Error("原运行命令记录超出预算");
  const value: unknown = JSON.parse(raw);
  if (!value || typeof value !== "object")
    throw new Error("原运行命令记录无效");
  const request = value as CanvasContract.WorkflowSubmitRequest & {
    schemaVersion?: number;
  };
  if (
    (request.schemaVersion != null && request.schemaVersion !== 1) ||
    String(request.canvasId) !== canvasId ||
    typeof request.commandId !== "string" ||
    !Array.isArray(request.nodeIds) ||
    request.nodeIds.length < 1 ||
    request.nodeIds.length > 20 ||
    !request.nodeIds.every((id) => typeof id === "string") ||
    request.expectedRevision == null ||
    request.expectedProjectBindingRevision == null ||
    typeof request.expectedEpoch !== "string" ||
    !/^[a-f0-9-]{36}$/.test(request.expectedEpoch) ||
    typeof request.expectedCommitSeq !== "string" ||
    !/^(0|[1-9][0-9]{0,18})$/.test(request.expectedCommitSeq) ||
    typeof request.expectedGraphHash !== "string" ||
    !/^[a-f0-9]{64}$/.test(request.expectedGraphHash)
  )
    throw new Error("原运行命令记录无效");
  return {
    commandId: request.commandId,
    canvasId: request.canvasId,
    nodeIds: request.nodeIds,
    expectedRevision: request.expectedRevision,
    expectedProjectBindingRevision: request.expectedProjectBindingRevision,
    expectedEpoch: request.expectedEpoch,
    expectedCommitSeq: request.expectedCommitSeq,
    expectedGraphHash: request.expectedGraphHash,
  };
}

export function claimWorkflowCommand(
  key: string,
  canvasId: string,
  request: CanvasContract.WorkflowSubmitRequest,
  fresh: boolean,
) {
  return Promise.resolve().then(() => {
    const current = readWorkflowCommand(key, canvasId);
    if (fresh ? current != null : current?.commandId !== request.commandId)
      throw new Error("本页运行命令已更新，请先核对原运行回执");
    if (
      current &&
      JSON.stringify(current) !==
        JSON.stringify({
          commandId: request.commandId,
          canvasId: request.canvasId,
          nodeIds: request.nodeIds,
          expectedRevision: request.expectedRevision,
          expectedProjectBindingRevision:
            request.expectedProjectBindingRevision,
          expectedEpoch: request.expectedEpoch,
          expectedCommitSeq: request.expectedCommitSeq,
          expectedGraphHash: request.expectedGraphHash,
        })
    )
      throw new Error("原运行命令内容不一致，请保留原记录核对");
    pageBusinessMemory.setItem(
      key,
      JSON.stringify({ ...request, schemaVersion: 1 }),
    );
  });
}

export function clearWorkflowCommand(
  key: string,
  canvasId: string,
  commandId: string,
) {
  return Promise.resolve().then(() => {
    const current = readWorkflowCommand(key, canvasId);
    if (current?.commandId === commandId) pageBusinessMemory.removeItem(key);
    return readWorkflowCommand(key, canvasId);
  });
}
