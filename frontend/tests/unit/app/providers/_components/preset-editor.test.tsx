import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { render } from "../../../../feedback";
import { PresetEditor } from "@/app/providers/_components/preset-editor";
const api = vi.hoisted(() => ({
  preview: vi.fn(),
  save: vi.fn(),
  version: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({
    actor: { id: "admin", platform_role: "platform_admin" },
  }),
}));
vi.mock("@/api/settings", () => ({
  previewProviderPreset: api.preview,
  createProviderPreset: api.save,
  publishProviderPresetVersion: api.version,
  getConfigReceipt: vi.fn(),
}));
vi.mock("@/app/providers/_components/preset-models", () => ({
  PresetModels: () => <div>可复用模型模板</div>,
}));
const preset: API.Preset = {
  id: "builtin",
  origin: "builtin",
  document: {
    schema_version: 1,
    name: "OpenRouter",
    config: {
      adapter_key: "openrouter",
      base_url: "https://openrouter.ai/api/v1",
      region: "overseas",
      capabilities: ["text", "image", "video"],
      timeout_seconds: 120,
      concurrency_limit: 2,
      rate_limit_per_min: 30,
    },
  },
};
beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  api.preview.mockImplementation(async ({ document_json }) => ({
    document: JSON.parse(document_json),
    hash: "preview",
  }));
  api.save.mockResolvedValue({ id: "custom" });
});
afterEach(cleanup);
it("管理员通过表单维护预设，无需手写 JSON 或标识；修改使预览失效", async () => {
  render(
    <PresetEditor presets={[preset]} onClose={vi.fn()} onSaved={vi.fn()} />,
  );
  expect(screen.queryByLabelText("预设 JSON（高级导入）")).toBeNull();
  fireEvent.change(screen.getByLabelText("预设名称"), {
    target: { value: "工作室 OpenRouter" },
  });
  fireEvent.click(screen.getByRole("button", { name: "高级设置" }));
  fireEvent.change(screen.getByLabelText("最大并发"), {
    target: { value: "4" },
  });
  fireEvent.click(screen.getByRole("button", { name: "校验并预览" }));
  await waitFor(() => expect(api.preview).toHaveBeenCalledOnce());
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "保存新版本" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
  fireEvent.change(screen.getByLabelText("预设名称"), {
    target: { value: "修改后的预设" },
  });
  expect(
    (screen.getByRole("button", { name: "保存新版本" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "校验并预览" }));
  await waitFor(() => expect(api.preview).toHaveBeenCalledTimes(2));
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "保存新版本" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "保存新版本" }));
  await waitFor(() => expect(api.save).toHaveBeenCalledOnce());
  const input = api.save.mock.calls[0][0];
  expect(input.slug).toMatch(/^preset-/);
  expect(input.document.config.concurrency_limit).toBe(4);
  expect(input.document.name).toBe("修改后的预设");
});
it("复制内置预设创建派生副本，不写回内置版本", async () => {
  render(
    <PresetEditor
      preset={preset}
      presets={[preset]}
      onClose={vi.fn()}
      onSaved={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "校验并预览" }));
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "保存新版本" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "保存新版本" }));
  await waitFor(() => expect(api.save).toHaveBeenCalledOnce());
  expect(api.save.mock.calls[0][0].parent_id).toBe("builtin");
  expect(api.version).not.toHaveBeenCalled();
});
