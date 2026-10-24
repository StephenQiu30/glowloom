import type { JSONContent } from "@tiptap/core";
import type { CanvasEditorState } from "./lib/canvas-document-adapter";
import type { CanvasNodeData, CanvasConnection } from "./types/canvas";
import {
  textToDocument,
  documentText,
  parseRichTextDocument,
} from "./runtime/components/editor/lib/rich-text-document";
import {
  emptyPromptDraft,
  validatePromptDraft,
  type PromptDraft,
  type PromptInline,
} from "@/components/generation/prompt-document";
import { isProjectID } from "@/components/projects/project";
import {
  revision,
  type Operation,
  validateHistoricalSnapshot,
} from "./service-document";

export function serviceGraph(
  dto: API.DocumentDTO,
  canvasID: string,
  projectID: string,
): CanvasEditorState {
  if (
    dto.id !== canvasID ||
    dto.project_id !== projectID ||
    !dto.permissions?.read ||
    dto.schema_version !== 1 ||
    dto.operation_version !== 1 ||
    !Array.isArray(dto.nodes) ||
    !Array.isArray(dto.edges)
  )
    throw new Error("画布身份或协议不匹配，已停止编辑");
  revision(dto.revision);
  validateHistoricalSnapshot(dto);
  const nodes = dto.nodes.map((node): CanvasNodeData => {
    const composer = node.config?.composer as PromptDraft | undefined;
    if (composer) validatePromptDraft(composer);
    const nodeType =
      node.node_type === "image_generation"
        ? "image"
        : node.node_type === "video_generation"
          ? "video"
          : node.node_type === "generation"
            ? "config"
            : node.node_type!;
    const editor = node.config?.editor;
    const stored = node.config?.rich_text ?? editor?.document;
    const richText = stored
      ? parseRichTextDocument(stored)
      : composer
        ? { schemaVersion: 1 as const, content: composer.document }
        : {
            schemaVersion: 1 as const,
            content: textToDocument(node.config?.text ?? ""),
          };
    return {
      id: node.id!,
      type: nodeType,
      title: node.title ?? "",
      position: { x: node.x!, y: node.y! },
      width: node.width!,
      height: node.height!,
      serviceData: structuredClone(node),
      documentData: {
        id: node.id!,
        type: nodeType,
        nodeTypeVersion: 1,
        title: node.title ?? "",
        position: { x: node.x!, y: node.y! },
        width: node.width!,
        height: node.height!,
        zIndex: node.z_index ?? 0,
        parentId: node.parent_id,
        collapsed: node.config?.collapsed,
        ...(node.node_type === "text" || editor?.document
          ? { textDocument: richText }
          : {}),
        parameters: (editor?.parameters as
          CanvasContract.CanvasNodeParametersDTO | undefined) ?? {
          mode: node.node_action === "generate" ? "GENERATE" : "MANUAL",
        },
        ...(editor
          ? {
              fileReferences: (editor.files ??
                []) as CanvasContract.CanvasFileReferenceDTO[],
            }
          : node.media_ref?.asset_id
            ? {
                fileReferences: [
                  {
                    referenceKey: node.media_ref.asset_id,
                    fileId: node.media_ref.asset_id,
                    version: node.content_version ?? "1",
                    purpose: "CONTENT",
                  },
                ],
              }
            : {}),
      },
      metadata: {
        freeResize: true,
        prompt: node.config?.draft?.prompt ?? node.config?.generation?.prompt,
        groupId: node.parent_id,
        content:
          node.node_type === "text" ? documentText(richText.content) : "",
        ...(node.node_type === "text" ? { richText } : {}),
        ...(editor?.document ? { richText } : {}),
        ...editor?.metadata,
        status: "idle",
      },
    };
  });
  const connections: CanvasConnection[] = dto.edges.map((edge) => ({
    id: edge.id!,
    fromNodeId: edge.source_node_id!,
    toNodeId: edge.target_node_id!,
    serviceData: structuredClone(edge),
    ...(edge.binding && "schemaVersion" in edge.binding
      ? {
          documentData: {
            fromPortId: edge.binding.fromPortId,
            toPortId: edge.binding.toPortId,
            purpose: edge.binding.purpose,
            order: edge.binding.order,
            binding: edge.binding.binding,
            id: edge.id!,
            fromNodeId: edge.source_node_id!,
            toNodeId: edge.target_node_id!,
          } as CanvasContract.CanvasDocumentConnectionDTO,
        }
      : {}),
  }));
  return {
    nodes,
    connections,
    backgroundMode: "dots",
    showImageInfo: false,
    viewport: { x: 0, y: 0, k: 1 },
  };
}

/** Formatting is stored with its complete, unchanged binding registry in one CAS field. */
export function richTextConfig(node: CanvasNodeData) {
  const original = node.serviceData?.config;
  const richText = parseRichTextDocument(
    node.metadata?.richText ?? {
      schemaVersion: 1,
      content: textToDocument(node.metadata?.content ?? ""),
    },
  );
  const composer: PromptDraft = original?.composer
    ? structuredClone(original.composer as PromptDraft)
    : emptyPromptDraft();
  const paragraphs: PromptDraft["document"]["content"] = [];
  let inline: PromptInline[] = [];
  function flush() {
    if (inline.length) {
      paragraphs.push({ type: "paragraph", content: inline });
      inline = [];
    }
  }
  function visit(part: JSONContent) {
    if (part.type === "text" && part.text)
      inline.push({ type: "text", text: part.text });
    else if (part.type === "hardBreak") inline.push({ type: "hardBreak" });
    else if (part.type === "reference")
      inline.push({
        type: "reference",
        attrs: { referenceId: String(part.attrs?.referenceId) },
      });
    else if (part.type === "mention")
      inline.push({
        type: "text",
        text: String(part.attrs?.token ?? part.attrs?.label ?? ""),
      });
    else {
      for (const child of part.content ?? []) visit(child);
      if (
        ["paragraph", "heading", "codeBlock", "horizontalRule"].includes(
          part.type ?? "",
        )
      )
        flush();
    }
  }
  visit(richText.content);
  flush();
  composer.document = {
    type: "doc",
    content: paragraphs.length ? paragraphs : [{ type: "paragraph" }],
  };
  validatePromptDraft(composer);
  return { format: "tiptap", format_version: 1, composer, rich_text: richText };
}
function contentConfig(config: API.NodeConfig = {}) {
  return Object.fromEntries(
    Object.entries(config).filter(([key]) =>
      ["text", "composer", "format", "format_version", "rich_text"].includes(
        key,
      ),
    ),
  );
}
const metadataFields = new Set([
  "fontSize",
  "prompt",
  "composerContent",
  "generationMode",
  "generationType",
  "model",
  "reasoningEffort",
  "size",
  "quality",
  "background",
  "count",
  "textCount",
  "seconds",
  "vquality",
  "generateAudio",
  "watermark",
  "videoMode",
  "audioVoice",
  "audioFormat",
  "audioSpeed",
  "audioInstructions",
  "freeResize",
  "interactive",
]);
function editorConfig(node: CanvasNodeData): API.EditorConfig {
  const parameters = node.documentData?.parameters;
  return {
    schemaVersion: 1,
    parameters: parameters
      ? {
          ...parameters,
          modelSelection: parameters.modelSelection
            ? {
                ...parameters.modelSelection,
                connectionId: String(parameters.modelSelection.connectionId),
              }
            : undefined,
        }
      : {
          mode: ["image", "video"].includes(node.type) ? "GENERATE" : "MANUAL",
        },
    metadata: Object.fromEntries(
      Object.entries(node.metadata ?? {}).filter(
        ([key, value]) => metadataFields.has(key) && value !== undefined,
      ),
    ),
    ...(node.type !== "text" && node.metadata?.richText
      ? { document: node.metadata.richText }
      : {}),
    files: (node.documentData?.fileReferences ?? []).map(
      ({ referenceKey, fileId, version, purpose }) => ({
        referenceKey,
        fileId,
        version,
        purpose,
      }),
    ),
  };
}
function serviceEdge(
  edge: CanvasConnection,
): NonNullable<API.DocumentDTO["edges"]>[number] {
  const data = edge.documentData;
  return {
    ...(edge.serviceData ?? {}),
    id: edge.id,
    edge_type: "annotation",
    source_node_id: edge.fromNodeId,
    target_node_id: edge.toNodeId,
    ...(data
      ? {
          binding: {
            schemaVersion: 1,
            fromPortId: data.fromPortId,
            toPortId: data.toPortId,
            purpose: data.purpose,
            order: data.order,
            binding: data.binding,
          },
        }
      : {}),
  };
}
export function serviceGraphOperations(
  before: CanvasEditorState,
  after: CanvasEditorState,
): Operation[] {
  const result: Operation[] = [];
  const old = new Map(before.nodes.map((n) => [n.id, n]));
  const next = new Map(after.nodes.map((n) => [n.id, n]));
  for (const e of before.connections)
    if (
      !after.connections.some((n) => n.id === e.id) &&
      next.has(e.fromNodeId) &&
      next.has(e.toNodeId)
    )
      result.push({ type: "disconnect", node_id: e.id });
  for (const node of [...after.nodes].sort(
    (a, b) => Number(b.type === "group") - Number(a.type === "group"),
  )) {
    if (!isProjectID(node.id)) throw new Error("节点身份无效");
    const previous = old.get(node.id);
    const localContent = node.metadata?.content;
    const primary = node.documentData?.fileReferences?.find(
      (ref) => ref.purpose === "CONTENT",
    )?.fileId;
    const oldPrimary = previous?.documentData?.fileReferences?.find(
      (ref) => ref.purpose === "CONTENT",
    )?.fileId;
    if (
      ["image", "video", "audio"].includes(node.type) &&
      localContent &&
      (localContent.startsWith("data:") || localContent.startsWith("blob:")) &&
      localContent !== previous?.metadata?.content &&
      (!primary || primary === oldPrimary)
    )
      throw new Error("本地素材需要上传后保存，修改已保留");
    const set = (path: string[], value: unknown) =>
      result.push({ type: "set", node_id: node.id, path, value });
    if (!previous) {
      const source = node.serviceData;
      const asset = node.documentData?.fileReferences?.find(
        (ref) => ref.purpose === "CONTENT",
      );
      if (
        !["text", "group", "image", "video", "audio", "config"].includes(
          node.type,
        )
      )
        throw new Error("此节点的保存接口尚未接入，修改已保留");
      result.push({
        type: "create_node",
        node: {
          ...(source ?? {}),
          id: node.id,
          node_type: node.type,
          node_action: "resource",
          title: node.title,
          x: node.position.x,
          y: node.position.y,
          width: node.width,
          height: node.height,
          parent_id: node.metadata?.groupId ?? null,
          z_index: node.documentData?.zIndex ?? (node.type === "group" ? 0 : 1),
          config:
            node.type === "group"
              ? { collapsed: node.documentData?.collapsed ?? false }
              : {
                  ...(node.type === "text" ? richTextConfig(node) : {}),
                  editor: editorConfig(node),
                },
          ...(asset ? { media_ref: { asset_id: asset.fileId } } : {}),
        },
      });
      continue;
    }
    if (node.type !== previous.type) throw new Error("不能改变已有节点类型");
    if (
      node.type !== "group" &&
      JSON.stringify(editorConfig(node)) !==
        JSON.stringify(editorConfig(previous))
    ) {
      result.push({
        type: "set",
        node_id: node.id,
        path: ["config", "editor"],
        expected_exists: !!previous.serviceData?.config?.editor,
        ...(previous.serviceData?.config?.editor
          ? { expected: previous.serviceData.config.editor }
          : {}),
        value: editorConfig(node),
      });
    }
    if (node.documentData?.zIndex !== previous.documentData?.zIndex)
      set(["z_index"], node.documentData?.zIndex ?? 0);
    if (node.title !== previous.title) set(["title"], node.title);
    if (
      node.position.x !== previous.position.x ||
      node.position.y !== previous.position.y
    )
      set(["position"], node.position);
    if (node.width !== previous.width || node.height !== previous.height)
      set(["size"], { width: node.width, height: node.height });
    if (node.metadata?.groupId !== previous.metadata?.groupId)
      set(["parent_id"], node.metadata?.groupId ?? null);
    if (
      node.type === "group" &&
      node.documentData?.collapsed !== previous.documentData?.collapsed
    )
      set(["config", "collapsed"], node.documentData?.collapsed ?? false);
    if (
      node.type === "text" &&
      JSON.stringify(node.metadata?.richText ?? node.metadata?.content) !==
        JSON.stringify(
          previous.metadata?.richText ?? previous.metadata?.content,
        )
    ) {
      result.push({
        type: "set",
        node_id: node.id,
        path: ["config", "text_content"],
        expected_exists: true,
        expected: contentConfig(previous.serviceData?.config),
        value: richTextConfig(node),
      });
    }
    const asset = node.documentData?.fileReferences?.find(
      (ref) => ref.purpose === "CONTENT",
    )?.fileId;
    const oldAsset = previous.documentData?.fileReferences?.find(
      (ref) => ref.purpose === "CONTENT",
    )?.fileId;
    if (asset !== oldAsset)
      result.push({
        type: "replace_media",
        node_id: node.id,
        expected_media_ref: oldAsset ? { asset_id: oldAsset } : { empty: true },
        new_media_ref: asset ? { asset_id: asset } : { empty: true },
      });
  }
  for (const edge of after.connections) {
    const previous = before.connections.find((item) => item.id === edge.id);
    if (!previous) result.push({ type: "connect", edge: serviceEdge(edge) });
    else if (
      JSON.stringify(edge.documentData) !==
        JSON.stringify(previous.documentData) ||
      edge.fromNodeId !== previous.fromNodeId ||
      edge.toNodeId !== previous.toNodeId
    )
      result.push({
        type: "set_edge",
        node_id: edge.id,
        expected: previous.serviceData ?? serviceEdge(previous),
        edge: serviceEdge(edge),
      });
  }
  for (const node of [...before.nodes].sort(
    (a, b) => Number(a.type === "group") - Number(b.type === "group"),
  ))
    if (!next.has(node.id))
      result.push({ type: "delete_node", node_id: node.id });
  return result;
}

/** Local editor intent only; Go revalidates the resulting Yjs projection. */
export function serviceDocumentFromEditor(
  base: API.DocumentDTO,
  state: CanvasEditorState,
): API.DocumentDTO {
  const next = structuredClone(base);
  const operations = serviceGraphOperations(
    serviceGraph(base, base.id!, base.project_id!),
    state,
  );
  for (const operation of operations) {
    const id = operation.node_id as string;
    const node = next.nodes!.find((item) => item.id === id);
    switch (operation.type) {
      case "create_node":
        next.nodes!.push(operation.node as API.NodeDTO);
        break;
      case "delete_node":
        next.nodes = next.nodes!.filter((item) => item.id !== id);
        next.edges = next.edges!.filter(
          (edge) => edge.source_node_id !== id && edge.target_node_id !== id,
        );
        break;
      case "disconnect":
        next.edges = next.edges!.filter((edge) => edge.id !== id);
        break;
      case "connect":
        next.edges!.push(operation.edge as API.Edge);
        break;
      case "set_edge":
        next.edges = next.edges!.map((edge) =>
          edge.id === id ? (operation.edge as API.Edge) : edge,
        );
        break;
      case "replace_media":
        if (!node) throw new Error("节点已删除");
        if ((operation.new_media_ref as { empty?: boolean }).empty)
          delete node.media_ref;
        else node.media_ref = operation.new_media_ref as API.MediaRef;
        break;
      case "set": {
        if (!node) throw new Error("节点已删除");
        const path = operation.path as string[];
        if (path[0] === "position" || path[0] === "size")
          Object.assign(node, operation.value);
        else if (path[0] === "config" && path[1] === "text_content") {
          const config = (node.config ??= {});
          for (const key of [
            "text",
            "composer",
            "format",
            "format_version",
            "rich_text",
          ] as const)
            delete config[key];
          Object.assign(config, operation.value);
        } else {
          const target = (
            path[0] === "config" ? (node.config ??= {}) : node
          ) as Record<string, unknown>;
          target[path.at(-1)!] = structuredClone(operation.value);
        }
        break;
      }
      default:
        throw new Error("不支持的画布变更");
    }
  }
  return next;
}
