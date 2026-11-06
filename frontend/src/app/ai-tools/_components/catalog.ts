export type MediaKind = "image" | "video";

type Entry = { id: string; name: string; summary: string };
export type ImportTool = Entry & {
  description: string;
  media: readonly MediaKind[];
  unavailable: string;
};
export type Tool =
  ImportTool | (Entry & { href: string }) | (Entry & { template: string });

const image = ["image"] as const;
const video = ["video"] as const;
const both = ["image", "video"] as const;

export const categories: readonly { name: string; tools: readonly Tool[] }[] = [
  {
    name: "精选",
    tools: [
      {
        id: "smart-edit",
        name: "智能编辑",
        summary: "标记画面，精准修改",
        media: both,
        description: "导入图片或视频，标记需要调整的位置并描述要求。",
        unavailable: "图片和视频的标记编辑服务尚未接入。",
      },
      {
        id: "super-resolution",
        name: "智能超清",
        summary: "增强图片与视频画质",
        media: both,
        description: "增强图片和视频的分辨率与画面细节。",
        unavailable: "AI 画质增强服务尚未接入，暂时无法选择输出档位。",
      },
      {
        id: "video-partial-reshoot",
        name: "视频局部重拍",
        summary: "重新生成指定片段",
        media: video,
        description: "选择需要调整的视频区间，描述要求并重新生成该片段。",
        unavailable: "视频区间重生成服务尚未接入。",
      },
      {
        id: "prompt-reverse",
        name: "提示词反推",
        summary: "解析画面与镜头语言",
        media: both,
        description: "从图片或视频提取可编辑的文字提示词。",
        unavailable: "素材解析及提示词输出服务尚未接入此入口。",
      },
    ],
  },
  {
    name: "视频",
    tools: [
      {
        id: "video-generation",
        name: "视频生成",
        summary: "用文字和参考素材生成视频",
        href: "/?mode=video",
      },
      {
        id: "video-edit",
        name: "视频编辑",
        summary: "标记画面，精准修改",
        media: video,
        description: "导入视频，标记要调整的对象或区域并描述修改要求。",
        unavailable: "视频标记编辑服务尚未接入。",
      },
      {
        id: "digital-human",
        name: "数字人",
        summary: "生成自然同步的口型",
        href: "/?mode=avatar",
      },
      {
        id: "video-extension",
        name: "视频延长",
        summary: "自然衔接后续镜头",
        media: video,
        description: "以已有视频为上下文，生成后续画面与动作。",
        unavailable: "按原片结尾延长视频的服务尚未接入。",
      },
    ],
  },
  {
    name: "图片",
    tools: [
      {
        id: "image-generation",
        name: "图片生成",
        summary: "描述灵感，生成图片",
        href: "/?mode=image",
      },
      {
        id: "image-edit",
        name: "图片编辑",
        summary: "标记画面，精准修改",
        media: image,
        description: "导入图片，标记要调整的对象或区域并描述修改要求。",
        unavailable: "图片标记编辑服务尚未接入此入口。",
      },
      {
        id: "image-inpaint",
        name: "局部重绘",
        summary: "重绘选中区域",
        media: image,
        description: "选择图片中的重绘范围，并描述希望生成的内容。",
        unavailable: "选区重绘服务尚未接入此入口。",
      },
      {
        id: "background-removal",
        name: "图片去背景",
        summary: "去掉背景，保留主体",
        media: image,
        description: "分离图片主体与背景，便于组合和编辑。",
        unavailable: "图片主体分离服务尚未接入。",
      },
      {
        id: "image-eraser",
        name: "消除笔",
        summary: "精准涂抹，自然消除",
        media: image,
        description: "涂抹图片中需要去除的对象，让系统补全局部画面。",
        unavailable: "涂抹区域消除与补全服务尚未接入。",
      },
      {
        id: "image-outpaint",
        name: "扩图",
        summary: "自由拓展画面范围",
        media: image,
        description: "选择扩展边界，生成原图以外的画面。",
        unavailable: "边界扩图服务尚未接入。",
      },
      {
        id: "layer-separation",
        name: "图层分离",
        summary: "拆分图层，分别编辑",
        media: image,
        description: "将一张图片分解为可以分别编辑的图层。",
        unavailable: "可编辑图层分离服务尚未接入。",
      },
    ],
  },
  {
    name: "镜头与动态",
    tools: [
      {
        id: "image-view-control",
        name: "图片多角度",
        summary: "变换拍摄角度",
        media: image,
        description: "导入图片，调整拍摄视角以获取同一主体或场景的新镜头。",
        unavailable: "多角度生成服务尚未接入此入口。",
      },
      {
        id: "image-relight",
        name: "智能打光",
        summary: "调整光线与氛围",
        media: image,
        description: "使用打光预设，或调整灯位、亮度、冷暖和柔硬。",
        unavailable: "基于图片重新布光的服务尚未接入。",
      },
      {
        id: "image-panorama",
        name: "720全景图",
        summary: "生成全景，自由取景",
        media: image,
        description: "将图片场景扩展为全景，拖动预览并截取新视角。",
        unavailable: "从普通图片生成全景的服务尚未接入。",
      },
      {
        id: "3d-director",
        name: "3D导演台",
        summary: "预演构图与动作",
        template: "director-stage-v1",
      },
      {
        id: "motion-depth-capture",
        name: "深度动作捕捉",
        summary: "提取动作与空间深度",
        media: video,
        description: "从参考视频提取动作及场景深度，供后续创作引用。",
        unavailable: "视频动作与深度提取服务尚未接入。",
      },
      {
        id: "3d-render-plugin",
        name: "3D渲染插件",
        summary: "Blender 与 Maya 接入状态",
        href: "/ai-tools/3d-render-plugin",
      },
    ],
  },
];
