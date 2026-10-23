import type { AiConfig } from "@/components/canvas/runtime/store/use-config-store";
import type { ReferenceImage } from "@/components/canvas/runtime/types/image";
import type {
  ReferenceAudio,
  ReferenceVideo,
} from "@/components/canvas/runtime/types/media";
import {
  uploadMediaFile,
  type UploadedFile,
} from "@/components/canvas/runtime/lib/media/file-storage";
import { unavailableGeneration } from "./unavailable";

export type VideoGenerationResult = {
  blob?: Blob;
  url?: string;
  mimeType?: string;
};
export type VideoGenerationTask = {
  id: string;
  provider: "agent";
  model: string;
};
export type VideoGenerationTaskState =
  | { status: "pending" }
  | { status: "completed"; result: VideoGenerationResult }
  | { status: "failed"; error: string };
type RequestOptions = { signal?: AbortSignal };
type VideoMediaOptions = RequestOptions & {
  videos?: ReferenceVideo[];
  audios?: ReferenceAudio[];
};

export function requestVideoGeneration(
  _config: AiConfig,
  _prompt: string,
  _references: ReferenceImage[] = [],
  options?: VideoMediaOptions,
) {
  void _references;
  return unavailableGeneration<VideoGenerationResult>(options?.signal);
}
export function createVideoGenerationTask(
  _config: AiConfig,
  _prompt: string,
  _references: ReferenceImage[] = [],
  options?: VideoMediaOptions,
) {
  void _references;
  return unavailableGeneration<VideoGenerationTask>(options?.signal);
}
export function waitForVideoGenerationTask(
  _config: AiConfig,
  _task: VideoGenerationTask,
  options?: RequestOptions,
) {
  return unavailableGeneration<VideoGenerationResult>(options?.signal);
}
export function pollVideoGenerationTask(
  _config: AiConfig,
  _task: VideoGenerationTask,
  options?: RequestOptions,
) {
  return unavailableGeneration<VideoGenerationTaskState>(options?.signal);
}
export function isVideoTaskFailed(error: unknown) {
  return error instanceof Error && error.name === "VideoTaskFailed";
}
export async function storeGeneratedVideo(
  result: VideoGenerationResult,
): Promise<UploadedFile> {
  if (!result.blob) throw new Error("视频内容不可用");
  return uploadMediaFile(result.blob, "video");
}
