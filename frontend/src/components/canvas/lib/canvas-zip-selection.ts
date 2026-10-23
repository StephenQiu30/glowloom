import { CanvasNodeType, type CanvasNodeData } from "../types/canvas";

export function canvasZipSelection(
  selected: CanvasNodeData[],
  allNodes = selected,
): Omit<CanvasContract.AssetZipExportRequest, "commandId"> {
  const expanded = new Map<string, CanvasNodeData>();
  const visit = (node: CanvasNodeData) => {
    if (expanded.has(node.id)) return;
    expanded.set(node.id, node);
    if (node.type === CanvasNodeType.Group)
      allNodes
        .filter((item) => item.metadata?.groupId === node.id)
        .forEach(visit);
  };
  selected.forEach(visit);
  const files = new Map<string, CanvasContract.FileReference>();
  const texts: CanvasContract.TextEntry[] = [];
  for (const node of expanded.values()) {
    if (node.type === CanvasNodeType.Text) {
      texts.push({
        fileName: `${node.title || "文本"}.txt`,
        content: node.metadata?.content ?? "",
      });
    } else if (
      [
        CanvasNodeType.Image,
        CanvasNodeType.Video,
        CanvasNodeType.Audio,
      ].includes(node.type as CanvasNodeType)
    ) {
      const references =
        node.metadata?.generationOutput?.files ??
        node.documentData?.fileReferences?.filter(
          (item) => item.purpose === "CONTENT",
        ) ??
        [];
      if (!references.length)
        throw new Error(`「${node.title}」尚未取得已发布的固定文件版本`);
      for (const reference of references)
        files.set(`${reference.fileId}:${reference.version}`, {
          fileId: reference.fileId,
          version: reference.version,
          ...("publicAssetId" in reference && reference.publicAssetId
            ? {
                publicAssetId: reference.publicAssetId,
                publicRevision: reference.publicRevision,
              }
            : {}),
        });
    }
  }
  if (!files.size && !texts.length)
    throw new Error("请选择文字、图片、视频或音频素材");
  return {
    ...(files.size ? { files: [...files.values()] } : {}),
    ...(texts.length ? { texts } : {}),
  };
}
