import { CanvasNodeType, type CanvasNodeData } from "../types/canvas";

export function canvasAssetRequest(
  node: CanvasNodeData,
  title: string,
  commandId: string,
): CanvasContract.AssetSaveRequest {
  const name = title.trim();
  if (!name || name.length > 100) throw new Error("请填写100字以内的素材名称");
  const body = { commandId, title: name, kind: node.type, source: "Canvas" };
  if (node.type === CanvasNodeType.Text) {
    const content = node.metadata?.content ?? "";
    if (!content.trim()) throw new Error("没有可保存的文本");
    if (new TextEncoder().encode(content).length > 65536)
      throw new Error("文字内容最多64 KiB");
    return { ...body, content };
  }
  if (!["image", "video", "audio"].includes(node.type))
    throw new Error("该节点不支持保存为素材");
  const generated = node.metadata?.generationOutput;
  const references = generated
    ? generated.files
    : (node.documentData?.fileReferences?.filter(
        (ref) => ref.purpose === "CONTENT",
      ) ?? []);
  const selected = node.metadata?.primaryImageId;
  const reference =
    generated || !selected
      ? references[0]
      : (references.find((ref) => ref.referenceKey === selected) ??
        (references.length === 1 ? references[0] : undefined));
  if (!reference?.fileId || !reference.version)
    throw new Error(
      "该素材尚未取得已发布的固定文件版本，请先完成上传或生成归档",
    );
  if ("publicAssetId" in reference && reference.publicAssetId)
    throw new Error(
      "公共素材保留共享引用，暂不支持另存为本人原件；可继续在画布中参考使用",
    );
  return { ...body, fileId: reference.fileId, fileVersion: reference.version };
}
