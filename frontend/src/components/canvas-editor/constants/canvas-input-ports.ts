/** 设计09的图协议；模型兼容性由执行预检裁定，不能缩窄草稿引用。 */
const contentTypes = ["text", "image", "video", "audio"] as const;

export const canvasInputPorts = [
  {
    id: "prompt",
    label: "参考文本",
    purpose: "PROMPT",
    source: "text",
    targets: contentTypes,
    multiple: true,
    allOutputs: false,
  },
  {
    id: "settings",
    label: "共享参数",
    purpose: "SETTINGS",
    source: "config",
    targets: contentTypes,
    multiple: false,
    allOutputs: false,
  },
  {
    id: "reference_images",
    label: "参考图片",
    purpose: "REFERENCE_IMAGE",
    source: "image",
    targets: contentTypes,
    multiple: true,
    allOutputs: true,
  },
  {
    id: "reference_videos",
    label: "参考视频",
    purpose: "REFERENCE_VIDEO",
    source: "video",
    targets: contentTypes,
    multiple: true,
    allOutputs: true,
  },
  {
    id: "reference_audio",
    label: "参考音频",
    purpose: "REFERENCE_AUDIO",
    source: "audio",
    targets: contentTypes,
    multiple: true,
    allOutputs: true,
  },
  {
    id: "first_frame",
    label: "首帧",
    purpose: "FIRST_FRAME",
    source: "image",
    targets: ["video"],
    multiple: false,
    allOutputs: false,
  },
  {
    id: "last_frame",
    label: "尾帧",
    purpose: "LAST_FRAME",
    source: "image",
    targets: ["video"],
    multiple: false,
    allOutputs: false,
  },
  {
    id: "reference_subjects",
    label: "参考主体",
    purpose: "REFERENCE_SUBJECT",
    source: "subject",
    targets: ["image", "video", "audio"],
    multiple: true,
    allOutputs: false,
  },
  {
    id: "camera",
    label: "相机",
    purpose: "CINEMA_CAMERA",
    source: "cinema_studio",
    targets: ["image"],
    multiple: false,
    allOutputs: false,
  },
  {
    id: "palette",
    label: "配色",
    purpose: "CINEMA_PALETTE",
    source: "cinema_studio",
    targets: ["image", "video"],
    multiple: false,
    allOutputs: false,
  },
  {
    id: "style",
    label: "风格",
    purpose: "CINEMA_STYLE",
    source: "cinema_studio",
    targets: ["image", "video"],
    multiple: false,
    allOutputs: false,
  },
  {
    id: "motion",
    label: "运镜",
    purpose: "CINEMA_MOTION",
    source: "cinema_studio",
    targets: ["video"],
    multiple: false,
    allOutputs: false,
  },
] as const;

export function canvasPortsFor(source: string, target: string) {
  return canvasInputPorts.filter(
    (port) =>
      port.source === source && port.targets.some((type) => type === target),
  );
}

export function canvasInputPort(id: string) {
  return canvasInputPorts.find((port) => port.id === id);
}

export function canvasConnectionHandles(type: string) {
  return {
    input:
      ["timeline", "subject"].includes(type) ||
      canvasInputPorts.some((port) =>
        port.targets.some((target) => target === type),
      ),
    output: canvasInputPorts.some((port) => port.source === type),
  };
}
