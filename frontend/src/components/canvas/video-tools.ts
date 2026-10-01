import { CanvasNodeType, type CanvasNodeData } from "./model";
import {
  createTimeline,
  timelineSchema,
  type TimelineProject,
} from "./timeline";
import {
  normalizeVideoCropForEncoding,
  type VideoCropRect,
} from "./video-crop-geometry";
import type { MediaAsset } from "./queries";
export type VideoToolSelection = {
  startMs: number;
  endMs: number;
  crop: VideoCropRect | null;
  aspectRatio: TimelineProject["aspectRatio"];
};
export function createVideoClipTimeline(
  source: CanvasNodeData,
  asset: MediaAsset,
  projectId: string,
  selection: VideoToolSelection,
): TimelineProject {
  const duration = asset.duration_ms;
  if (
    source.type !== CanvasNodeType.Video ||
    source.assetId !== asset.id ||
    asset.project_id !== projectId ||
    asset.kind !== "video" ||
    !duration ||
    !Number.isInteger(duration) ||
    ![selection.startMs, selection.endMs].every(Number.isInteger) ||
    selection.startMs < 0 ||
    selection.endMs > duration ||
    selection.endMs - selection.startMs < 100
  )
    throw new Error("视频身份、检测时长或剪辑范围无效。");
  const value = createTimeline(),
    track = value.tracks.find((item) => item.kind === "video")!;
  const crop = selection.crop
    ? normalizeVideoCropForEncoding(selection.crop, {
        width: asset.width ?? 0,
        height: asset.height ?? 0,
      })
    : null;
  value.tracks = [track];
  value.aspectRatio = selection.aspectRatio;
  value.clips = [
    {
      id: crypto.randomUUID(),
      trackId: track.id,
      kind: "video",
      nodeId: source.id,
      assetId: asset.id,
      title: source.title,
      startMs: 0,
      durationMs: selection.endMs - selection.startMs,
      sourceStartMs: selection.startMs,
      sourceDurationMs: duration,
      volume: 1,
      fadeInMs: 0,
      fadeOutMs: 0,
      text: "",
      ...(crop ? { crop } : {}),
    },
  ];
  return timelineSchema.parse(value);
}
function waitForVideo(
  video: HTMLVideoElement,
  event: "seeked" | "loadeddata",
  signal: AbortSignal,
) {
  signal.throwIfAborted();
  return new Promise<void>((resolve, reject) => {
    const finish = (failure?: Error) => {
      clearTimeout(timeout);
      video.removeEventListener(event, done);
      video.removeEventListener("error", error);
      signal.removeEventListener("abort", abort);
      if (failure) reject(failure);
      else resolve();
    };
    const done = () => finish(),
      error = () => finish(new Error("视频帧无法解码。")),
      abort = () => finish(new DOMException("已取消截帧", "AbortError"));
    const timeout = setTimeout(
      () => finish(new Error("视频帧读取超时，请播放到目标帧后重试。")),
      10000,
    );
    video.addEventListener(event, done, { once: true });
    video.addEventListener("error", error, { once: true });
    signal.addEventListener("abort", abort, { once: true });
  });
}
/** Browser capture of an actually decoded frame; no video-processing task is invented. */
export async function captureVideoFrame(
  video: HTMLVideoElement,
  selection: VideoCropRect | null,
  title: string,
  signal: AbortSignal,
) {
  video.pause();
  signal.throwIfAborted();
  if (video.seeking) await waitForVideo(video, "seeked", signal);
  if (video.readyState < 2) await waitForVideo(video, "loadeddata", signal);
  const source = { width: video.videoWidth, height: video.videoHeight };
  const crop = selection
    ? normalizeVideoCropForEncoding(selection, source)
    : { x: 0, y: 0, ...source };
  if (!crop.width || !crop.height || crop.width * crop.height > 16_000_000)
    throw new Error("截帧支持最多1600万像素，请缩小裁切区域。");
  const canvas = document.createElement("canvas");
  canvas.width = crop.width;
  canvas.height = crop.height;
  try {
    const context = canvas.getContext("2d");
    if (!context) throw new Error("浏览器无法截取视频帧。");
    context.drawImage(
      video,
      crop.x,
      crop.y,
      crop.width,
      crop.height,
      0,
      0,
      crop.width,
      crop.height,
    );
    const blob = await new Promise<Blob>((resolve, reject) => {
      try {
        canvas.toBlob(
          (value) =>
            value ? resolve(value) : reject(new Error("视频截帧编码失败。")),
          "image/png",
        );
      } catch {
        reject(new Error("视频预览未允许像素读取，请检查素材授权后重试。"));
      }
    });
    signal.throwIfAborted();
    if (blob.size > 20 * 1024 * 1024)
      throw new Error("截帧文件超过20 MiB，请缩小裁切区域。");
    const name = (
      title
        .split(/[\\/]/)
        .at(-1)
        ?.replace(/\.[^.]+$/, "")
        .replace(/[<>:"|?*\u0000-\u001f]/g, "") || "视频"
    ).slice(0, 100);
    return new File(
      [blob],
      `${name}-截帧-${Math.round(video.currentTime * 1000)}ms.png`,
      { type: "image/png" },
    );
  } finally {
    canvas.width = 0;
    canvas.height = 0;
  }
}
