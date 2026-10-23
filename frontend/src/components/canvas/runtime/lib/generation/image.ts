import type { AiConfig } from "@/components/canvas/runtime/store/use-config-store";
import type { ReferenceImage } from "@/components/canvas/runtime/types/image";
import { unavailableGeneration } from "./unavailable";

export type AiTextMessage = {
  role: "system" | "user" | "assistant";
  content:
    | string
    | Array<
        | { type: "text"; text: string }
        | { type: "image_url"; image_url: { url: string } }
      >;
};
type RequestOptions = { signal?: AbortSignal };
type GeneratedImage = { id: string; dataUrl: string };

export function requestGeneration(
  _config: AiConfig,
  _prompt: string,
  options?: RequestOptions,
) {
  return unavailableGeneration<GeneratedImage[]>(options?.signal);
}
export function requestEdit(
  _config: AiConfig,
  _prompt: string,
  _references: ReferenceImage[],
  options?: RequestOptions,
) {
  return unavailableGeneration<GeneratedImage[]>(options?.signal);
}
export function requestImageQuestion(
  _config: AiConfig,
  _messages: AiTextMessage[],
  _onDelta: (text: string) => void,
  options?: RequestOptions,
) {
  return unavailableGeneration<string>(options?.signal);
}
