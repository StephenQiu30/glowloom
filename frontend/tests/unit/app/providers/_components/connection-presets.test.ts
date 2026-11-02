import { expect, it } from "vitest";
import { connectionPresets } from "@/app/providers/_components/provider-label";
const preset = (adapter: string): API.Preset => ({
  id: adapter,
  document: { name: adapter, config: { adapter_key: adapter } },
});
it("新接入展示七类服务并按序排列，历史预设不删除", () => {
  const input = [
    "openrouter",
    "volcengine_music",
    "deepseek",
    "byteplus",
    "volcengine_ark",
    "openai",
    "volcengine_speech",
    "vercel",
  ].map(preset);
  expect(connectionPresets(input).map((p) => p.id)).toEqual([
    "openai",
    "deepseek",
    "volcengine_ark",
    "volcengine_speech",
    "volcengine_music",
    "openrouter",
    "vercel",
  ]);
  expect(input).toHaveLength(8);
});
it("只恢复指定旧请求，不提供其他历史服务选项", () => {
  const input = [preset("byteplus"), preset("deepseek")];
  expect(connectionPresets(input, "byteplus").map((p) => p.id)).toEqual([
    "deepseek",
    "byteplus",
  ]);
});
