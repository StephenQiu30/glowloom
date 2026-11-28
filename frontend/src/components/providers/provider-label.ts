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

export function providerLabel(
  adapter?: string,
  adapters?: { adapter_key?: string; display_name?: string }[],
) {
  const declared = adapters?.find(
    (item) => item.adapter_key === adapter,
  )?.display_name;
  if (declared) return declared;
  return adapter && Object.hasOwn(labels, adapter)
    ? labels[adapter]
    : "其他服务";
}
