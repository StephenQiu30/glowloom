const labels: Record<string, string> = {
  volcengine_ark: "火山方舟",
  volcengine_speech: "豆包语音",
  openrouter: "OpenRouter",
  deepseek: "DeepSeek",
  byteplus: "BytePlus",
};

export function providerLabel(adapter?: string) {
  return adapter && Object.hasOwn(labels, adapter)
    ? labels[adapter]
    : "其他服务";
}
