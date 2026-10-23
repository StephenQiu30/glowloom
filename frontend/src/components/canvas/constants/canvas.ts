import { CanvasNodeType } from "@/components/canvas/types/canvas";
import type { CanvasNodeMetadata } from "@/components/canvas/types/canvas";
import { getNodeSpec as getRegistryNodeSpec } from "@/components/canvas/lib/node-registry";

type CanvasNodeSpec = {
  width: number;
  height: number;
  title: string;
  metadata?: CanvasNodeMetadata;
};

export const NODE_DEFAULT_SIZE = {
  [CanvasNodeType.Image]: {
    width: 320,
    height: 180,
    get title() {
      return "图片";
    },
  },
  [CanvasNodeType.Text]: {
    width: 320,
    height: 320,
    get title() {
      return "文本";
    },
  },
  [CanvasNodeType.Config]: {
    width: 624,
    height: 352,
    get title() {
      return "生成配置";
    },
  },
  [CanvasNodeType.Video]: {
    width: 320,
    height: 180,
    get title() {
      return "视频";
    },
  },
  [CanvasNodeType.Audio]: {
    width: 320,
    height: 180,
    get title() {
      return "音频";
    },
  },
  [CanvasNodeType.Group]: {
    width: 760,
    height: 480,
    get title() {
      return "组";
    },
  },
} satisfies Record<
  CanvasNodeType,
  { width: number; height: number; title: string }
>;

export const NODE_SPECS = {
  [CanvasNodeType.Image]: {
    width: 320,
    height: 180,
    get title() {
      return NODE_DEFAULT_SIZE[CanvasNodeType.Image].title;
    },
    metadata: { content: "", status: "idle" },
  },
  [CanvasNodeType.Text]: {
    width: 320,
    height: 320,
    get title() {
      return NODE_DEFAULT_SIZE[CanvasNodeType.Text].title;
    },
    metadata: { content: "", status: "idle", fontSize: 14 },
  },
  [CanvasNodeType.Config]: {
    width: 624,
    height: 352,
    get title() {
      return NODE_DEFAULT_SIZE[CanvasNodeType.Config].title;
    },
    metadata: { content: "", status: "idle", generationMode: "image" },
  },
  [CanvasNodeType.Video]: {
    width: 320,
    height: 180,
    get title() {
      return NODE_DEFAULT_SIZE[CanvasNodeType.Video].title;
    },
    metadata: { content: "", status: "idle" },
  },
  [CanvasNodeType.Audio]: {
    width: 320,
    height: 180,
    get title() {
      return NODE_DEFAULT_SIZE[CanvasNodeType.Audio].title;
    },
    metadata: { content: "", status: "idle" },
  },
  [CanvasNodeType.Group]: {
    width: 760,
    height: 480,
    get title() {
      return NODE_DEFAULT_SIZE[CanvasNodeType.Group].title;
    },
    metadata: { status: "idle" },
  },
} satisfies Record<CanvasNodeType, CanvasNodeSpec>;

// Return built-in specs directly and resolve plugin types from the registry.
export function getNodeSpec(type: string) {
  if ((Object.values(CanvasNodeType) as string[]).includes(type))
    return NODE_SPECS[type as CanvasNodeType];
  const spec = getRegistryNodeSpec(type);
  return {
    width: spec.width,
    height: spec.height,
    title: spec.title,
    metadata: spec.metadata,
  };
}
