import type { AiConfig } from "@/components/canvas/runtime/store/use-config-store";
import { uploadMediaFile } from "@/components/canvas/runtime/lib/media/file-storage";
import { unavailableGeneration } from "./unavailable";

export function requestAudioGeneration(
  _config: AiConfig,
  _prompt: string,
  options?: { signal?: AbortSignal },
) {
  return unavailableGeneration<Blob>(options?.signal);
}
export function storeGeneratedAudio(blob: Blob, _format = "mp3") {
  void _format;
  return uploadMediaFile(blob, "audio");
}
