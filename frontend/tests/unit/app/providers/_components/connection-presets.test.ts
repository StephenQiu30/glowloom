import { expect, it } from "vitest";
import { connectionPresets } from "@/app/providers/_components/provider-label";
const preset = (adapter: string): API.Preset => ({
  id: adapter,
  document: { name: adapter, config: { adapter_key: adapter } },
});
it("新接入只展示五类服务并排序，历史预设不删除", () => {
  const input = [
    "openrouter",
    "volcengine_music",
    "deepseek",
    "byteplus",
    "volcengine_ark",
    "openai",
    "volcengine_speech",
  ].map(preset);
  expect(connectionPresets(input).map((p) => p.id)).toEqual([
    "openai",
    "deepseek",
    "volcengine_ark",
    "volcengine_speech",
    "volcengine_music",
  ]);
  expect(input).toHaveLength(7);
});
it("只恢复指定旧请求，不提供其他历史服务选项", () => {
  const input = [preset("openrouter"), preset("byteplus"), preset("deepseek")];
  expect(connectionPresets(input, "openrouter").map((p) => p.id)).toEqual([
    "deepseek",
    "openrouter",
  ]);
});
