import { getThumbnailDimensions } from "../runtime/lib/media/image-thumbnail";

type FrameEntry = {
  consumers: number;
  abort: AbortController;
  promise: Promise<string>;
  objectURL?: string;
};
// Authorized frames live only while mounted, with bounded decoders and storage.
const frames = new Map<string, FrameEntry>();
const queue: Array<() => void> = [];
let decoding = 0;
function drain() {
  while (decoding < 2 && queue.length) {
    decoding++;
    queue.shift()!();
  }
}

function captureFrame(url: string, startMS: number, signal: AbortSignal) {
  return new Promise<Blob>((resolve, reject) => {
    const video = document.createElement("video");
    let seeking = false;
    let capturing = false;
    let finished = false;
    const cleanup = () => {
      clearTimeout(timeout);
      signal.removeEventListener("abort", cancel);
      video.removeEventListener("loadedmetadata", metadata);
      video.removeEventListener("loadeddata", capture);
      video.removeEventListener("seeked", settled);
      video.removeEventListener("error", failed);
      video.pause();
      video.removeAttribute("src");
      video.load();
    };
    const finish = (blob?: Blob) => {
      if (finished) return;
      finished = true;
      cleanup();
      if (blob && blob.size <= 512 * 1024) resolve(blob);
      else reject(new Error("Video thumbnail unavailable"));
    };
    const cancel = () => finish();
    const failed = () => finish();
    const capture = () => {
      if (finished || capturing || seeking || video.readyState < 2) return;
      if (!video.videoWidth || !video.videoHeight) return;
      capturing = true;
      try {
        const canvas = document.createElement("canvas");
        const size = getThumbnailDimensions(
          video.videoWidth,
          video.videoHeight,
          256,
        );
        canvas.width = size.width;
        canvas.height = size.height;
        const context = canvas.getContext("2d");
        if (!context) return finish();
        context.drawImage(video, 0, 0, size.width, size.height);
        canvas.toBlob((blob) => finish(blob ?? undefined), "image/webp", 0.86);
      } catch {
        // A cross-origin source can play while disallowing canvas extraction.
        finish();
      }
    };
    const metadata = () => {
      const target = Math.min(
        Math.max(0, startMS / 1000),
        Number.isFinite(video.duration)
          ? Math.max(0, video.duration - 0.001)
          : Infinity,
      );
      if (Math.abs(video.currentTime - target) > 0.0001) {
        seeking = true;
        video.currentTime = target;
      } else capture();
    };
    const settled = () => {
      seeking = false;
      capture();
    };
    const timeout = setTimeout(failed, 15000);
    signal.addEventListener("abort", cancel, { once: true });
    video.addEventListener("loadedmetadata", metadata);
    video.addEventListener("loadeddata", capture);
    video.addEventListener("seeked", settled);
    video.addEventListener("error", failed);
    video.crossOrigin = "anonymous";
    video.muted = true;
    video.preload = "auto";
    video.playsInline = true;
    if (signal.aborted) return cancel();
    video.src = url;
    video.load();
  });
}

export function acquireTimelineVideoFrame(url: string, startMS: number) {
  const key = JSON.stringify([url, Math.max(0, Math.round(startMS))]);
  let entry = frames.get(key);
  if (!entry) {
    if (frames.size >= 64)
      return {
        promise: Promise.reject<string>(new Error("Thumbnail capacity")),
        release() {},
      };
    const owned: FrameEntry = {
      consumers: 0,
      abort: new AbortController(),
      promise: Promise.resolve(""),
    };
    owned.promise = (async () => {
      await new Promise<void>((resolve) => {
        queue.push(resolve);
        drain();
      });
      try {
        if (owned.abort.signal.aborted) throw new Error("cancelled");
        const blob = await captureFrame(url, startMS, owned.abort.signal);
        if (owned.abort.signal.aborted) throw new Error("cancelled");
        owned.objectURL = URL.createObjectURL(blob);
        return owned.objectURL;
      } finally {
        decoding--;
        drain();
      }
    })().catch((error: unknown) => {
      if (frames.get(key) === owned) frames.delete(key);
      throw error;
    });
    entry = owned;
    frames.set(key, owned);
  }
  entry.consumers++;
  const owned = entry;
  let released = false;
  return {
    promise: owned.promise,
    release() {
      if (released) return;
      released = true;
      if (--owned.consumers) return;
      owned.abort.abort();
      if (owned.objectURL) URL.revokeObjectURL(owned.objectURL);
      if (frames.get(key) === owned) frames.delete(key);
    },
  };
}
