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
it("唯一模型无默认音色时必须明确选择，再生成真实试听文本", () => {
  const value = owner("choosing");
  render(<UnifiedSubjectVoiceControls owner={value} />);
  expect(screen.queryByRole("combobox", { name: "配音模型" })).toBeNull();
  expect(screen.getByRole("button", { name: "生成音色" })).toHaveProperty(
    "disabled",
    true,
  );
  fireEvent.keyDown(screen.getByRole("combobox", { name: "配音音色" }), {
    key: "ArrowDown",
  });
  fireEvent.keyDown(screen.getByRole("option", { name: "清晰女声" }), {
    key: "Enter",
  });
  fireEvent.change(screen.getByLabelText("试听文本"), {
    target: { value: "这是确认过的试听句。" },
  });
  fireEvent.click(screen.getByRole("button", { name: "生成音色" }));
  expect(value.generate).toHaveBeenCalledWith(
    release,
    "speaker",
    "智能配音1",
    "这是确认过的试听句。",
  );
  expect(value.begin).not.toHaveBeenCalled();
});
it("多模型配置切换会清除不属于新模型的音色，取消不创建任务", () => {
  const value = owner("choosing");
  value.options!.models.push({
    ...model,
    release_id: "33333333-3333-4333-8333-333333333333",
    display_name: "豆包语音备选",
    voices: [{ key: "other", name: "清晰男声" }],
  });
  render(<UnifiedSubjectVoiceControls owner={value} />);
  function select(label: string, option: string) {
    fireEvent.keyDown(screen.getByRole("combobox", { name: label }), {
      key: "ArrowDown",
    });
    fireEvent.keyDown(screen.getByRole("option", { name: option }), {
      key: "Enter",
    });
  }
  select("配音模型", "豆包语音");
  select("配音音色", "清晰女声");
  expect(screen.getByRole("button", { name: "生成音色" })).toHaveProperty(
    "disabled",
    false,
  );
  select("配音模型", "豆包语音备选");
  expect(screen.getByRole("button", { name: "生成音色" })).toHaveProperty(
    "disabled",
    true,
  );
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(value.reset).toHaveBeenCalledOnce();
  expect(value.generate).not.toHaveBeenCalled();
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
