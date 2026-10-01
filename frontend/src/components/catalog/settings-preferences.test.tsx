import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { PromptPreferenceEditor } from "./settings-prompts";
import { ProjectDefaultsForm } from "./settings-defaults";
import {
  promptOperations,
  queryProjectModelDefaults,
  queryPromptPreferences,
  type PromptPreference,
} from "./settings-preferences-queries";

const api = vi.hoisted(() => ({
  savePromptPreference: vi.fn(),
  saveProjectModelDefaults: vi.fn(),
  getProjectModelDefaults: vi.fn(),
  listPromptPreferences: vi.fn(),
}));
vi.mock("@/gen/api/settings", async (original) => ({
  ...(await original<typeof import("@/gen/api/settings")>()),
  ...api,
}));
const id = "40000000-0000-4000-8000-000000000001";
const customization = {
  id,
  operation: "short_drama_outline" as const,
  mode: "append" as const,
  content: "保存的创作要求",
  base_template_id: id,
  revision: 7,
  update_time: "2026-10-01T00:00:00Z",
};
const preference: PromptPreference = {
  definition: {
    operation: "short_drama_outline",
    label: "短剧大纲",
    category: "剧本",
    description: "生成短剧大纲",
    output_type: "json",
    schema_key: "outline/v1",
    content: "创作 {{章节数量}} 个章节。",
    output_contract: '必须输出 {"chapters":[]}',
    template_id: id,
    template_version: 1,
    variables: [{ label: "章节数量", placeholder: "{{章节数量}}" }],
  },
  customization,
  outdated: false,
};
beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
describe("持久创作偏好", () => {
  it("未知变量阻止保存，输出契约仍只读", async () => {
    render(
      <PromptPreferenceEditor
        initial={preference}
        saved={vi.fn()}
        dirty={vi.fn()}
        reload={vi.fn()}
      />,
    );
    fireEvent.change(screen.getByLabelText("追加的创作要求"), {
      target: { value: "创作 {{未声明变量}}" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存提示词偏好" }));
    await waitFor(() =>
      expect(
        screen.getByText("仅支持下方声明的变量，请检查双花括号"),
      ).toBeTruthy(),
    );
    expect(api.savePromptPreference).not.toHaveBeenCalled();
    expect(
      screen.getByText(preference.definition.output_contract),
    ).toBeTruthy();
    expect(screen.queryByLabelText("受保护的输出格式")).toBeNull();
  });
  it("未确认保存重试复用同一请求，后台刷新不会改写编辑修订", async () => {
    api.savePromptPreference.mockRejectedValue(
      new ApiError(0, "dependency_unavailable"),
    );
    const saved = vi.fn(),
      dirty = vi.fn();
    const { rerender } = render(
      <PromptPreferenceEditor
        initial={preference}
        saved={saved}
        dirty={dirty}
        reload={vi.fn()}
      />,
    );
    fireEvent.change(screen.getByLabelText("追加的创作要求"), {
      target: { value: "保持 {{章节数量}} 集悬念" },
    });
    rerender(
      <PromptPreferenceEditor
        initial={{
          ...preference,
          customization: { ...customization, revision: 9 },
        }}
        saved={saved}
        dirty={dirty}
        reload={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "保存提示词偏好" }));
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toContain("尚未确认保存"),
    );
    fireEvent.click(screen.getByRole("button", { name: "保存提示词偏好" }));
    await waitFor(() =>
      expect(api.savePromptPreference).toHaveBeenCalledTimes(2),
    );
    expect(api.savePromptPreference.mock.calls[0][1]).toMatchObject({
      expected_revision: 7,
      base_template_id: id,
      mode: "append",
      content: "保持 {{章节数量}} 集悬念",
    });
    expect(
      api.savePromptPreference.mock.calls[0][2].headers["Idempotency-Key"],
    ).toBe(
      api.savePromptPreference.mock.calls[1][2].headers["Idempotency-Key"],
    );
    expect(screen.getByLabelText("追加的创作要求")).toHaveProperty(
      "value",
      "保持 {{章节数量}} 集悬念",
    );
    expect(saved).not.toHaveBeenCalled();
  });
  it("恢复默认要确认并持久写入当前操作，不改其他模板", async () => {
    api.savePromptPreference.mockResolvedValue({
      ...customization,
      mode: "inherit",
      content: "",
      revision: 8,
    });
    const saved = vi.fn();
    render(
      <PromptPreferenceEditor
        initial={preference}
        saved={saved}
        dirty={vi.fn()}
        reload={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "恢复默认" }));
    expect(api.savePromptPreference).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "确认恢复默认" }));
    await waitFor(() => expect(saved).toHaveBeenCalledOnce());
    expect(api.savePromptPreference.mock.calls[0][0]).toEqual({
      operation: "short_drama_outline",
    });
    expect(api.savePromptPreference.mock.calls[0][1]).toEqual({
      expected_revision: 7,
      base_template_id: id,
      mode: "inherit",
      content: "",
    });
    expect(screen.queryByLabelText("追加的创作要求")).toBeNull();
    expect(screen.getByText("已恢复为默认模板。")).toBeTruthy();
  });
  it("默认选择后台刷新后仍用旧修订保存，失联不重复建请求", async () => {
    api.saveProjectModelDefaults.mockRejectedValue(
      new ApiError(0, "dependency_unavailable"),
    );
    const initial = {
      project_id: id,
      revision: 7,
      default_models: { "image.generate": "fixture-model" },
    };
    const props = { initial, models: [], reload: vi.fn(), saved: vi.fn() };
    const { container, rerender } = render(<ProjectDefaultsForm {...props} />);
    rerender(
      <ProjectDefaultsForm {...props} initial={{ ...initial, revision: 9 }} />,
    );
    fireEvent.submit(container.querySelector("form")!);
    await waitFor(() =>
      expect(api.saveProjectModelDefaults).toHaveBeenCalledOnce(),
    );
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toContain("尚未确认保存"),
    );
    fireEvent.submit(container.querySelector("form")!);
    await waitFor(() =>
      expect(api.saveProjectModelDefaults).toHaveBeenCalledTimes(2),
    );
    expect(api.saveProjectModelDefaults.mock.calls[0][1]).toEqual({
      expected_revision: 7,
      default_models: { "image.generate": "fixture-model" },
    });
    expect(
      api.saveProjectModelDefaults.mock.calls[0][2].headers["Idempotency-Key"],
    ).toBe(
      api.saveProjectModelDefaults.mock.calls[1][2].headers["Idempotency-Key"],
    );
    expect(props.saved).not.toHaveBeenCalled();
  });
  it("拒绝跨项目响应和重复模板目录", async () => {
    api.getProjectModelDefaults.mockResolvedValue({
      project_id: "40000000-0000-4000-8000-000000000002",
      revision: 1,
      default_models: {},
    });
    await expect(queryProjectModelDefaults(id)).rejects.toMatchObject({
      code: "invalid_response",
    });
    api.listPromptPreferences.mockResolvedValue({
      items: promptOperations.map(() => preference),
    });
    await expect(queryPromptPreferences()).rejects.toMatchObject({
      code: "invalid_response",
    });
  });
});
