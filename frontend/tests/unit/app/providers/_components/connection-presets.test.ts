import { expect, it } from "vitest";
import {
  connectionPresets,
  getCandidateBrand,
  BrandSpecification,
} from "@/app/providers/_components/provider-label";
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

it("规约模式正确识别四大品牌 ChatGPT Anthropic Gemini Grok", () => {
  expect(
    getCandidateBrand({
      provider_model_id: "openai/gpt-4o",
      display_name: "GPT-4o",
    }),
  ).toBe("ChatGPT");

  expect(
    getCandidateBrand({
      provider_model_id: "anthropic/claude-3.7-sonnet",
      display_name: "Claude 3.7 Sonnet",
    }),
  ).toBe("Anthropic");

  expect(
    getCandidateBrand({
      provider_model_id: "google/gemini-2.5-pro",
      display_name: "Gemini 2.5 Pro",
    }),
  ).toBe("Gemini");

  expect(
    getCandidateBrand({
      provider_model_id: "x-ai/grok-2",
      display_name: "Grok 2",
    }),
  ).toBe("Grok");

  // Non-brand returns undefined
  expect(
    getCandidateBrand({
      provider_model_id: "meta-llama/llama-3-70b",
      display_name: "Llama 3 70B",
    }),
  ).toBeUndefined();
});
