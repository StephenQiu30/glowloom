import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ProviderLogo } from "@/app/providers/_components/provider-logo";
import { providerLabel } from "@/app/providers/_components/provider-label";

describe("ProviderLogo", () => {
  it("OpenAI 和 OpenRouter 使用前景色遮罩自适应单色渲染", () => {
    const { container: openaiContainer } = render(
      <ProviderLogo adapterKey="openai" />,
    );
    const spanOpenAI = openaiContainer.querySelector("span");
    expect(spanOpenAI).toBeTruthy();
    expect(spanOpenAI?.getAttribute("style")).toContain(
      "/brand/providers/openai.svg",
    );

    const { container: openrouterContainer } = render(
      <ProviderLogo adapterKey="openrouter" />,
    );
    const spanOpenRouter = openrouterContainer.querySelector("span");
    expect(spanOpenRouter).toBeTruthy();
    expect(spanOpenRouter?.getAttribute("style")).toContain(
      "/brand/providers/openrouter.svg",
    );
  });

  it("火山引擎各服务及即梦均精准映射官方 Logo，无一回退为地球网格", () => {
    const checks: [string, string][] = [
      ["volcengine_ark", "/brand/providers/volcengine.svg"],
      ["volcengine_music", "/brand/providers/volcengine.svg"],
      ["volcengine_avatar", "/brand/providers/jimeng.svg"],
      ["volcengine_motion", "/brand/providers/jimeng.svg"],
      ["volcengine_speech", "/brand/providers/doubao.svg"],
      ["deepseek", "/brand/providers/deepseek.svg"],
      ["byteplus", "/brand/providers/byteplus.png"],
    ];

    for (const [adapter, expectedSrc] of checks) {
      const { container } = render(<ProviderLogo adapterKey={adapter} />);
      const img = container.querySelector("img");
      expect(img, `${adapter} 应该渲染为 img 元素`).toBeTruthy();
      expect(img?.getAttribute("src")).toBe(expectedSrc);
    }
  });

  it("未知适配器正确回退到通用地球图标", () => {
    const { container } = render(<ProviderLogo adapterKey="unknown_provider" />);
    expect(container.querySelector("svg")).toBeTruthy();
    expect(container.querySelector("img")).toBeNull();
  });
});

describe("providerLabel", () => {
  it("为所有支持的服务返回规范中文或品牌名称", () => {
    expect(providerLabel("openai")).toBe("OpenAI");
    expect(providerLabel("volcengine_ark")).toBe("火山方舟");
    expect(providerLabel("volcengine_music")).toBe("火山音乐");
    expect(providerLabel("volcengine_avatar")).toBe("即梦数字人");
    expect(providerLabel("volcengine_motion")).toBe("即梦动作模仿");
    expect(providerLabel("volcengine_speech")).toBe("豆包语音");
    expect(providerLabel("deepseek")).toBe("DeepSeek");
    expect(providerLabel("openrouter")).toBe("OpenRouter");
    expect(providerLabel("byteplus")).toBe("BytePlus");
    expect(providerLabel("unknown")).toBe("其他服务");
  });
});
