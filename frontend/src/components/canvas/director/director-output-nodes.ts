import type { CanvasYjsLiveSession } from "../yjs-live-session";
import { createCanvasNode } from "../lib/canvas-node-factory";
import { directorFingerprint } from "./director-model";
/** Reuses one UUID per result for retry; only confirmed project media can become canvas resources. */
export function addDirectorOutputs(
  client: CanvasYjsLiveSession,
  sourceID: string,
  projectID: string,
  expected: API.DirectorConfig,
  epoch: string,
  assets: { asset: API.AssetSummary; nodeID: string }[],
) {
  const view = client.getSnapshot(),
    graph = view.document,
    source = graph?.nodes.find((node) => node.id === sourceID);
  if (
    !graph ||
    !source ||
    !view.writable ||
    view.migration ||
    client.getDocumentEpoch() !== epoch ||
    directorFingerprint(source.serviceData?.config?.director) !==
      directorFingerprint(expected)
  )
    throw new Error("场景或画布版本已变化，请重新添加");
  const nodes = [...graph.nodes];
  for (const [index, { asset, nodeID }] of assets.entries()) {
    if (
      asset.project_id !== projectID ||
      !asset.id ||
      !asset.revision ||
      !["image", "video"].includes(asset.kind ?? "")
    )
      throw new Error("素材身份无效");
    const existing = nodes.find((node) => node.id === nodeID);
    if (existing) {
      if (
        existing.documentData?.fileReferences?.find(
          (ref) => ref.purpose === "CONTENT",
        )?.fileId !== asset.id
      )
        throw new Error("画布节点身份冲突");
      continue;
    }
    const node = createCanvasNode(
      asset.kind as "image" | "video",
      {
        x: source.position.x + source.width + 220 + index * 360,
        y: source.position.y + 140,
      },
      {
        status: "success",
        naturalWidth: asset.width,
        naturalHeight: asset.height,
      },
      nodes,
    );
    node.id = nodeID;
    node.title = asset.file_name ?? "镜头素材";
    node.documentData = {
      id: nodeID,
      type: node.type,
      nodeTypeVersion: 1,
      title: node.title,
      position: node.position,
      width: node.width,
      height: node.height,
      zIndex: 1,
      parameters: { mode: "MANUAL" },
      fileReferences: [
        {
          referenceKey: asset.id,
          fileId: asset.id,
          version: String(asset.revision),
          purpose: "CONTENT",
        },
      ],
    };
    nodes.push(node);
  }
  client.change({ ...graph, nodes }, true);
  const saved = client.getSnapshot().document;
  if (
    assets.some(
      ({ nodeID }) => !saved?.nodes.some((node) => node.id === nodeID),
    )
  )
    throw new Error("素材尚未添加到画布，可重试");
}
