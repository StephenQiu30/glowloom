import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SettingsWorkspace } from "./settings-workspace";
const navigation = vi.hoisted(() => ({ tab: "providers" }));
const projectScope = vi.hoisted(() => vi.fn());
vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams({ tab: navigation.tab }),
  useRouter: () => ({ replace: vi.fn() }),
}));
vi.mock("@/components/workbench/project-scope", () => ({
  ProjectScope: projectScope,
}));
vi.mock("./settings-providers", () => ({
  SettingsProviders: () => <p>供应商测试视图</p>,
}));
vi.mock("./settings-prompts", () => ({
  SettingsPrompts: () => <input aria-label="偏好草稿" defaultValue="" />,
}));
afterEach(cleanup);
it("直接打开管理分区不挂载项目选择器，切换分区保留已编辑的偏好草稿", async () => {
  navigation.tab = "providers";
  const view = render(<SettingsWorkspace />);
  await screen.findByText("供应商测试视图");
  expect(projectScope).not.toHaveBeenCalled();
  navigation.tab = "prompts";
  view.rerender(<SettingsWorkspace />);
  const input = await screen.findByLabelText("偏好草稿");
  fireEvent.change(input, { target: { value: "保留用户未保存要求" } });
  navigation.tab = "providers";
  view.rerender(<SettingsWorkspace />);
  navigation.tab = "prompts";
  view.rerender(<SettingsWorkspace />);
  expect(screen.getByLabelText("偏好草稿")).toHaveProperty(
    "value",
    "保留用户未保存要求",
  );
  expect(projectScope).not.toHaveBeenCalled();
});
