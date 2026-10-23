import { uploadMediaAsset } from "@/api/media";
import { isProjectID } from "@/components/projects/project";
import { validateUpload } from "@/components/media/library";
import type { CanvasNodeData } from "../types";

export type DerivedReceipt = { key: string; assetID?: string };

/** Retry retains each upload identity and confirmed receipt, including after a graph rejection. */
export async function publishDerivedNodes(
  nodes: CanvasNodeData[],
  projectID: string,
  receipts: Map<string, DerivedReceipt>,
  signal: AbortSignal,
) {
  const result: CanvasNodeData[] = [];
  for (const node of nodes) {
    signal.throwIfAborted();
    let receipt = receipts.get(node.id);
    if (!receipt) {
      receipt = { key: crypto.randomUUID() };
      receipts.set(node.id, receipt);
    }
    if (!receipt.assetID) {
      const source = node.metadata?.content;
      if (!source?.startsWith("data:image/png;base64,"))
        throw new Error("派生图片格式无效");
      const binary = atob(source.slice("data:image/png;base64,".length));
      const file = new File(
        [Uint8Array.from(binary, (c) => c.charCodeAt(0))],
        `${node.title}.png`,
        { type: "image/png" },
      );
      const error = validateUpload(file);
      if (error) throw new Error(error);
      const response = await uploadMediaAsset(
        { pid: projectID },
        { local_review_confirmed: true },
        file,
        { signal, headers: { "Idempotency-Key": receipt.key } },
      );
      if (
        !isProjectID(response.asset?.id) ||
        response.asset?.project_id !== projectID ||
        response.asset?.kind !== "image"
      )
        throw new Error("派生素材保存回执无效，请重试原请求");
      receipt.assetID = response.asset.id;
    }
    signal.throwIfAborted();
    result.push({
      ...node,
      metadata: { ...node.metadata, assetId: receipt.assetID, content: "" },
    });
  }
  return result;
}
