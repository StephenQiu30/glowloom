const labels: Record<string, string> = {
  volcengine_ark: "火山方舟",
  volcengine_speech: "豆包语音",
  volcengine_music: "火山音乐",
  volcengine_avatar: "即梦数字人",
  volcengine_motion: "即梦动作模仿",
  openrouter: "OpenRouter",
  openai: "OpenAI",
  deepseek: "DeepSeek",
  byteplus: "BytePlus",
  vercel: "Vercel",
};

export function providerLabel(adapter?: string) {
  return adapter && Object.hasOwn(labels, adapter)
    ? labels[adapter]
    : "其他服务";
}

const connectionAdapters = [
  "openai",
  "deepseek",
  "volcengine_ark",
  "volcengine_speech",
  "volcengine_music",
  "openrouter",
  "vercel",
];

export function canConnectPreset(preset: API.Preset) {
  return connectionAdapters.includes(
    preset.document?.config?.adapter_key ?? "",
  );
}

/** Restrict new choices while keeping a pending historical request recoverable. */
export function connectionPresets(
  presets: API.Preset[],
  pendingPreset?: string,
) {
  return presets
    .filter((preset) => canConnectPreset(preset) || preset.id === pendingPreset)
    .toSorted((a, b) => {
      const rank = (preset: API.Preset) => {
        const index = connectionAdapters.indexOf(
          preset.document?.config?.adapter_key ?? "",
        );
        return index < 0 ? connectionAdapters.length : index;
      };
      return rank(a) - rank(b);
    });
}

/** Public console roots only; never append keys, prompts or task URLs. */
export function providerConsoleURL(adapter?: string) {
  return (
    {
      openai: "https://platform.openai.com/",
      deepseek: "https://platform.deepseek.com/",
      volcengine_ark: "https://console.volcengine.com/ark",
      openrouter: "https://openrouter.ai/",
      vercel: "https://vercel.com/",
    } as Record<string, string>
  )[adapter ?? ""];
}

export type ModelBrand = "ChatGPT" | "Anthropic" | "Gemini" | "Grok";

export function getCandidateBrand(candidate?: {
  provider_model_id?: string;
  display_name?: string;
  key?: string;
}): ModelBrand | undefined {
  if (!candidate) return undefined;
  const id = candidate.provider_model_id?.toLowerCase() ?? "";
  const name = candidate.display_name?.toLowerCase() ?? "";
  const key = candidate.key?.toLowerCase() ?? "";

  if (
    id.startsWith("openai/") ||
    name.includes("gpt") ||
    name.includes("chatgpt") ||
    name.startsWith("o1") ||
    name.startsWith("o3") ||
    name.includes("dall-e") ||
    key.includes("gpt") ||
    key.includes("openai") ||
    key.includes("-o1") ||
    key.includes("-o3")
  ) {
    return "ChatGPT";
  }
  if (
    id.startsWith("anthropic/") ||
    name.includes("claude") ||
    name.includes("anthropic") ||
    key.includes("claude")
  ) {
    return "Anthropic";
  }
  if (
    id.startsWith("google/") ||
    name.includes("gemini") ||
    name.includes("google") ||
    key.includes("gemini")
  ) {
    return "Gemini";
  }
  if (
    id.startsWith("x-ai/") ||
    name.includes("grok") ||
    name.includes("xai") ||
    key.includes("grok")
  ) {
    return "Grok";
  }
  return undefined;
}
