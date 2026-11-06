import { pageBusinessMemory } from "@/lib/page-business-memory";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { render } from "../../../../feedback";
import { ProvidersPage } from "@/app/providers/_components/providers-page";
const api = vi.hoisted(() => ({
  providers: vi.fn(),
  detail: vi.fn(),
  presets: vi.fn(),
  deletionPreview: vi.fn(),
  deleteConnection: vi.fn(),
  role: "creator",
  delayed: false,
  pendingSave: null as null | (() => void),
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ actor: { id: "actor", platform_role: api.role } }),
}));
vi.mock("@/api/settings", () => ({
  listAdminProviders: api.providers,
  getAdminProvider: api.detail,
  previewAdminProviderDeletion: api.deletionPreview,
  deleteAdminProvider: api.deleteConnection,
  listProviderPresets: api.presets,
  listAdminProviderAdapters: vi.fn().mockResolvedValue([]),
  getConfigReceipt: vi.fn(),
  updateProviderPresetState: vi.fn(),
  updateAdminProvider: vi.fn(),
  exportProviderPreset: vi.fn(),
}));
vi.mock("@/app/providers/_components/preset-editor", () => ({
  PresetEditor: () => null,
}));
vi.mock("@/app/providers/_components/preset-connect", () => ({
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
vi.mock("@/app/providers/_components/connection-detail", () => ({
  ConnectionDetail: ({ Title = "h2" }: { Title?: React.ElementType }) => (
    <>
      <Title>连接详情</Title>
      <div>连接详情内容</div>
    </>
  ),
}));
beforeEach(() => {
  vi.clearAllMocks();
  pageBusinessMemory.clear();
  api.detail.mockResolvedValue({
    provider: { id: "connection" },
    credential: { status: "active" },
  });
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
  const trigger = screen.getByRole("button", { name: "接入服务" });
  trigger.focus();
  fireEvent.click(trigger);
  expect(screen.getByRole("dialog", { name: "接入服务" })).toBeTruthy();
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
  expect(
    screen.getByRole("dialog", { name: "连接详情" }).getAttribute("data-slot"),
  ).toBe("dialog-content");
  expect(screen.getByText("连接详情内容")).toBeTruthy();
});
it("加载失败显示恢复入口，不伪装成空连接", async () => {
  api.role = "platform_admin";
  api.providers.mockRejectedValue(new Error("暂不可用"));
  render(<ProvidersPage />);
  await screen.findByRole("button", { name: "重试" });
  expect(screen.queryByText("还没有连接")).toBeNull();
});

it("成功回执后关闭接入弹窗并刷新列表，不自动打开详情", async () => {
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
  await screen.findByRole("button", { name: "查看 保存的连接" });
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.queryByText("连接详情内容")).toBeNull();
  expect(api.detail).not.toHaveBeenCalled();
});

it("关闭后仍展示未确认写入，继续接入只打开原流程而不自动写入", async () => {
  api.role = "platform_admin";
  pageBusinessMemory.setItem(
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
  expect(screen.getByRole("dialog", { name: "接入服务" })).toBeTruthy();
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
  expect(api.providers).toHaveBeenCalledTimes(4);
});

it.each(["creator", "admin", "team_admin"])(
  "%s 直接打开连接不展示管理数据",
  (role) => {
    api.role = role;
    render(<ProvidersPage initialConnection="private-connection" />);
    expect(screen.getByText("暂无访问权限")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "接入服务" })).toBeNull();
    expect(screen.queryByText("连接详情内容")).toBeNull();
    expect(api.providers).not.toHaveBeenCalled();
    expect(api.presets).not.toHaveBeenCalled();
  },
);

it("页头打开独立供应商预设列表并可返回连接", async () => {
  api.role = "platform_admin";
  api.presets.mockResolvedValue({
    items: [
      {
        id: "preset",
        origin: "builtin",
        version: 9,
        document: {
          name: "BytePlus",
          config: { adapter_key: "byteplus", region: "overseas" },
        },
      },
    ],
  });
  render(<ProvidersPage />);
  await screen.findByText("还没有连接");
  // Shared management navigation is exercised by the persistent shell tests.
  expect(screen.queryByRole("navigation", { name: "管理页面" })).toBeNull();
  expect(screen.queryByRole("button", { name: "使用预设" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "供应商预设" }));
  expect(
    await screen.findByRole("heading", { name: "供应商预设" }),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "BytePlus预设的操作" }),
  ).toBeTruthy();
  expect(screen.queryByText("v9")).toBeNull();
  expect(screen.queryByRole("button", { name: "接入服务" })).toBeNull();
  expect(screen.queryByRole("button", { name: "预设管理" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "返回 AI 服务接入" }));
  expect(await screen.findByText("还没有连接")).toBeTruthy();
});

it("权限撤销立即卸载管理详情，再恢复权限重新读取", async () => {
  api.role = "platform_admin";
  const page = render(<ProvidersPage initialConnection="private-connection" />);
  await screen.findByText("还没有连接");
  expect(screen.getByText("连接详情内容")).toBeTruthy();
  api.role = "creator";
  page.rerender(<ProvidersPage initialConnection="private-connection" />);
  expect(screen.getByText("暂无访问权限")).toBeTruthy();
  expect(screen.queryByText("连接详情内容")).toBeNull();
  api.role = "platform_admin";
  page.rerender(<ProvidersPage />);
  await screen.findByText("还没有连接");
  expect(api.providers).toHaveBeenCalledTimes(2);
});

it.each([
  ["disabled", "Key 已停用"],
  ["draining", "Key 仅供恢复"],
])("凭据 %s 时不将历史检查通过显示为当前健康状态", async (status, label) => {
  api.role = "platform_admin";
  api.providers.mockResolvedValue({
    items: [
      {
        provider: { id: "connection", name: "服务", status: "active" },
        credential: { status, last_test_result: "ok" },
        model_count: 0,
      },
    ],
  });
  render(<ProvidersPage />);
  await screen.findByRole("button", { name: "显示列" });
  fireEvent.pointerDown(screen.getByRole("button", { name: "显示列" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(
    await screen.findByRole("menuitemcheckbox", { name: "最近检查" }),
  );
  await screen.findByText(label);
  expect(screen.queryByText("检查通过")).toBeNull();
});

it("服务筛选往返后不恢复此前的批量选择", async () => {
  api.role = "platform_admin";
  api.providers.mockResolvedValue({
    items: [
      { provider: { id: "alpha", name: "Alpha", status: "active" } },
      { provider: { id: "beta", name: "Beta", status: "active" } },
    ],
  });
  const page = render(<ProvidersPage search="" />);
  const alpha = await screen.findByRole("checkbox", { name: "选择 Alpha" });
  fireEvent.click(alpha);
  expect(
    screen
      .getByRole("checkbox", { name: "选择 Alpha" })
      .getAttribute("aria-checked"),
  ).toBe("true");
  page.rerender(<ProvidersPage search="Beta" />);
  expect(screen.queryByRole("checkbox", { name: "选择 Alpha" })).toBeNull();
  page.rerender(<ProvidersPage search="" />);
  expect(
    screen
      .getByRole("checkbox", { name: "选择 Alpha" })
      .getAttribute("aria-checked"),
  ).toBe("false");
});

it("保存连接后稍后验证，关闭弹窗仍能从列表继续", async () => {
  api.role = "platform_admin";
  api.delayed = true;
  api.providers.mockResolvedValueOnce({ items: [] }).mockResolvedValue({
    items: [
      {
        provider: {
          id: "connection",
          name: "待验证连接",
          status: "disabled",
        },
        model_count: 3,
      },
    ],
  });
  render(<ProvidersPage />);
  await screen.findByText("还没有连接");
  fireEvent.click(screen.getAllByRole("button", { name: "接入服务" })[0]);
  fireEvent.click(screen.getByRole("button", { name: "模拟保存完成" }));
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  await screen.findAllByRole("button", { name: "查看 待验证连接" });
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(api.detail).not.toHaveBeenCalled();
});

it("连接列表显示品牌名称和模型数，不将未启用连接称为已停用", async () => {
  api.role = "platform_admin";
  api.providers.mockResolvedValue({
    items: [
      {
        provider: {
          id: "connection",
          name: "我的连接",
          adapter_key: "volcengine_ark",
          region: "domestic",
          status: "disabled",
        },
        credential: { status: "active", last4: "1234" },
        model_count: 8,
      },
    ],
  });
  render(<ProvidersPage />);
  await screen.findByText("火山方舟");
  expect(screen.queryByText("volcengine_ark")).toBeNull();
  expect(screen.queryByText(/••••|1234/)).toBeNull();
  expect(screen.getByRole("columnheader", { name: "供应商" })).toBeTruthy();
  expect(screen.getByRole("columnheader", { name: "接入模型" })).toBeTruthy();
  expect(screen.queryByRole("columnheader", { name: "配置状态" })).toBeNull();
  expect(screen.queryByRole("columnheader", { name: "最近检查" })).toBeNull();
  fireEvent.pointerDown(screen.getByRole("button", { name: "显示列" }), {
    button: 0,
    ctrlKey: false,
  });
  fireEvent.click(
    await screen.findByRole("menuitemcheckbox", { name: "配置状态" }),
  );
  fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
  expect(screen.getByRole("columnheader", { name: "配置状态" })).toBeTruthy();
  expect(screen.getByText("未启用")).toBeTruthy();
  expect(screen.queryByText("已停用")).toBeNull();
});

it("鼠标打开连接弹窗后关闭仍返回原行入口", async () => {
  api.role = "platform_admin";
  api.providers.mockResolvedValue({
    items: [
      {
        provider: { id: "connection", name: "OpenAI", status: "disabled" },
        model_count: 1,
      },
    ],
  });
  render(<ProvidersPage />);
  const trigger = await screen.findByRole("button", { name: "管理 OpenAI" });
  fireEvent.click(trigger);
  await screen.findByText("连接详情内容");
  fireEvent.click(screen.getByRole("button", { name: "关闭对话框" }));
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "管理 OpenAI" }),
    ),
  );
});

it("批量删除只显示简短确认，结果说明历史保护", async () => {
  api.role = "platform_admin";
  const draft = {
    provider: {
      id: "draft",
      name: "草稿连接",
      status: "disabled",
      revision: 1,
    },
  };
  const historical = {
    provider: {
      id: "history",
      name: "历史连接",
      status: "disabled",
      revision: 1,
    },
  };
  api.providers.mockResolvedValue({ items: [draft, historical] });
  api.deletionPreview.mockImplementation(async ({ id }) =>
    id === "draft"
      ? {
          revision: 1,
          token: "preview-token",
          drafts: [{ name: "待配置模型" }],
        }
      : { revision: 1, blocked_reason: "task_history" },
  );
  api.deleteConnection.mockImplementation(async () => {
    api.providers.mockResolvedValue({ items: [historical] });
    return { id: "draft" };
  });

  render(<ProvidersPage />);
  fireEvent.click(
    await screen.findByRole("checkbox", { name: "选择当前页全部连接" }),
  );
  const toolbar = screen.getByRole("group", { name: "批量操作" });
  expect(within(toolbar).getByText("已选 2 项")).toBeTruthy();
  expect(within(toolbar).getAllByRole("button")).toHaveLength(2);
  fireEvent.click(within(toolbar).getByRole("button", { name: "删除连接" }));
  await screen.findByRole("button", { name: "删除 1 个连接" });
  const confirmation = screen.getByRole("dialog", { name: "删除连接？" });
  expect(within(confirmation).queryByRole("table")).toBeNull();
  expect(
    within(confirmation).queryByText(/待配置模型|历史连接|草稿连接/),
  ).toBeNull();
  expect(within(confirmation).getByText("1 项无法执行。")).toBeTruthy();
  expect(api.deleteConnection).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(api.deleteConnection).not.toHaveBeenCalled();

  fireEvent.click(within(toolbar).getByRole("button", { name: "删除连接" }));
  fireEvent.click(await screen.findByRole("button", { name: "删除 1 个连接" }));
  await screen.findByText("连接已删除");
  expect(
    screen.getByText("关联在途或历史任务，可在连接详情归档。"),
  ).toBeTruthy();
  expect(api.deleteConnection).toHaveBeenCalledTimes(1);
  expect(api.deleteConnection).toHaveBeenCalledWith(
    { id: "draft" },
    { token: "preview-token" },
    expect.objectContaining({
      headers: { "Idempotency-Key": expect.any(String) },
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "完成" }));
  expect(screen.queryByRole("checkbox", { name: "选择 草稿连接" })).toBeNull();
  expect(screen.getByRole("checkbox", { name: "选择 历史连接" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "查看批量操作结果" })).toBeTruthy();
});

it("删除成功后直接关闭确认弹窗并刷新列表", async () => {
  api.role = "platform_admin";
  api.providers.mockResolvedValue({
    items: [
      {
        provider: {
          id: "draft",
          name: "草稿连接",
          status: "disabled",
          revision: 1,
        },
      },
    ],
  });
  api.deletionPreview.mockResolvedValue({
    revision: 1,
    token: "preview-token",
  });
  api.deleteConnection.mockImplementation(async () => {
    api.providers.mockResolvedValue({ items: [] });
    return { id: "draft" };
  });

  render(<ProvidersPage />);
  fireEvent.click(
    await screen.findByRole("checkbox", { name: "选择 草稿连接" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "删除连接" }));
  fireEvent.click(await screen.findByRole("button", { name: "删除 1 个连接" }));
  await screen.findByText("还没有连接");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(api.deleteConnection).toHaveBeenCalledTimes(1);
});
