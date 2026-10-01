// Adapted from BeefTV 1ae25027 director-viewport.tsx recordCanvas; MIT.
// See docs/licenses/beeftv.txt. Recording ownership and upload are Lanverse contracts.
import {
  resolveDirectorPixelCrop,
  type DirectorAspectRatio,
} from "./aspect-ratio";

const maximumBytes = 500 * 1024 * 1024;
type RecordingInput = {
  duration: number;
  fps: 24 | 25 | 30;
  aspectRatio: DirectorAspectRatio;
  /** Produces an actual committed clay frame for the requested scene time. */
  renderFrame: (seconds: number) => Promise<HTMLCanvasElement>;
  signal: AbortSignal;
};

/** Records actual browser frames; an incomplete or cancelled file is never returned. */
export async function recordDirectorCanvas(
  input: RecordingInput,
): Promise<Blob> {
  const { duration, fps, aspectRatio, renderFrame, signal } = input;
  if (!Number.isFinite(duration) || duration < 0.25 || duration > 60)
    throw new Error("白膜录制时长须为 0.25～60 秒。");
  if (![24, 25, 30].includes(fps)) throw new Error("白膜录制帧率无效。");
  if (signal.aborted) throw new Error("录制已取消。");
  const mimeType =
    typeof MediaRecorder === "undefined"
      ? undefined
      : ["video/webm;codecs=vp9", "video/webm;codecs=vp8"].find((type) =>
          MediaRecorder.isTypeSupported(type),
        );
  if (!mimeType) throw new Error("当前浏览器不支持 VP8／VP9 白膜录制。");
  const source = await renderFrame(0);
  if (signal.aborted) throw new Error("录制已取消。");
  if (source.width < 2 || source.height < 2)
    throw new Error("3D 视口尚未就绪。");
  const sourceWidth = source.width;
  const sourceHeight = source.height;
  const crop = resolveDirectorPixelCrop(sourceWidth, sourceHeight, aspectRatio);
  const output = document.createElement("canvas");
  output.width = crop.width;
  output.height = crop.height;
  const context = output.getContext("2d");
  if (!context || typeof output.captureStream !== "function")
    throw new Error("当前浏览器不支持画布视频录制。");
  const draw = (canvas: HTMLCanvasElement) => {
    if (canvas.width !== sourceWidth || canvas.height !== sourceHeight)
      throw new Error("录制期间视口尺寸已改变，请重试。");
    context.drawImage(
      canvas,
      crop.x,
      crop.y,
      crop.width,
      crop.height,
      0,
      0,
      output.width,
      output.height,
    );
  };
  draw(source);
  const stream = output.captureStream(fps);
  let recorder: MediaRecorder | undefined;
  let frame = 0;
  let stopTimer: ReturnType<typeof setTimeout> | undefined;
  let cleanupEvents = () => {};
  try {
    recorder = new MediaRecorder(stream, { mimeType });
    const activeRecorder = recorder;
    const blob = await new Promise<Blob>((resolve, reject) => {
      const chunks: Blob[] = [];
      let bytes = 0;
      let finished = false;
      const finish = (failure?: Error) => {
        if (finished) return;
        finished = true;
        cleanupEvents();
        if (activeRecorder.state !== "inactive") activeRecorder.stop();
        if (failure) reject(failure);
        else if (!bytes) reject(new Error("录制未产生可解码的视频。"));
        else resolve(new Blob(chunks, { type: activeRecorder.mimeType }));
      };
      const data = (event: BlobEvent) => {
        if (finished || !event.data.size) return;
        bytes += event.data.size;
        if (bytes > maximumBytes) {
          finish(new Error("白膜录制超过 500 MiB 上限。"));
          return;
        }
        chunks.push(event.data);
      };
      const failed = () => finish(new Error("白膜视频录制失败。"));
      const stopped = () => finish();
      const aborted = () => finish(new Error("录制已取消。"));
      const contextLost = () =>
        finish(new Error("录制期间 3D 上下文已失效，请重新打开导演台。"));
      const hidden = () => {
        if (document.hidden)
          finish(new Error("录制页面已隐藏，请返回导演台后重试。"));
      };
      cleanupEvents = () => {
        activeRecorder.removeEventListener("dataavailable", data);
        activeRecorder.removeEventListener("error", failed);
        activeRecorder.removeEventListener("stop", stopped);
        signal.removeEventListener("abort", aborted);
        source.removeEventListener("webglcontextlost", contextLost);
        document.removeEventListener("visibilitychange", hidden);
      };
      activeRecorder.addEventListener("dataavailable", data);
      activeRecorder.addEventListener("error", failed);
      activeRecorder.addEventListener("stop", stopped);
      signal.addEventListener("abort", aborted, { once: true });
      source.addEventListener("webglcontextlost", contextLost);
      document.addEventListener("visibilitychange", hidden);
      if (signal.aborted) {
        aborted();
        return;
      }
      activeRecorder.start(250);
      const start = performance.now();
      const render = async () => {
        if (finished) return;
        try {
          const seconds = Math.min(
            duration,
            (performance.now() - start) / 1000,
          );
          const next = await renderFrame(seconds);
          if (finished) return;
          draw(next);
          frame = requestAnimationFrame(() => void render());
        } catch (failure) {
          finish(
            failure instanceof Error ? failure : new Error("白膜渲染失败。"),
          );
        }
      };
      frame = requestAnimationFrame(() => void render());
      stopTimer = setTimeout(() => {
        try {
          if (activeRecorder.state !== "inactive") activeRecorder.stop();
        } catch {
          failed();
        }
      }, duration * 1000);
    });
    const actual = await probeDirectorRecording(blob, signal);
    validateDirectorRecordingDuration(actual.duration, duration, fps);
    if (actual.width < 2 || actual.height < 2)
      throw new Error("白膜视频画面异常，请重试。");
    return blob;
  } finally {
    cleanupEvents();
    if (stopTimer !== undefined) clearTimeout(stopTimer);
    if (frame) cancelAnimationFrame(frame);
    if (recorder && recorder.state !== "inactive") recorder.stop();
    stream.getTracks().forEach((track) => track.stop());
  }
}

/** Checks actual decode duration, allowing only a bounded scheduling/frame error. */
export function validateDirectorRecordingDuration(
  actual: number,
  expected: number,
  fps: number,
) {
  const tolerance = Math.max(0.2, 2 / fps);
  if (
    !Number.isFinite(actual) ||
    actual < 0.25 ||
    actual > 60 ||
    !Number.isFinite(expected) ||
    expected < 0.25 ||
    expected > 60 ||
    ![24, 25, 30].includes(fps) ||
    Math.abs(actual - expected) > tolerance
  )
    throw new Error("白膜视频时长异常，录制可能不完整，请重试。");
}

/** Reads an actual decoded browser frame and resolves durationless live WebM. */
export async function probeDirectorRecording(blob: Blob, signal: AbortSignal) {
  if (signal.aborted) throw new Error("录制已取消。");
  const url = URL.createObjectURL(blob);
  const video = document.createElement("video");
  video.muted = true;
  video.preload = "auto";
  let cleanup = () => {};
  try {
    await new Promise<void>((resolve, reject) => {
      let decoded = false;
      let seekingEnd = false;
      const timer = setTimeout(
        () => finish(new Error("白膜视频解码超时。")),
        10_000,
      );
      const finish = (failure?: Error) => {
        cleanup();
        if (failure) reject(failure);
        else resolve();
      };
      const ready = () => {
        if (video.readyState >= 2) decoded = true;
        if (!decoded) return;
        if (Number.isFinite(video.duration) && video.duration > 0) finish();
        else if (!seekingEnd && video.duration === Infinity) {
          seekingEnd = true;
          video.currentTime = 1e6;
        }
      };
      const failed = () => finish(new Error("白膜视频无法解码。"));
      const aborted = () => finish(new Error("录制已取消。"));
      cleanup = () => {
        clearTimeout(timer);
        video.removeEventListener("loadeddata", ready);
        video.removeEventListener("durationchange", ready);
        video.removeEventListener("seeked", ready);
        video.removeEventListener("error", failed);
        signal.removeEventListener("abort", aborted);
      };
      video.addEventListener("loadeddata", ready);
      video.addEventListener("durationchange", ready);
      video.addEventListener("seeked", ready);
      video.addEventListener("error", failed);
      signal.addEventListener("abort", aborted, { once: true });
      video.src = url;
      video.load();
    });
    return {
      duration: video.duration,
      width: video.videoWidth,
      height: video.videoHeight,
    };
  } finally {
    cleanup();
    video.pause();
    video.removeAttribute("src");
    video.load();
    URL.revokeObjectURL(url);
  }
}
