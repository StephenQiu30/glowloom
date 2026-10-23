import {
  Type,
  Group,
  Image as ImageIcon,
  AudioLines,
  Settings2,
  Video,
} from "lucide-react";

import { NODE_SPECS } from "@/components/canvas/constants/canvas";
import { registerNodeDefinitions } from "@/components/canvas/lib/node-registry";
import {
  CanvasNodeType,
  type CanvasNodeData,
} from "@/components/canvas/types/canvas";
import type {
  CanvasNodeDefinition,
  CanvasNodeResource,
} from "@/components/canvas/types/canvas-plugin";

// Extensible metadata for built-in nodes, reusing NODE_SPECS for size and initial metadata.
// Rendering remains in canvas-node's internal renderer, so no Content component is provided.
function builtinResource(node: CanvasNodeData): CanvasNodeResource | null {
  if (node.type === CanvasNodeType.Image && node.metadata?.content)
    return { kind: "image", url: node.metadata.content };
  if (node.type === CanvasNodeType.Video && node.metadata?.content)
    return { kind: "video", url: node.metadata.content };
  if (node.type === CanvasNodeType.Audio && node.metadata?.content)
    return { kind: "audio", url: node.metadata.content };
  if (
    node.type === CanvasNodeType.Text &&
    (node.metadata?.content || node.metadata?.prompt)
  )
    return {
      kind: "text",
      text: node.metadata.content || node.metadata.prompt,
    };
  return null;
}

const iconClass = "size-5";

const BUILTIN_DEFINITIONS: CanvasNodeDefinition[] = [
  {
    type: CanvasNodeType.Text,
    title: "文本",
    icon: <Type className={iconClass} />,
    minimapColor: undefined,
    resource: builtinResource,
  },
  {
    type: CanvasNodeType.Image,
    title: "图片",
    icon: <ImageIcon className={iconClass} />,
    minimapColor: "var(--node-image)",
    keepAspectRatio: (node: CanvasNodeData) => !node.metadata?.freeResize,
    resource: builtinResource,
  },
  {
    type: CanvasNodeType.Video,
    title: "视频",
    icon: <Video className={iconClass} />,
    minimapColor: "var(--node-video)",
    keepAspectRatio: () => true,
    resource: builtinResource,
  },
  {
    type: CanvasNodeType.Audio,
    title: "音频",
    icon: <AudioLines className={iconClass} />,
    minimapColor: "var(--node-audio)",
    resource: builtinResource,
  },
  {
    type: CanvasNodeType.Config,
    title: "生成配置",
    showInCreateMenu: false,
    icon: <Settings2 className={iconClass} />,
    minimapColor: "var(--node-config)",
    hasSourceHandle: false,
  },
  {
    type: CanvasNodeType.Group,
    title: "组",
    icon: <Group className={iconClass} />,
    minimapColor: "var(--node-group)",
  },
].map((def) => {
  const spec = NODE_SPECS[def.type];
  return {
    ...def,
    title: spec.title,
    defaultSize: { width: spec.width, height: spec.height },
    defaultMetadata: spec.metadata,
  };
});

let registered = false;
export function registerBuiltinNodes() {
  if (registered) return;
  registered = true;
  registerNodeDefinitions(BUILTIN_DEFINITIONS, "builtin");
}
