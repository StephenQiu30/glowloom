import { createPreviewStorage } from "./preview-storage";
import { canvasId as nanoid } from "@/components/canvas/lib/canvas-id";

export type UploadedFile = {
  url: string;
  storageKey: string;
  bytes: number;
  mimeType: string;
  width?: number;
  height?: number;
  durationMs?: number;
};

const store = createPreviewStorage();
const objectUrls = new Map<string, string>();

export async function uploadMediaFile(
  input: string | Blob,
  prefix = "file",
): Promise<UploadedFile> {
  const blob =
    // eslint-disable-next-line no-restricted-syntax -- Decode media bytes; service uploads use the generated media client.
    typeof input === "string" ? await (await fetch(input)).blob() : input;
  const storageKey = `${prefix}:${nanoid()}`;
  await store.setItem(storageKey, blob);
  const url = URL.createObjectURL(blob);
  objectUrls.set(storageKey, url);
  const meta = blob.type.startsWith("video/")
    ? await readVideoMeta(url)
    : blob.type.startsWith("audio/")
      ? await readAudioMeta(url)
      : {};
  return {
    url,
    storageKey,
    bytes: blob.size,
    mimeType: blob.type || "application/octet-stream",
    ...meta,
  };
}

export async function resolveMediaUrl(storageKey?: string, fallback = "") {
  if (!storageKey) return fallback;
  const cached = objectUrls.get(storageKey);
  if (cached) return cached;
  const blob = await store.getItem<Blob>(storageKey);
  if (!blob) return fallback;
  const url = URL.createObjectURL(blob);
  objectUrls.set(storageKey, url);
  return url;
}

export async function getMediaBlob(storageKey: string) {
  return store.getItem<Blob>(storageKey);
}

export async function setMediaBlob(storageKey: string, blob: Blob) {
  await store.setItem(storageKey, blob);
  const url = URL.createObjectURL(blob);
  objectUrls.set(storageKey, url);
  return url;
}

export async function deleteStoredMedia(keys: Iterable<string>) {
  await Promise.all(
    Array.from(new Set(keys)).map(async (key) => {
      const url = objectUrls.get(key);
      if (url) URL.revokeObjectURL(url);
      objectUrls.delete(key);
      await store.removeItem(key);
    }),
  );
}

function readVideoMeta(url: string) {
  return new Promise<{ width: number; height: number; durationMs?: number }>(
    (resolve) => {
      const video = document.createElement("video");
      const done = () =>
        resolve({
          width: video.videoWidth || 1280,
          height: video.videoHeight || 720,
          durationMs: Number.isFinite(video.duration)
            ? Math.round(video.duration * 1000)
            : undefined,
        });
      video.onloadedmetadata = done;
      video.onerror = done;
      video.src = url;
    },
  );
}

function readAudioMeta(url: string) {
  return new Promise<{ durationMs?: number }>((resolve) => {
    const audio = document.createElement("audio");
    const done = () =>
      resolve({
        durationMs: Number.isFinite(audio.duration)
          ? Math.round(audio.duration * 1000)
          : undefined,
      });
    audio.onloadedmetadata = done;
    audio.onerror = done;
    audio.src = url;
  });
}
