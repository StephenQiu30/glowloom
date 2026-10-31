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
};

export function providerLabel(adapter?: string) {
  return adapter && Object.hasOwn(labels, adapter)
    ? labels[adapter]
    : "其他服务";
}
