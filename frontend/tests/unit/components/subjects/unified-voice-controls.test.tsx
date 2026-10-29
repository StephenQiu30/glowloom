import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { UnifiedSubjectVoiceControls } from "@/components/subjects/unified-voice-controls";
import type { useSubjectVoiceJob } from "@/components/subjects/unified-voice-job";
type Owner = ReturnType<typeof useSubjectVoiceJob>;
const release = "11111111-1111-4111-8111-111111111111";
const model = {
  release_id: release,
  model_version_id: "22222222-2222-4222-8222-222222222222",
  model_key: "doubao",
  display_name: "豆包语音",
  voices: [{ key: "speaker", name: "清晰女声" }],
};
function owner(phase: Owner["phase"]): Owner {
  return {
    phase,
    options: {
      current_actor_id: "actor",
      current_scope_id: "scope",
      available: true,
      models: [model],
    },
    job: null,
    error: "",
    active: true,
    begin: vi.fn(),
    generate: vi.fn(),
    recover: vi.fn(),
    retryOriginal: vi.fn(),
    reset: vi.fn(),
  };
}
afterEach(cleanup);
it("配置不可用仅提供重试，不显示技术配置或供应商管理入口", () => {
  const value = {
    ...owner("unavailable"),
    active: false,
    error: "智能配音暂不可用，请稍后重试。",
  };
  render(<UnifiedSubjectVoiceControls owner={value} />);
  expect(screen.getByRole("alert").textContent).toContain("智能配音暂不可用");
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.queryByRole("combobox")).toBeNull();
  expect(screen.queryByRole("textbox")).toBeNull();
  expect(screen.queryByRole("link", { name: "接入语音模型" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试" }));
  expect(value.begin).toHaveBeenCalledOnce();
});
it("音频保存待重试展示明确阶段，只恢复原媒体导入", () => {
  const value = {
    ...owner("import_pending"),
    error: "音频已生成，保存尚未完成。重试保存音频不会重新合成。",
  };
  render(<UnifiedSubjectVoiceControls owner={value} />);
  expect(screen.queryByRole("button", { name: "音色加载中" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试保存音频" }));
  expect(value.retryOriginal).toHaveBeenCalledOnce();
  expect(value.begin).not.toHaveBeenCalled();
});
it("unknown只提供恢复原任务，不提供新生成入口", () => {
  const value = {
    ...owner("unknown"),
    error: "音色生成结果尚未确认，请恢复读取。",
  };
  render(<UnifiedSubjectVoiceControls owner={value} />);
  fireEvent.click(screen.getByRole("button", { name: "恢复读取" }));
  expect(value.recover).toHaveBeenCalledOnce();
  expect(screen.queryByRole("button", { name: "重新生成" })).toBeNull();
});
