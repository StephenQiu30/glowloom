import { getMediaPreview } from "@/api/media";
import type { Task } from "@/components/generation/contracts";
import { canonical } from "@/components/generation/draft-save";
import type { CanvasYjsLiveSession } from "../yjs-live-session";
import { textToDocument } from "../runtime/components/editor/lib/rich-text-document";
import type { CanvasNodeData } from "../types/canvas";
import { replaceCanvasNodeAsset } from "./canvas-asset-replacement";

export function canvasOperationTarget(
  client: CanvasYjsLiveSession,
  nodeID: string,
  includeRevision = true,
) {
  const node = client
    .getSnapshot()
    .document?.nodes.find((item) => item.id === nodeID);
  return canonical({
    epoch: client.getDocumentEpoch(),
    files: node?.documentData?.fileReferences ?? [],
    content: node?.metadata?.richText ?? node?.metadata?.content,
    input:
      client.getSnapshot().inputDrafts?.[nodeID]?.draft ??
      node?.serviceData?.config?.draft?.composer,
    ...(includeRevision
      ? { inputRevision: node?.serviceData?.input_revision }
      : {}),
  });
}

export async function applyCanvasOperationResult(
  client: CanvasYjsLiveSession,
  projectID: string,
  canvasID: string,
  nodeID: string,
  task: Task,
  expectedTarget?: string,
) {
  const source = task.source;
  if (
    task.project_id !== projectID ||
    task.status !== "completed" ||
    source?.canvas_id !== canvasID ||
    source.node_id !== nodeID
  )
    throw new Error("生成结果来源不匹配");
  const target = () => {
    const view = client.getSnapshot();
    const node = view.document?.nodes.find((item) => item.id === nodeID);
    if (!node || !view.writable || view.migration || !client.getDocumentEpoch())
      throw new Error("结果目标已删除或不可编辑");
    return node;
  };
  const before = target();
  if (
    expectedTarget &&
    canvasOperationTarget(client, nodeID) !== expectedTarget
  )
    throw new Error("节点输入或素材已变化，结果已保留在历史记录");
  const identity = () => canvasOperationTarget(client, nodeID);
  const original = identity();
  const write = async (node: CanvasNodeData) => {
    const graph = client.getSnapshot().document;
    if (!graph || identity() !== original)
      throw new Error("节点内容已变化，结果已保留在历史记录");
    const next = {
      ...graph,
      nodes: graph.nodes.map((item) => (item.id === nodeID ? node : item)),
    };
    if (!client.canAccept(next))
      throw new Error("画布当前不可编辑，结果已保留在历史记录");
    client.change(next, true);
    const expected = canvasOperationTarget(client, nodeID, false);
    await client.waitForSaved(() => {
      target();
      if (canvasOperationTarget(client, nodeID, false) !== expected)
        throw new Error("节点结果再次变化，请查看历史记录");
    });
  };
  if (before.type === "text") {
    const output = task.outputs?.find(
      (item) =>
        item.kind === "json" &&
        item.moderation_status === "passed" &&
        item.text,
    );
    if (!output?.text) throw new Error("生成结果尚不可用");
    if (before.metadata?.content === output.text) return;
    if (!expectedTarget)
      throw new Error("节点输入已变化，结果已保留在历史记录");
    await write({
      ...before,
      metadata: {
        ...before.metadata,
        content: output.text,
        richText: { schemaVersion: 1, content: textToDocument(output.text) },
        texts: undefined,
        primaryTextId: undefined,
      },
    });
    return;
  }
  const ordered = (task.outputs ?? [])
    .filter(
      (item) =>
        item.kind === before.type &&
        item.moderation_status === "passed" &&
        item.media_asset_id,
    )
    .sort((a, b) => (a.sequence ?? 0) - (b.sequence ?? 0));
  const outputs = before.type === "image" ? ordered : ordered.slice(0, 1);
  if (!outputs.length) throw new Error("生成结果尚不可用");
  const references = (before.documentData?.fileReferences ?? []).filter(
    (ref) => ref.purpose === "CONTENT",
  );
  if (
    references.length >= outputs.length &&
    outputs.every(
      (output, index) => references[index].fileId === output.media_asset_id,
    )
  )
    return;
  if (!expectedTarget && references.length)
    throw new Error("节点素材已变化，结果已保留在历史记录");
  const previews = await Promise.all(
    outputs.map(async (output) => {
      const preview = await getMediaPreview({
        pid: projectID,
        asset_id: output.media_asset_id!,
      });
      const asset = preview.asset;
      if (
        !asset ||
        asset.id !== output.media_asset_id ||
        asset.project_id !== projectID ||
        asset.kind !== before.type ||
        !Number.isSafeInteger(asset.revision) ||
        asset.revision! < 1 ||
        !preview.url
      )
        throw new Error("生成素材不可用");
      return { asset, url: preview.url };
    }),
  );
  const preview = previews[0],
    asset = preview.asset;
  const graph = client.getSnapshot().document;
  if (!graph || identity() !== original)
    throw new Error("节点素材已变化，结果已保留在历史记录");
  const current = target();
  const kind = current.type as "image" | "video" | "audio";
  let node = replaceCanvasNodeAsset(
    current,
    kind === "image"
      ? {
          kind,
          title: current.title,
          fileId: asset.id,
          fileVersion: String(asset.revision),
          dataUrl: preview.url,
          width: asset.width,
          height: asset.height,
        }
      : {
          kind,
          title: current.title,
          fileId: asset.id,
          fileVersion: String(asset.revision),
          url: preview.url,
          width: asset.width,
          height: asset.height,
        },
  );
  if (
    kind !== "audio" &&
    asset.width &&
    asset.height &&
    asset.width > 0 &&
    asset.height > 0
  ) {
    const base = Math.min(current.width, current.height);
    const scale = Math.min(
      base / Math.min(asset.width, asset.height),
      640 / Math.max(asset.width, asset.height),
    );
    const width = asset.width * scale,
      height = asset.height * scale;
    node = {
      ...node,
      width,
      height,
      position: {
        x: current.position.x + (current.width - width) / 2,
        y: current.position.y + (current.height - height) / 2,
      },
    };
  }
  if (kind === "image" && node.documentData) {
    const files = previews.map(({ asset }, index) => ({
      referenceKey: `result/${task.id}/${outputs[index].id ?? asset.id!}`,
      purpose: "CONTENT",
      fileId: asset.id!,
      version: String(asset.revision),
    }));
    node.documentData = {
      ...node.documentData,
      fileReferences: [
        ...files,
        ...references.filter(
          (ref) =>
            ref.referenceKey.startsWith("result/") &&
            !files.some((file) => file.fileId === ref.fileId),
        ),
      ].slice(0, 64),
    };
    if (files.length > 1)
      node.metadata = {
        ...node.metadata,
        primaryImageId: files[0].referenceKey,
        images: previews.map(({ asset, url }, i) => ({
          id: files[i].referenceKey,
          content: url,
          status: "success",
          naturalWidth: asset.width ?? node.width,
          naturalHeight: asset.height ?? node.height,
          bytes: 0,
          mimeType: "image/*",
        })),
      };
  }
  await write(node);
}
