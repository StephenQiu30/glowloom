import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { render } from "../../../../tests/feedback";
import { ProvidersPage } from "./providers-page";
const api = vi.hoisted(() => ({
  providers: vi.fn(),
  presets: vi.fn(),
  role: "creator",
  delayed: false,
  pendingSave: null as null | (() => void),
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ actor: { id: "actor", platform_role: api.role } }),
}));
vi.mock("@/api/settings", () => ({
  listAdminProviders: api.providers,
  listProviderPresets: api.presets,
  getConfigReceipt: vi.fn(),
  updateProviderPresetState: vi.fn(),
  updateAdminProvider: vi.fn(),
  exportProviderPreset: vi.fn(),
}));
vi.mock("./preset-editor", () => ({ PresetEditor: () => null }));
vi.mock("./preset-connect", () => ({
  PresetConnect: ({
    onCreated,
    onSaved,
  }: {
    onCreated: (id: string) => void;
    onSaved: (id: string) => void;
  }) => (
    <div>
      接入表单
      <button
        onClick={() => {
          onCreated("connection");
          if (api.delayed) api.pendingSave = () => onSaved("connection");
          else onSaved("connection");
        }}
      >
        模拟保存完成
      </button>
    </div>
  ),
}));
vi.mock("./connection-detail", () => ({
  ConnectionDetail: () => <div>连接详情内容</div>,
}));
beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  api.role = "creator";
  api.delayed = false;
  api.pendingSave = null;
  api.providers.mockResolvedValue({ items: [] });
  api.presets.mockResolvedValue({ items: [] });
});
afterEach(cleanup);
it("普通账号不发管理查询", () => {
  render(<ProvidersPage />);
  expect(screen.getByText("暂无访问权限")).toBeTruthy();
  expect(api.providers).not.toHaveBeenCalled();
  expect(api.presets).not.toHaveBeenCalled();
});
it("空列表只保留接入入口，表单使用可关闭的 Dialog", async () => {
  api.role = "platform_admin";
  render(<ProvidersPage />);
  await screen.findByText("还没有连接");
  expect(screen.queryByText("接入表单")).toBeNull();
  const trigger = screen.getAllByRole("button", { name: "接入服务" })[0];
  trigger.focus();
  fireEvent.click(trigger);
  expect(screen.getByRole("dialog", { name: "接入 AI 服务" })).toBeTruthy();
  expect(screen.getByText("接入表单")).toBeTruthy();
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();
  await waitFor(() => expect(document.activeElement).toBe(trigger));
});
it("连接通过详情抽屉查看，列表不把模型数量当发布结果", async () => {
  api.role = "platform_admin";
  api.providers.mockResolvedValue({
    items: [
      {
        provider: { id: "connection", name: "OpenRouter", status: "disabled" },
        credential: { last4: "1234", status: "active" },
        model_count: 2,
      },
    ],
  });
  render(<ProvidersPage />);
  const buttons = await screen.findAllByRole("button", {
    name: "查看 OpenRouter",
  });
  expect(screen.queryByText("连接详情内容")).toBeNull();
  expect(screen.queryByText("已发布")).toBeNull();
  fireEvent.click(buttons[0]);
  expect(screen.getByRole("dialog", { name: "连接详情" })).toBeTruthy();
  expect(screen.getByText("连接详情内容")).toBeTruthy();
});
it("加载失败显示恢复入口，不伪装成空连接", async () => {
  api.role = "platform_admin";
  api.providers.mockRejectedValue(new Error("暂不可用"));
  render(<ProvidersPage />);
  await screen.findByRole("button", { name: "重新加载" });
  expect(screen.queryByText("还没有连接")).toBeNull();
});

it("保存后读回成功才关闭弹窗，详情读取不会触发重复写入", async () => {
  api.role = "platform_admin";
  api.providers.mockResolvedValueOnce({ items: [] }).mockResolvedValue({
    items: [
      {
        provider: { id: "connection", name: "保存的连接" },
        credential: { status: "active", last4: "test" },
      },
    ],
  });
  render(<ProvidersPage />);
  await screen.findByText("还没有连接");
  fireEvent.click(screen.getAllByRole("button", { name: "接入服务" })[0]);
  fireEvent.click(screen.getByRole("button", { name: "模拟保存完成" }));
  await screen.findByText("连接详情内容");
  expect(screen.queryByRole("dialog", { name: "接入 AI 服务" })).toBeNull();
  expect(api.providers).toHaveBeenCalledTimes(2);
});

it("关闭后仍展示未确认写入，继续接入只打开原流程而不自动写入", async () => {
  api.role = "platform_admin";
  localStorage.setItem(
    "glowloom.catalog.pending.v1.actor.preset.onboarding.credential",
    JSON.stringify({
      id: "original",
      action: "credential.set",
      target: "connection",
    }),
  );
  render(<ProvidersPage />);
  await screen.findByText("还没有连接");
  expect(screen.getByText("保存待确认")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "继续接入" }));
  expect(screen.getByRole("dialog", { name: "接入 AI 服务" })).toBeTruthy();
});

it("关闭后收到保存成功只刷新列表，不重新弹出详情", async () => {
  api.role = "platform_admin";
  api.delayed = true;
  render(<ProvidersPage />);
  await screen.findByText("还没有连接");
  fireEvent.click(screen.getAllByRole("button", { name: "接入服务" })[0]);
  fireEvent.click(screen.getByRole("button", { name: "模拟保存完成" }));
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  api.providers.mockResolvedValue({
    items: [
      {
        provider: { id: "connection", name: "后台保存的连接" },
        credential: { status: "active", last4: "test" },
      },
    ],
  });
  act(() => api.pendingSave?.());
  await screen.findAllByRole("button", { name: "查看 后台保存的连接" });
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(api.providers).toHaveBeenCalledTimes(2);
});
