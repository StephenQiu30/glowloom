import { isProjectID } from "@/components/projects/project";
import { editTimeline, readTimeline } from "../timeline/timeline-model";
import type { CanvasNodeData, CanvasConnection } from "../types/canvas";
import type { CanvasEditorState } from "./canvas-document-adapter";

function nativeRelation(edge: CanvasConnection) {
  return (
    !edge.documentData &&
    (!edge.serviceData ||
      (edge.serviceData.edge_type === "annotation" &&
        !edge.serviceData.binding &&
        !edge.serviceData.role))
  );
}

/** Read the durable content identity; a local replacement is not a ready source. */
export function canvasConnectionMediaAssetId(node: CanvasNodeData): string {
  const refs =
    node.documentData?.fileReferences?.filter(
      (ref) => ref.purpose === "CONTENT",
    ) ?? [];
  const assetID =
    refs.length === 1
      ? refs[0].fileId
      : (node.serviceData?.media_ref?.asset_id ?? node.serviceData?.ref_id);
  if (
    !["image", "video", "audio"].includes(node.type) ||
    node.metadata?.status === "loading" ||
    node.metadata?.status === "error" ||
    refs.length > 1 ||
    !isProjectID(assetID) ||
    /^(blob:|data:)/.test(node.metadata?.content ?? "")
  )
    throw new Error("来源素材尚未就绪或存在多个结果，请先选择一个素材");
  return assetID!;
}

/** Resolve every source first, then return one immutable clip-and-relation candidate. */
export function connectCanvasTimelineSources(
  state: CanvasEditorState,
  sourceIDs: string[],
  targetID: string,
  options: { projectID: string; assets: ReadonlyMap<string, API.AssetSummary> },
): CanvasEditorState {
  if (
    !isProjectID(options.projectID) ||
    !sourceIDs.length ||
    sourceIDs.length > 50 ||
    new Set(sourceIDs).size !== sourceIDs.length
  )
    throw new Error("请选择1至50个不重复的素材节点");
  const target = state.nodes.find((node) => node.id === targetID);
  if (target?.type !== "timeline") throw new Error("时间线已不存在");
  const timeline = readTimeline(target.serviceData?.config?.timeline);
  const assets = sourceIDs.map((nodeID) => {
    const node = state.nodes.find((item) => item.id === nodeID);
    if (!node || nodeID === targetID) throw new Error("来源节点已不存在");
    const assetID = canvasConnectionMediaAssetId(node);
    const asset = options.assets.get(assetID);
    if (
      !asset ||
      asset.id !== assetID ||
      asset.project_id !== options.projectID ||
      asset.kind !== node.type ||
      !Number.isSafeInteger(asset.revision) ||
      asset.revision! < 1
    )
      throw new Error("素材身份或版本尚未确认");
    // Legacy serviceGraph uses node content_version as a display fallback, not an asset pin.
    const pinned = !node.serviceData || !!node.serviceData.config?.editor;
    const ref = node.documentData?.fileReferences?.find(
      (item) => item.purpose === "CONTENT",
    );
    if (pinned && ref?.version !== String(asset.revision))
      throw new Error("来源素材版本已变化，请重新连接");
    if (node.serviceData?.ref_id && node.serviceData.ref_id !== assetID)
      throw new Error("来源素材已变化，请重新连接");
    if (
      node.type !== "image" &&
      (!Number.isSafeInteger(asset.duration_ms) ||
        asset.duration_ms! < 100 ||
        asset.duration_ms! > 86400000)
    )
      throw new Error("素材时长尚未确认");
    if (
      timeline.clips.some((clip) => clip.node_id === nodeID) ||
      state.connections.some(
        (edge) => edge.fromNodeId === nodeID && edge.toNodeId === targetID,
      )
    )
      throw new Error("这项素材已加入时间线，请勿重复连接");
    return {
      asset:
        node.type === "image" ? { ...asset, duration_ms: undefined } : asset,
      nodeID,
    };
  });
  const adjacency = new Map<string, string[]>();
  const add = (from: string, to: string) =>
    adjacency.set(from, [...(adjacency.get(from) ?? []), to]);
  state.connections.forEach((edge) => {
    if (
      edge.documentData ||
      state.nodes.find((n) => n.id === edge.toNodeId)?.type === "timeline"
    )
      add(edge.fromNodeId, edge.toNodeId);
  });
  state.nodes.forEach((node) =>
    node.serviceData?.config?.timeline?.clips?.forEach((clip) => {
      if (clip.node_id) add(clip.node_id, node.id);
    }),
  );
  const visited = new Set<string>(),
    pending = [targetID];
  while (pending.length) {
    const id = pending.pop()!;
    if (sourceIDs.includes(id))
      throw new Error("这条连接会形成循环，请调整连接方向");
    if (visited.has(id)) continue;
    visited.add(id);
    pending.push(...(adjacency.get(id) ?? []));
  }
  const nextTimeline = editTimeline(timeline, { type: "add", assets });
  return {
    ...state,
    nodes: state.nodes.map((node) =>
      node.id === targetID
        ? {
            ...node,
            serviceData: {
              ...node.serviceData,
              id: node.id,
              config: { ...node.serviceData?.config, timeline: nextTimeline },
            },
          }
        : node,
    ),
    connections: [
      ...state.connections,
      ...sourceIDs.map((sourceID): CanvasConnection => {
        const id = crypto.randomUUID();
        return {
          id,
          fromNodeId: sourceID,
          toNodeId: targetID,
          serviceData: {
            id,
            edge_type: "annotation",
            source_node_id: sourceID,
            target_node_id: targetID,
          },
        };
      }),
    ],
  };
}

/** A removed clip no longer owns a visual relation; legacy unlinked clips remain valid. */
export function reconcileTimelineConnections(
  state: CanvasEditorState,
): CanvasEditorState {
  const connections = state.connections.filter((edge) => {
    const target = state.nodes.find((node) => node.id === edge.toNodeId);
    return (
      !nativeRelation(edge) ||
      target?.type !== "timeline" ||
      !!target.serviceData?.config?.timeline?.clips?.some(
        (clip) => clip.node_id === edge.fromNodeId,
      )
    );
  });
  return connections.length === state.connections.length
    ? state
    : { ...state, connections };
}

/** Detaching a canvas relation retains the independently usable project media clip. */
export function detachTimelineSources(
  nodes: CanvasNodeData[],
  previous: CanvasConnection[],
  remaining: CanvasConnection[],
  removedNodeIDs?: Set<string>,
  sourceNodes: CanvasNodeData[] = nodes,
) {
  return nodes.map((node) => {
    const timeline = node.serviceData?.config?.timeline;
    if (node.type !== "timeline" || !timeline) return node;
    let changed = false;
    const clips = timeline.clips?.map((clip) => {
      const sourceID = clip.node_id;
      if (!sourceID) return clip;
      const hadRelation = previous.some(
        (edge) =>
          nativeRelation(edge) &&
          edge.fromNodeId === sourceID &&
          edge.toNodeId === node.id,
      );
      const hasRelation = remaining.some(
        (edge) =>
          nativeRelation(edge) &&
          edge.fromNodeId === sourceID &&
          edge.toNodeId === node.id,
      );
      if (!removedNodeIDs?.has(sourceID) && (!hadRelation || hasRelation))
        return clip;
      changed = true;
      const detached = { ...clip };
      delete detached.node_id;
      const source = sourceNodes.find((item) => item.id === sourceID);
      return {
        ...detached,
        asset_id:
          clip.asset_id ??
          source?.documentData?.fileReferences?.find(
            (ref) => ref.purpose === "CONTENT",
          )?.fileId ??
          source?.serviceData?.ref_id ??
          source?.serviceData?.media_ref?.asset_id,
      };
    });
    return changed
      ? {
          ...node,
          serviceData: {
            ...node.serviceData,
            config: {
              ...node.serviceData?.config,
              timeline: { ...timeline, clips },
            },
          },
        }
      : node;
  });
}
