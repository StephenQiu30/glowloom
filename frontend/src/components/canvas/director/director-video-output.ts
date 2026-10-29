import type { Director } from "./director-model";
export type DirectorVideoOptions = {
  camera: string;
  duration: number;
  fps: number;
  longEdge: number;
  aspect: string;
  quality: "standard" | "high";
  clay: boolean;
};
export type DirectorVideoExporter = (
  options: DirectorVideoOptions,
  signal: AbortSignal,
  progress: (value: number) => void,
) => Promise<Blob>;
export function directorVideoDimensions(options: DirectorVideoOptions) {
  const [x, y] = options.aspect.split(":").map(Number);
  if (
    ![1280, 1920].includes(options.longEdge) ||
    ![24, 25, 30, 50, 60].includes(options.fps) ||
    !Number.isFinite(options.duration) ||
    options.duration < 0.1 ||
    options.duration > 3600 ||
    !x ||
    !y ||
    !Number.isFinite(x / y) ||
    x / y < 0.25 ||
    x / y > 4
  )
    throw new Error("视频导出参数无效");
  const ratio = x / y;
  return {
    width:
      Math.round(
        (ratio >= 1 ? options.longEdge : options.longEdge * ratio) / 2,
      ) * 2,
    height:
      Math.round(
        (ratio >= 1 ? options.longEdge / ratio : options.longEdge) / 2,
      ) * 2,
  };
}
export function directorOutputCamera(
  scene: Director,
  camera: string,
  time: number,
) {
  if (camera !== "sequence") {
    if (!scene.cameras.some((item) => item.id === camera))
      throw new Error("导出机位不存在");
    return camera;
  }
  let end = 0;
  for (const shot of scene.shots) {
    end += shot.duration;
    if (time < end) return shot.camera_id;
  }
  return scene.shots.at(-1)!.camera_id;
}
/** Offline frame timestamps, codec capability detection and decoded readback are part of output acceptance. */
export async function encodeDirectorVideo(
  scene: Director,
  options: DirectorVideoOptions,
  signal: AbortSignal,
  progress: (value: number) => void,
  draw: (
    time: number,
    camera: string,
    width: number,
    height: number,
  ) => HTMLCanvasElement,
): Promise<Blob> {
  const { width, height } = directorVideoDimensions(options);
  const {
    Output,
    WebMOutputFormat,
    BufferTarget,
    CanvasSource,
    canEncodeVideo,
    Input,
    BlobSource,
    WEBM,
    VideoSampleSink,
  } = await import("mediabunny");
  signal.throwIfAborted();
  const codec = (await canEncodeVideo("vp9", {
    width,
    height,
    frameRate: options.fps,
  }))
    ? "vp9"
    : (await canEncodeVideo("vp8", { width, height, frameRate: options.fps }))
      ? "vp8"
      : null;
  if (!codec)
    throw new Error("当前浏览器不支持视频编码，请使用支持 WebCodecs 的浏览器");
  const output = new Output({
    format: new WebMOutputFormat(),
    target: new BufferTarget(),
  });
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const context = canvas.getContext("2d");
  if (!context) throw new Error("无法建立输出画面");
  let bytes = 0;
  const source = new CanvasSource(canvas, {
    codec,
    bitrate: options.quality === "high" ? 12_000_000 : 5_000_000,
    onEncodedPacket: (packet) => {
      bytes += packet.data.byteLength;
      if (bytes > 256 * 1024 * 1024)
        throw new Error("成片超过 256 MB，请缩短范围后分段导出");
    },
  });
  output.addVideoTrack(source, { frameRate: options.fps });
  try {
    await output.start();
    const count = Math.ceil(options.duration * options.fps);
    for (let frame = 0; frame < count; frame++) {
      signal.throwIfAborted();
      const time = frame / options.fps;
      context.drawImage(
        draw(
          time,
          directorOutputCamera(scene, options.camera, time),
          width,
          height,
        ),
        0,
        0,
      );
      await source.add(
        time,
        Math.min(1 / options.fps, options.duration - time),
      );
      progress(((frame + 1) / count) * 0.95);
      if (frame % 4 === 0)
        await new Promise<void>((resolve) => setTimeout(resolve, 0));
    }
    source.close();
    await output.finalize();
    signal.throwIfAborted();
    if (!output.target.buffer?.byteLength)
      throw new Error("视频编码未产生文件");
    const blob = new Blob([output.target.buffer], { type: "video/webm" });
    const input = new Input({ formats: [WEBM], source: new BlobSource(blob) });
    try {
      const track = await input.getPrimaryVideoTrack(),
        duration = await input.computeDuration();
      if (
        !track ||
        (await track.getDisplayWidth()) !== width ||
        (await track.getDisplayHeight()) !== height ||
        Math.abs(duration - options.duration) > 1 / options.fps + 0.01 ||
        !(await track.canDecode())
      )
        throw new Error("视频尺寸、时长或解码校验失败");
      const sink = new VideoSampleSink(track);
      const first = await sink.getSample(0),
        last = await sink.getSample(
          Math.max(0, options.duration - 1 / options.fps),
        );
      if (!first || !last) {
        first?.close();
        last?.close();
        throw new Error("视频首尾帧解码失败");
      }
      first.close();
      last.close();
    } finally {
      input.dispose();
    }
    signal.throwIfAborted();
    progress(1);
    return blob;
  } catch (error) {
    await output.cancel().catch(() => {});
    throw error;
  }
}
