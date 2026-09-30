import type { Edge, Node } from "@xyflow/react";

export type PocCardData = {
  kind: "image" | "video" | "text";
  title: string;
  playing: boolean;
  src?: string;
  localMedia?: boolean;
};

export type PocMedia = { kind: "image" | "video"; url: string };

export function mediaSelectionError(
  files: Pick<File, "type" | "size">[],
): string | null {
  if (files.length > 40) return "最多选择 40 个素材。";
  if (
    files.some(
      (file) =>
        ![
          "image/png",
          "image/jpeg",
          "image/webp",
          "image/avif",
          "video/mp4",
          "video/webm",
        ].includes(file.type),
    )
  )
    return "请选择 PNG、JPEG、WebP、AVIF 图片或 MP4、WebM 视频。";
  if (files.some((file) => file.size > 100 * 1024 * 1024))
    return "单个素材不能超过 100 MB。";
  if (files.reduce((bytes, file) => bytes + file.size, 0) > 300 * 1024 * 1024)
    return "素材总大小不能超过 300 MB。";
  return null;
}

export type PocNode = Node<PocCardData, "pocCard">;

/** Fixed-size, deterministic scenes keep every measured run comparable. */
export function makePocGraph(
  nodeCount: number,
  edgeCount: number,
  media: PocMedia[] = [],
): { nodes: PocNode[]; edges: Edge[] } {
  if (
    !Number.isInteger(nodeCount) ||
    nodeCount < 2 ||
    !Number.isInteger(edgeCount) ||
    edgeCount < 0
  ) {
    throw new RangeError("invalid PoC graph size");
  }

  const imageCount = Math.round(nodeCount * 0.6);
  const videoCount = Math.round(nodeCount * 0.08);
  const columns = nodeCount <= 500 ? 25 : 50;
  const videoIndices = new Set<number>();
  for (let i = 0; i < videoCount; i += 1) {
    videoIndices.add(Math.floor((i * nodeCount) / videoCount));
  }

  let images = 0;
  let playing = 0;
  let videos = 0;
  const imageSources = media.filter((item) => item.kind === "image");
  const videoSources = media.filter((item) => item.kind === "video");
  const nodes: PocNode[] = Array.from({ length: nodeCount }, (_, index) => {
    let kind: PocCardData["kind"] = "text";
    if (videoIndices.has(index)) {
      kind = "video";
      videos += 1;
    } else if (images < imageCount) {
      kind = "image";
      images += 1;
    }
    const shouldPlay = kind === "video" && playing < 3;
    if (shouldPlay) playing += 1;
    return {
      id: `node-${index}`,
      type: "pocCard",
      position: {
        x: (index % columns) * 330,
        y: Math.floor(index / columns) * 230,
      },
      width: 280,
      height: 170,
      data: {
        kind,
        title: `${kind === "image" ? "场景画面" : kind === "video" ? "镜头预览" : "创作笔记"} ${String(index + 1).padStart(3, "0")}`,
        playing: shouldPlay,
        src:
          kind === "image"
            ? imageSources[(images - 1) % imageSources.length]?.url
            : kind === "video"
              ? videoSources[(videos - 1) % videoSources.length]?.url
              : undefined,
        localMedia:
          kind === "image"
            ? imageSources.length > 0
            : kind === "video"
              ? videoSources.length > 0
              : false,
      },
    };
  });

  const edges: Edge[] = Array.from({ length: edgeCount }, (_, index) => {
    const source = index % nodeCount;
    const target = (source + 1 + Math.floor(index / nodeCount)) % nodeCount;
    return {
      id: `edge-${index}`,
      source: `node-${source}`,
      target: `node-${target}`,
      type: "smoothstep",
      style: { stroke: "#9ba9b9", strokeWidth: 1.5 },
    };
  });

  return { nodes, edges };
}
