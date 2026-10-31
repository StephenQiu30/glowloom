import { isProjectID } from "@/components/projects/project";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
import type { CanvasConnection, CanvasNodeData } from "../types/canvas";
import {
  localSubjectConfig,
  materializeSubject,
  projectSubjectMedia,
  setSubjectConfig,
  subjectConfig,
  subjectLocalMedia,
  type CanvasSubjectMedia,
} from "../subject/subject-node-state";
import type { CanvasEditorState } from "./canvas-document-adapter";
import { canvasConnectionMediaAssetId } from "./canvas-timeline-connections";

export type ResolvedCanvasSubjectSource = {
  sourceNodeId: string;
  asset: API.AssetSummary;
};

/** Build one complete subject assignment and relation candidate; never publish a subset. */
export function connectCanvasSubjectSources(
  state: CanvasEditorState,
  sourceIDs: string[],
  targetID: string,
  resolved: readonly ResolvedCanvasSubjectSource[],
  projectID: string,
  pinnedSubject?: UnifiedSubject,
): CanvasEditorState {
  if (
    !isProjectID(projectID) ||
    !sourceIDs.length ||
    sourceIDs.length > 5 ||
    new Set(sourceIDs).size !== sourceIDs.length
  )
    throw new Error("请选择1至5个不重复的来源节点");
  const target = state.nodes.find((node) => node.id === targetID);
  if (target?.type !== "subject") throw new Error("主体目标已不存在");
  const config = subjectConfig(target);
  const local = materializeSubject(config, pinnedSubject);
  if (
    resolved.length !== sourceIDs.length ||
    new Set(resolved.map((entry) => entry.sourceNodeId)).size !==
      resolved.length
  )
    throw new Error("来源素材尚未全部就绪");
  const media = sourceIDs.map((sourceID): CanvasSubjectMedia => {
    const source = state.nodes.find((node) => node.id === sourceID);
    const asset = resolved.find(
      (entry) => entry.sourceNodeId === sourceID,
    )?.asset;
    if (
      !source ||
      sourceID === targetID ||
      source.metadata?.status === "loading" ||
      source.metadata?.status === "error"
    )
      throw new Error("来源素材尚未就绪");
    const assetID = canvasConnectionMediaAssetId(source);
    if (!asset || asset.id !== assetID || asset.kind !== source.type)
      throw new Error("来源素材已变化，请重新连接");
    const pinned = !source.serviceData || !!source.serviceData.config?.editor;
    const ref = source.documentData?.fileReferences?.find(
      (item) => item.purpose === "CONTENT",
    );
    if (
      (pinned && ref?.version !== String(asset.revision)) ||
      (source.serviceData?.ref_id && source.serviceData.ref_id !== assetID)
    )
      throw new Error("来源素材版本已变化，请重新连接");
    if (
      state.connections.some(
        (edge) => edge.fromNodeId === sourceID && edge.toNodeId === targetID,
      )
    )
      throw new Error("这项素材已加入主体，请勿重复连接");
    if (
      asset.kind === "audio" &&
      (!Number.isSafeInteger(asset.duration_ms) ||
        asset.duration_ms! < 2000 ||
        asset.duration_ms! > 5000)
    )
      throw new Error("音色素材时长应为2至5秒");
    if (
      asset.kind === "video" &&
      (!Number.isSafeInteger(asset.duration_ms) ||
        asset.duration_ms! <= 0 ||
        Math.round(asset.duration_ms! / 1000) > 15)
    )
      throw new Error("视频素材时长尚未确认或超过15秒");
    return {
      ...projectSubjectMedia(asset, projectID),
      source_node_id: sourceID,
    };
  });
  const seenAssets = new Set(
    subjectLocalMedia(local).map((item) => item.asset_id),
  );
  for (const item of media) {
    if (seenAssets.has(item.asset_id))
      throw new Error("主体素材重复，请调整选择");
    seenAssets.add(item.asset_id);
  }
  for (const item of subjectLocalMedia(local)) {
    if (!item.source_node_id) continue;
    const source = state.nodes.find((node) => node.id === item.source_node_id);
    if (!source || canvasConnectionMediaAssetId(source) !== item.asset_id)
      throw new Error("主体来源已变化，请重新连接或移除引用");
    const pinned = !source.serviceData || !!source.serviceData.config?.editor;
    const ref = source.documentData?.fileReferences?.find(
      (entry) => entry.purpose === "CONTENT",
    );
    if (
      (pinned && ref?.version !== String(item.asset_revision)) ||
      !state.connections.some(
        (edge) =>
          edge.fromNodeId === source.id &&
          edge.toNodeId === targetID &&
          !edge.documentData &&
          edge.serviceData?.edge_type === "annotation" &&
          !edge.serviceData.binding &&
          !edge.serviceData.role,
      )
    )
      throw new Error("主体来源绑定已变化，请重新连接或移除引用");
  }
  const adjacency = new Map<string, string[]>();
  for (const edge of state.connections)
    adjacency.set(edge.fromNodeId, [
      ...(adjacency.get(edge.fromNodeId) ?? []),
      edge.toNodeId,
    ]);
  const pending = [targetID],
    visited = new Set<string>();
  while (pending.length) {
    const id = pending.pop()!;
    if (sourceIDs.includes(id))
      throw new Error("这条连接会形成循环，请调整连接方向");
    if (visited.has(id)) continue;
    visited.add(id);
    pending.push(...(adjacency.get(id) ?? []));
  }
  let remaining = media;
  if (!local.primary) {
    const main = media.find((item) => item.kind !== "audio");
    if (!main) throw new Error("请先添加图片或视频主来源");
    local.primary = main;
    remaining = media.filter((item) => item !== main);
  }
  if (local.primary.kind === "video" && remaining.length)
    throw new Error("视频主来源不能追加辅助素材或音色");
  for (const item of remaining) {
    if (item.kind === "image") {
      if (local.references.length >= 3)
        throw new Error("图片主体最多添加3个辅助来源");
      local.references.push(item);
    } else if (item.kind === "audio") {
      if (local.voice) throw new Error("主体已有音色，请先移除原音色");
      const asset = resolved.find(
        (entry) => entry.sourceNodeId === item.source_node_id,
      )!.asset;
      local.voice = {
        name: asset.file_name?.trim().slice(0, 512) || "音色",
        media: item,
      };
    } else throw new Error("图片主来源只能追加图片辅助来源或音色");
  }
  if (
    local.references.length > 3 ||
    (local.primary.kind === "video" && (local.references.length || local.voice))
  )
    throw new Error("当前主体素材组合超出连接容量，请先调整");
  const nextTarget = setSubjectConfig(
    target,
    localSubjectConfig(config, local),
  );
  const edges = sourceIDs.map((sourceID): CanvasConnection => {
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
  });
  return {
    ...state,
    nodes: state.nodes.map((node) =>
      node.id === targetID ? nextTarget : node,
    ),
    connections: [...state.connections, ...edges],
  };
}

/** Detaching freezes the already pinned asset; it never silently follows a replacement. */
export function detachSubjectSources(
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
): CanvasNodeData[] {
  const ids = new Set(nodes.map((node) => node.id));
  return nodes.map((node) => {
    if (node.type !== "subject" || !node.serviceData?.config?.subject?.local)
      return node;
    const config = subjectConfig(node),
      local = structuredClone(config.local!);
    let changed = false;
    for (const media of subjectLocalMedia(local)) {
      if (!media.source_node_id) continue;
      const connected =
        ids.has(media.source_node_id) &&
        connections.some(
          (edge) =>
            edge.fromNodeId === media.source_node_id &&
            edge.toNodeId === node.id &&
            !edge.documentData &&
            edge.serviceData?.edge_type === "annotation" &&
            !edge.serviceData.binding &&
            !edge.serviceData.role,
        );
      if (!connected) {
        delete media.source_node_id;
        changed = true;
      }
    }
    return changed ? setSubjectConfig(node, { ...config, local }) : node;
  });
}

/** Manual replacement removes only relations that represented prior subject inputs. */
export function reconcileSubjectSourceRelations(
  before: CanvasEditorState,
  after: CanvasEditorState,
): CanvasEditorState {
  const prior = new Set<string>();
  for (const node of before.nodes) {
    if (node.type !== "subject" || !node.serviceData?.config?.subject?.local)
      continue;
    for (const media of subjectLocalMedia(subjectConfig(node).local!))
      if (media.source_node_id) prior.add(`${media.source_node_id}/${node.id}`);
  }
  const active = new Set<string>();
  for (const node of after.nodes) {
    if (node.type !== "subject" || !node.serviceData?.config?.subject?.local)
      continue;
    for (const media of subjectLocalMedia(subjectConfig(node).local!))
      if (media.source_node_id)
        active.add(`${media.source_node_id}/${node.id}`);
  }
  return {
    ...after,
    connections: after.connections.filter((edge) => {
      const key = `${edge.fromNodeId}/${edge.toNodeId}`;
      return (
        !prior.has(key) ||
        active.has(key) ||
        !!edge.documentData ||
        edge.serviceData?.edge_type !== "annotation" ||
        !!edge.serviceData.binding ||
        !!edge.serviceData.role
      );
    }),
  };
}
