import { getMediaPreview } from "@/api/media";
import type { CanvasYjsLiveSession } from "../yjs-live-session";
import type { CanvasNodeData } from "../types/canvas";
import { canvasGraphFingerprint } from "./canvas-json";
import { canvasConnectionMediaAssetId } from "./canvas-timeline-connections";

export type CanvasDomainConnectionRequest = {
  sourceNodeIds: string[];
  targetNodeId?: string;
  newTarget?: CanvasNodeData;
};

function contentIdentity(node: CanvasNodeData) {
  return canvasGraphFingerprint({
    type: node.type,
    metadata: node.metadata,
    parameters: node.documentData?.parameters,
    files: node.documentData?.fileReferences,
    config: node.serviceData?.config,
    ref: node.serviceData?.ref_id,
    media: node.serviceData?.media_ref,
  });
}

/** Resolve all media first, then recheck the current graph and publish one transaction. */
export function createCanvasDomainConnector(
  client: CanvasYjsLiveSession,
  projectID: string,
) {
  let busy = false;
  let disposed = false;
  const requests = new Set<AbortController>();
  return {
    start() {
      disposed = false;
    },
    dispose() {
      disposed = true;
      for (const request of requests) request.abort();
      requests.clear();
    },
    async connect(input: CanvasDomainConnectionRequest): Promise<string> {
      const request = structuredClone(input);
      if (busy || disposed) throw new Error("正在处理连接，请稍后重试");
      const view = client.getSnapshot();
      const before = view.document;
      const epoch = client.getDocumentEpoch();
      if (!before || !epoch || !view.writable)
        throw new Error("画布暂时无法接收修改，请稍后重试");
      if (!client.canAcceptConnectionGraph(before))
        throw new Error("素材尚未就绪，请稍后重试");
      if (!!request.targetNodeId === !!request.newTarget)
        throw new Error("连接目标无效");
      const target =
        request.newTarget ??
        before.nodes.find((node) => node.id === request.targetNodeId);
      if (!target || target.type !== "timeline")
        throw new Error("连接目标已不存在");
      if (
        !request.sourceNodeIds.length ||
        new Set(request.sourceNodeIds).size !== request.sourceNodeIds.length
      )
        throw new Error("请选择不同的来源节点");
      const sources = request.sourceNodeIds.map((id) => {
        const node = before.nodes.find((item) => item.id === id);
        if (!node) throw new Error("来源节点已不存在");
        return {
          id,
          assetID: canvasConnectionMediaAssetId(
            client.connectionMediaNode(node),
          ),
          identity: contentIdentity(node),
        };
      });
      const targetIdentity = contentIdentity(target);
      const abort = new AbortController();
      requests.add(abort);
      busy = true;
      try {
        const entries = await Promise.all(
          [...new Set(sources.map((source) => source.assetID))].map(
            async (assetID) => {
              const result = await getMediaPreview(
                { pid: projectID, asset_id: assetID },
                { signal: abort.signal },
              );
              if (!result.asset || result.asset.id !== assetID)
                throw new Error("素材身份不匹配");
              return [assetID, result.asset] as const;
            },
          ),
        );
        const assets = new Map(entries);
        abort.signal.throwIfAborted();
        const currentView = client.getSnapshot();
        const current = currentView.document;
        if (
          disposed ||
          !current ||
          !currentView.writable ||
          client.getDocumentEpoch() !== epoch
        )
          throw new Error("画布权限或版本已变化，请重新连接");
        for (const source of sources) {
          const node = current.nodes.find((item) => item.id === source.id);
          if (
            !node ||
            contentIdentity(node) !== source.identity ||
            canvasConnectionMediaAssetId(client.connectionMediaNode(node)) !==
              source.assetID
          )
            throw new Error("来源素材已变化，请重新连接");
        }
        const currentTarget = current.nodes.find(
          (node) => node.id === target.id,
        );
        if (
          request.newTarget
            ? !!currentTarget
            : !currentTarget ||
              contentIdentity(currentTarget) !== targetIdentity
        )
          throw new Error("连接目标已变化，请重新连接");
        const sourceIDs = new Set(request.sourceNodeIds);
        const prepared = {
          ...current,
          nodes: current.nodes.map((node) =>
            sourceIDs.has(node.id) ? client.connectionMediaNode(node) : node,
          ),
        };
        const graph = request.newTarget
          ? { ...prepared, nodes: [...prepared.nodes, request.newTarget] }
          : prepared;
        const resolved = (
          await import("./canvas-timeline-connections")
        ).connectCanvasTimelineSources(
          graph,
          request.sourceNodeIds,
          target.id,
          { projectID, assets },
        );
        const originalNodes = new Map(
          current.nodes.map((node) => [node.id, node]),
        );
        const candidate = {
          ...resolved,
          nodes: resolved.nodes.map((node) =>
            sourceIDs.has(node.id) ? originalNodes.get(node.id)! : node,
          ),
        };
        // Dynamic import is an async boundary too: never overwrite an intervening edit.
        if (
          client.getSnapshot().document !== current ||
          client.getDocumentEpoch() !== epoch ||
          !client.canAcceptConnectionGraph(candidate)
        )
          throw new Error("画布已变化或素材尚未就绪，请重新连接");
        client.change(candidate, true);
        const saved = await client.waitForSaved(() => {
          if (disposed || client.getDocumentEpoch() !== epoch)
            throw new Error("画布版本已变化");
        });
        const previousEdges = new Set(
          current.connections.map((edge) => edge.id),
        );
        const savedEdges = new Set(saved.edges?.map((edge) => edge.id));
        if (
          !saved.nodes?.some((node) => node.id === target.id) ||
          candidate.connections.some(
            (edge) => !previousEdges.has(edge.id) && !savedEdges.has(edge.id),
          )
        )
          throw new Error("连接保存结果尚未确认，请检查画布后重试");
        return target.id;
      } finally {
        abort.abort();
        busy = false;
        requests.delete(abort);
      }
    },
  };
}
