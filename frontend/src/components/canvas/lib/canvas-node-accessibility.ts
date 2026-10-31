import type { CanvasNodeData, CanvasNodeStatus } from "../types/canvas";
import {
  canvasNodeName,
  canvasNodeTypeLabel,
} from "../search/canvas-node-search-state";
function resources(node: CanvasNodeData) {
  const counts = { ready: 0, processing: 0, failed: 0, unverified: 0 };
  const count = (status?: CanvasNodeStatus) => {
    if (status === "success") counts.ready++;
    else if (status === "loading") counts.processing++;
    else if (status === "error") counts.failed++;
    else counts.unverified++;
  };
  if (node.metadata?.images?.length) {
    for (const image of node.metadata.images) count(image.status);
  } else {
    const ids = new Set<string>();
    if (node.serviceData?.media_ref?.asset_id)
      ids.add(node.serviceData.media_ref.asset_id);
    for (const file of node.documentData?.fileReferences ?? [])
      if (file.purpose === "CONTENT" && file.fileId) ids.add(file.fileId);
    const subject = node.serviceData?.config?.subject;
    if (subject?.local) {
      for (const media of [
        subject.local.primary,
        ...(subject.local.references ?? []),
        subject.local.voice?.media,
      ])
        if (media?.asset_id) ids.add(media.asset_id);
    }
    if (subject?.subject_id && !subject.local)
      return "已固定主体版本；媒体状态未核验。";
    if (ids.size) {
      for (let index = 0; index < ids.size; index++)
        count(node.metadata?.status);
    } else if (
      [
        "image",
        "video",
        "audio",
        "image_generation",
        "video_generation",
      ].includes(node.type)
    ) {
      if (
        node.metadata?.status === "loading" ||
        node.metadata?.status === "error"
      )
        count(node.metadata.status);
      else if (node.metadata?.content) count();
    }
  }
  if (!Object.values(counts).some(Boolean)) return "无媒体资源。";
  return `媒体资源：${counts.ready} 项就绪，${counts.processing} 项处理中，${counts.failed} 项失败${counts.unverified ? `，${counts.unverified} 项状态未核验` : ""}。`;
}
export function canvasNodeAccessibility(
  node: CanvasNodeData,
  state: {
    selected: boolean;
    readOnly: boolean;
    editing?: boolean;
    reference?: "target" | "disabled" | "available";
  },
) {
  const description = [
    state.selected ? "已选中。" : "未选中。",
    state.readOnly ? "只读。" : "可编辑。",
    resources(node),
  ];
  if (state.editing) description.push("正在编辑。");
  else if (node.type === "text" && !state.readOnly && !state.reference)
    description.push("双击或按 Enter、空格编辑文本。");
  if (state.reference)
    description.push(
      state.reference === "target"
        ? "当前引用选择目标。"
        : state.reference === "available"
          ? "可选择为引用。"
          : "不可选择为引用。",
    );
  return {
    label: `${canvasNodeTypeLabel(node.type, node)}节点：${canvasNodeName(node)}`,
    description: description.join(" "),
  };
}
