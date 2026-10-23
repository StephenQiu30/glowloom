/** Shared device receipt; it is not a substitute for the server submission fence. */
export function readWorkflowCommand(
  key: string,
  canvasId: string,
): CanvasContract.WorkflowSubmitRequest | null {
  const raw = localStorage.getItem(key);
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

function locked<T>(key: string, action: () => T): Promise<T> {
  if (!navigator.locks)
    return Promise.reject(
      new Error("浏览器不支持安全的多页提交，请使用支持 Web Locks 的浏览器"),
    );
  return navigator.locks.request(key, action);
}

export function claimWorkflowCommand(
  key: string,
  canvasId: string,
  request: CanvasContract.WorkflowSubmitRequest,
  fresh: boolean,
) {
  return locked(key, () => {
    const current = readWorkflowCommand(key, canvasId);
    if (fresh ? current != null : current?.commandId !== request.commandId)
      throw new Error("运行命令已在另一页面更新，请先核对原运行回执");
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
    localStorage.setItem(key, JSON.stringify({ ...request, schemaVersion: 1 }));
  });
}

export function clearWorkflowCommand(
  key: string,
  canvasId: string,
  commandId: string,
) {
  return locked(key, () => {
    const current = readWorkflowCommand(key, canvasId);
    if (current?.commandId === commandId) localStorage.removeItem(key);
    return readWorkflowCommand(key, canvasId);
  });
}
