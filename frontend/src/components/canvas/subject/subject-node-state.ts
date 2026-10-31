import { isProjectID } from "@/components/projects/project";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
import type { CanvasNodeData } from "../types/canvas";
import type { CanvasEditorState } from "../lib/canvas-document-adapter";
import { createCanvasNode } from "../lib/canvas-node-factory";
export type CanvasSubjectConfig = {
  version: 1;
  subject_id?: string;
  version_id?: string;
  local?: CanvasSubjectLocal;
};
export function subjectConfig(node: CanvasNodeData): CanvasSubjectConfig {
  const value = node.serviceData?.config?.subject;
  if (
    node.type !== "subject" ||
    !value ||
    value.version !== 1 ||
    Object.keys(value).some(
      (key) => !["version", "subject_id", "version_id", "local"].includes(key),
    ) ||
    (value.subject_id !== undefined) !== (value.version_id !== undefined) ||
    (value.subject_id !== undefined &&
      (!isProjectID(value.subject_id) || !isProjectID(value.version_id)))
  )
    throw new Error("主体节点配置无效");
  if (value.local) requireSubjectLocal(value.local, !!value.subject_id);
  return value as CanvasSubjectConfig;
}
export function setSubjectConfig(
  node: CanvasNodeData,
  config: CanvasSubjectConfig,
): CanvasNodeData {
  return {
    ...node,
    serviceData: {
      ...node.serviceData,
      config: {
        ...node.serviceData?.config,
        subject: config,
      } as API.NodeConfig,
    },
  };
}
/** All selected immutable versions enter one canvas transaction. */
export function importCanvasSubjects(
  graph: CanvasEditorState,
  nodeID: string,
  subjects: UnifiedSubject[],
): CanvasEditorState {
  const target = graph.nodes.find((node) => node.id === nodeID);
  if (!target || target.type !== "subject") throw new Error("导入目标已不存在");
  const previous = subjectConfig(target);
  if (
    !subjects.length ||
    subjects.length > 32 ||
    new Set(subjects.map((subject) => subject.id)).size !== subjects.length
  )
    throw new Error("请选择1至32个不同主体");
  const nodes = [...graph.nodes];
  subjects.forEach((subject, index) => {
    if (!isProjectID(subject.id) || !isProjectID(subject.version_id))
      throw new Error("主体版本无效");
    const base =
      index === 0
        ? target
        : createCanvasNode(
            "subject",
            {
              x:
                target.position.x +
                index * (target.width + 48) +
                target.width / 2,
              y: target.position.y + target.height / 2,
            },
            undefined,
            nodes,
          );
    const next = setSubjectConfig(
      { ...base, title: subject.name },
      { version: 1, subject_id: subject.id, version_id: subject.version_id },
    );
    if (index === 0) nodes[nodes.indexOf(target)] = next;
    else nodes.push(next);
  });
  const previousSources = new Set(
    previous.local
      ? subjectLocalMedia(previous.local).flatMap((media) =>
          media.source_node_id ? [media.source_node_id] : [],
        )
      : [],
  );
  return {
    ...graph,
    nodes,
    connections: previousSources.size
      ? graph.connections.filter(
          (edge) =>
            edge.toNodeId !== target.id ||
            !previousSources.has(edge.fromNodeId) ||
            !!edge.documentData ||
            edge.serviceData?.edge_type !== "annotation" ||
            !!edge.serviceData.binding ||
            !!edge.serviceData.role,
        )
      : graph.connections,
  };
}

export type CanvasSubjectMedia = {
  asset_id: string;
  asset_revision: number;
  kind: "image" | "video" | "audio";
  binding_id?: string;
  source_node_id?: string;
};
export type CanvasSubjectLocal = {
  description: string;
  primary?: CanvasSubjectMedia;
  references: CanvasSubjectMedia[];
  voice?: { name: string; media: CanvasSubjectMedia };
};
export function subjectLocalMedia(local: CanvasSubjectLocal) {
  return [
    ...(local.primary ? [local.primary] : []),
    ...local.references,
    ...(local.voice ? [local.voice.media] : []),
  ];
}
export function requireSubjectLocal(
  value: unknown,
  pinned: boolean,
): CanvasSubjectLocal {
  const local = value as CanvasSubjectLocal;
  if (
    !local ||
    Object.keys(local).some(
      (key) => !["description", "primary", "references", "voice"].includes(key),
    ) ||
    typeof local.description !== "string" ||
    Array.from(local.description).length > 8192 ||
    local.description.includes("\0") ||
    !Array.isArray(local.references) ||
    local.references.length > 120 ||
    (!local.primary && (local.references.length > 0 || local.voice))
  )
    throw new Error("主体局部配置无效");
  if (
    (local.primary && !["image", "video"].includes(local.primary.kind)) ||
    local.references.some((media) => media.kind !== "image") ||
    (local.voice &&
      (Object.keys(local.voice).some(
        (key) => !["name", "media"].includes(key),
      ) ||
        typeof local.voice.name !== "string" ||
        !local.voice.name.trim() ||
        Array.from(local.voice.name).length > 512 ||
        local.voice.name.includes("\0") ||
        local.voice.media?.kind !== "audio"))
  )
    throw new Error("主体媒体角色无效");
  const ids = new Set<string>();
  for (const media of subjectLocalMedia(local)) {
    if (
      !media ||
      Object.keys(media).some(
        (key) =>
          ![
            "asset_id",
            "asset_revision",
            "kind",
            "binding_id",
            "source_node_id",
          ].includes(key),
      ) ||
      !isProjectID(media.asset_id) ||
      !Number.isSafeInteger(media.asset_revision) ||
      media.asset_revision < 1 ||
      media.asset_revision > 2147483647 ||
      (media.binding_id !== undefined &&
        (!pinned || !isProjectID(media.binding_id))) ||
      (media.source_node_id !== undefined &&
        (!isProjectID(media.source_node_id) ||
          media.binding_id !== undefined)) ||
      ids.has(media.asset_id)
    )
      throw new Error("主体媒体来源无效或重复");
    ids.add(media.asset_id);
  }
  return local;
}
export function materializeSubject(
  config: CanvasSubjectConfig,
  subject?: UnifiedSubject,
): CanvasSubjectLocal {
  if (config.local) return structuredClone(config.local);
  if (!config.subject_id) return { description: "", references: [] };
  if (
    !subject ||
    subject.id !== config.subject_id ||
    subject.version_id !== config.version_id
  )
    throw new Error("请先读取已固定的主体版本");
  const media = (value: UnifiedSubject["primary"]): CanvasSubjectMedia => ({
    asset_id: value.asset_id,
    asset_revision: value.asset_revision,
    kind: value.kind,
    binding_id: value.binding_id,
  });
  return {
    description: subject.description,
    primary: media(subject.primary),
    references: subject.references.map(media),
    ...(subject.voice
      ? {
          voice: {
            name: subject.voice.name,
            media: media(subject.voice.media),
          },
        }
      : {}),
  };
}
export function localSubjectConfig(
  config: CanvasSubjectConfig,
  local: CanvasSubjectLocal,
): CanvasSubjectConfig {
  requireSubjectLocal(local, !!config.subject_id);
  const bound = subjectLocalMedia(local).some((media) => !!media.binding_id);
  return {
    version: 1,
    ...(bound
      ? { subject_id: config.subject_id, version_id: config.version_id }
      : {}),
    local,
  };
}
export function projectSubjectMedia(
  asset: API.AssetSummary,
  projectID: string,
): CanvasSubjectMedia {
  if (
    asset.project_id !== projectID ||
    !isProjectID(asset.id) ||
    !Number.isSafeInteger(asset.revision) ||
    asset.revision! < 1 ||
    !["image", "video", "audio"].includes(asset.kind ?? "")
  )
    throw new Error("素材来源或版本无效");
  return {
    asset_id: asset.id!,
    asset_revision: asset.revision!,
    kind: asset.kind as CanvasSubjectMedia["kind"],
  };
}
