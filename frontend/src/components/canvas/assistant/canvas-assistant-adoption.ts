import { getOperation } from "@/api/operations";
import { getMediaPreview } from "@/api/media";
import { requireTask } from "@/components/generation/contracts";
import { canonical } from "@/components/generation/draft-save";
import type { CanvasYjsLiveSession } from "../yjs-live-session";
import { createCanvasNode } from "../lib/canvas-node-factory";
import { replaceCanvasNodeAsset } from "../lib/canvas-asset-replacement";
import { textToDocument } from "../runtime/components/editor/lib/rich-text-document";
import type { CanvasNodeData } from "../types/canvas";

type Attempt = {
  epoch: string;
  nodeID?: string;
  content?: string;
  flight?: Promise<void>;
};
/** Keep this owner for the canvas lifetime: unknown-save retries reuse the same node. */
export function createCanvasAssistantAdoption(
  client: CanvasYjsLiveSession,
  projectID: string,
) {
  const attempts = new Map<string, Attempt>();
  function current(epoch: string) {
    const graph = client.getSnapshot().document;
    if (
      !graph ||
      client.getDocumentEpoch() !== epoch ||
      !client.canAccept(graph)
    )
      throw new Error("画布已变化或当前不可编辑，请重新确认采用");
    return graph;
  }
  const fingerprint = (node: CanvasNodeData) =>
    canonical({
      type: node.type,
      text: node.metadata?.richText ?? node.metadata?.content ?? "",
      files: node.documentData?.fileReferences ?? [],
    });
  async function run(
    key: string,
    load: () => Promise<(node: CanvasNodeData) => CanvasNodeData>,
    kind: "text" | "image" | "video" | "audio",
  ) {
    const epoch = client.getDocumentEpoch();
    if (!epoch) throw new Error("画布尚未就绪");
    current(epoch);
    const identity = `${epoch}/${key}`;
    let attempt = attempts.get(identity);
    if (attempt?.flight) return attempt.flight;
    if (!attempt) {
      attempt = { epoch };
      attempts.set(identity, attempt);
    }
    const owned = attempt;
    const check = () => {
      const graph = current(epoch);
      const node = graph.nodes.find((n) => n.id === owned.nodeID);
      if (!node || fingerprint(node) !== owned.content)
        throw new Error("采用的节点已变化，请核对当前画布");
    };
    const flight = (async () => {
      if (!owned.nodeID) {
        const update = await load();
        const graph = current(epoch);
        const right = graph.nodes.reduce(
          (x, n) => Math.max(x, n.position.x + n.width),
          0,
        );
        const node = update(
          createCanvasNode(
            kind,
            { x: right + 240, y: 240 },
            undefined,
            graph.nodes,
          ),
        );
        const next = { ...graph, nodes: [...graph.nodes, node] };
        if (!client.canAccept(next))
          throw new Error("画布无法加入此结果，任务结果仍保留");
        owned.nodeID = node.id;
        owned.content = fingerprint(node);
        client.change(next, true);
      }
      check();
      await client.waitForSaved(check);
      check();
    })();
    owned.flight = flight;
    try {
      await flight;
    } finally {
      delete owned.flight;
    }
  }
  return {
    adoptText: (operationID: string, outputID: string) =>
      run(
        `text/${operationID}/${outputID}`,
        async () => {
          const task = requireTask(
            await getOperation({ id: operationID, project_id: projectID }),
            projectID,
          );
          const output = task.outputs?.find((o) => o.id === outputID);
          if (
            task.id !== operationID ||
            task.status !== "completed" ||
            output?.kind !== "json" ||
            output.moderation_status !== "passed" ||
            !output.text
          )
            throw new Error("文字结果尚未就绪");
          const text = output.text;
          return (node) => ({
            ...node,
            metadata: {
              ...node.metadata,
              content: text,
              richText: { schemaVersion: 1, content: textToDocument(text) },
            },
          });
        },
        "text",
      ),
    adoptMedia: (assetID: string, kind: string) => {
      if (kind !== "image" && kind !== "video" && kind !== "audio")
        return Promise.reject(new Error("不支持此结果类型"));
      return run(
        `media/${kind}/${assetID}`,
        async () => {
          const preview = await getMediaPreview({
            pid: projectID,
            asset_id: assetID,
          });
          const asset = preview.asset;
          if (
            asset?.id !== assetID ||
            asset.project_id !== projectID ||
            asset.kind !== kind ||
            !Number.isSafeInteger(asset.revision) ||
            asset.revision! < 1 ||
            !preview.url
          )
            throw new Error("结果素材不可用");
          return (node) =>
            replaceCanvasNodeAsset(
              node,
              kind === "image"
                ? {
                    kind,
                    title: node.title,
                    fileId: assetID,
                    fileVersion: String(asset.revision),
                    dataUrl: preview.url!,
                    width: asset.width,
                    height: asset.height,
                  }
                : {
                    kind,
                    title: node.title,
                    fileId: assetID,
                    fileVersion: String(asset.revision),
                    url: preview.url!,
                  },
            );
        },
        kind,
      );
    },
  };
}
