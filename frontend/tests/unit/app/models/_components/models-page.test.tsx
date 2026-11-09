import {
  cleanup,
  screen,
  fireEvent,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, it, expect, vi } from "vitest";
import { render } from "../../../../feedback";
import { ModelsPage } from "@/app/models/_components/models-page";
const api = vi.hoisted(() => ({
  models: vi.fn(),
  providers: vi.fn(),
  config: vi.fn(),
  key: vi.fn(),
  add: vi.fn(),
  probe: vi.fn(),
  check: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({
    actor: { id: "admin", platform_role: "platform_admin" },
  }),
}));
vi.mock("@/api/settings", () => ({
  listAdminModels: api.models,
  listAdminProviders: api.providers,
  listAdminCapabilities: async () => [],
  getProviderConfig: api.config,
  setAdminCredential: api.key,
  addProviderPresetModel: api.add,
  getConfigReceipt: vi.fn(),
  readSavedProviderModelDirectory: api.probe,
  getProviderCheck: api.check,
}));
vi.mock("@/app/models/_components/model-detail", () => ({
  ModelDetail: ({ initialView }: { initialView: string }) => (
    <div>{initialView}</div>
  ),
}));
beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  api.config.mockResolvedValue({
    id: "config",
    config: { capabilities: ["text"] },
    model_catalog: {
      version: "catalog",
      models: [
        {
          key: "preset",
          display_name: "预设模型",
          capability: "text.generate",
          provider_model_id: "test-preset",
        },
        {
          key: "placeholder",
          display_name: "账号专属模型",
          requires_model_selection: true,
          capability: "text.generate",
        },
      ],
    },
  });
  api.key.mockResolvedValue({
    id: "credential",
    provider_id: "connection",
    last4: "test",
    status: "active",
  });
  api.add.mockResolvedValue({ id: "created", provider_id: "connection" });
  api.models.mockResolvedValue({
    items: [
      { id: "a", provider_id: "connection", display_name: "当前连接模型" },
      { id: "b", provider_id: "other", display_name: "其他连接模型" },
    ],
  });
  api.providers.mockResolvedValue({
    items: [
      {
        provider: {
          id: "connection",
          name: "目标连接",
          adapter_key: "openrouter",
        },
      },
    ],
  });
});
afterEach(cleanup);
it("连接详情跳转后预选连接，由服务端筛选模型并保留全部入口", async () => {
  api.models.mockResolvedValue({
    items: [
      { id: "a", provider_id: "connection", display_name: "当前连接模型" },
    ],
  });
  api.providers.mockResolvedValue({
    items: [
      {
        provider: {
          id: "connection",
          name: "OpenAI · 9f26c41a",
          adapter_key: "openai",
        },
      },
      {
        provider: {
          id: "other",
          name: "OpenAI · 88c8adae",
          adapter_key: "openai",
        },
      },
    ],
  });
  render(<ModelsPage initialProvider="connection" />);
  await screen.findByRole("button", { name: "查看 当前连接模型" });
  expect(
    screen.getByRole("link", { name: "OpenAI" }).getAttribute("href"),
  ).toBe("/providers?connection=connection");
  expect(screen.queryByRole("button", { name: /其他连接模型/ })).toBeNull();
  expect(
    screen.getByRole("link", { name: "查看全部模型" }).getAttribute("href"),
  ).toBe("/models");
  fireEvent.click(screen.getByRole("button", { name: "添加模型" }));
  const dialog = screen.getByRole("dialog", { name: "添加模型" });
  expect(dialog.textContent).toContain("OpenAI");
  expect(screen.queryByRole("combobox", { name: "供应商" })).toBeNull();
  expect(api.models.mock.calls[0][0]).toHaveProperty(
    "provider_id",
    "connection",
  );
});

it("配置、验证与停用的详细状态在列表统一为不可用", async () => {
  api.models.mockResolvedValue({
    items: [
      {
        id: "draft",
        display_name: "未配置模型",
        status: "disabled",
        current_version_id: "00000000-0000-0000-0000-000000000000",
      },
      {
        id: "disabled",
        display_name: "历史版本模型",
        status: "disabled",
        current_version_id: "version",
        has_publication: true,
      },
    ],
  });
  render(<ModelsPage />);
  await screen.findByText("未配置模型");
  const table = within(screen.getByRole("table", { name: "模型列表" }));
  expect(table.getAllByText("不可用")).toHaveLength(2);
  expect(table.queryByText("已停用")).toBeNull();
});

it.each([true, false])(
  "添加模型只提交预设并按需写入密钥（已有凭据 %s）",
  async (hasKey) => {
    if (hasKey) {
      api.providers.mockResolvedValue({
        items: [
          {
            provider: {
              id: "connection",
              name: "目标连接",
              adapter_key: "openrouter",
            },
            credential: { id: "existing-key", status: "active" },
          },
        ],
      });
    }
    render(<ModelsPage initialProvider="connection" />);
    fireEvent.click(await screen.findByRole("button", { name: "添加模型" }));
    await screen.findByRole("checkbox", { name: /账号专属模型/ });
    fireEvent.click(screen.getByRole("checkbox", { name: /预设模型/ }));
    expect(screen.queryByText("稳定标识")).toBeNull();
    expect(screen.queryByText("生成能力")).toBeNull();
    if (hasKey) expect(screen.queryByLabelText("API Key")).toBeNull();
    else
      fireEvent.change(screen.getByLabelText("API Key"), {
        target: { value: "synthetic-key" },
      });
    fireEvent.click(screen.getByRole("button", { name: "保存模型" }));
    await waitFor(() => expect(api.add).toHaveBeenCalledOnce());
    expect(api.add).toHaveBeenCalledWith(
      { id: "connection" },
      {
        candidate_key: "preset",
        config_id: "config",
        credential_id: hasKey ? "existing-key" : "credential",
        draft_only: true,
        catalog_version: "catalog",
        request_id: expect.any(String),
      },
    );
    if (hasKey) expect(api.key).not.toHaveBeenCalled();
    else {
      expect(api.key).toHaveBeenCalledExactlyOnceWith(
        { id: "connection" },
        {
          label: "平台统一 Key",
          secret: { api_key: "synthetic-key" },
        },
        expect.anything(),
      );
      expect(api.key.mock.invocationCallOrder[0]).toBeLessThan(
        api.add.mock.invocationCallOrder[0],
      );
    }
  },
);

it("读取只读目录后能搜索并添加候选模型", async () => {
  api.providers.mockResolvedValue({
    items: [
      {
        provider: {
          id: "connection",
          name: "目标连接",
          adapter_key: "openrouter",
        },
        credential: { id: "directory-key", status: "active" },
      },
    ],
  });
  let finishDirectory!: (result: API.CredentialTestAccepted) => void;
  api.probe.mockReturnValue(
    new Promise<API.CredentialTestAccepted>((resolve) => {
      finishDirectory = resolve;
    }),
  );
  api.check.mockResolvedValue({
    id: "test-check",
    kind: "discovery",
    current: true,
    config_version_id: "config",
    credential_id: "directory-key",
    status: "succeeded",
    discovery_result: "ok",
    candidates: [{ provider_model_id: "fixture/model" }],
  });
  render(<ModelsPage initialProvider="connection" openAdd />);
  expect(await screen.findByRole("dialog", { name: "添加模型" })).toBeTruthy();
  const read = await screen.findByRole("button", { name: "读取在线目录" });
  expect(api.probe).not.toHaveBeenCalled();
  await waitFor(() => expect(read).toHaveProperty("disabled", false));
  fireEvent.click(read);
  expect(await screen.findByText("正在读取模型目录…")).toBeTruthy();
  expect(screen.queryByText(/待确认请求/)).toBeNull();
  expect(
    screen
      .getByRole("button", { name: "保存模型" })
      .querySelector("[role=status]"),
  ).toBeNull();
  finishDirectory({
    accepted: true,
    provider_id: "connection",
    check_id: "test-check",
    kind: "discovery",
  });
  await screen.findByText(/目录返回 1 个候选/);
  fireEvent.change(screen.getByRole("textbox", { name: "搜索模型" }), {
    target: { value: "fixture" },
  });
  fireEvent.click(screen.getByRole("button", { name: /fixture\/model/ }));
  fireEvent.click(screen.getByRole("button", { name: "保存模型" }));
  await waitFor(() =>
    expect(api.add).toHaveBeenCalledWith(
      { id: "connection" },
      expect.objectContaining({
        custom_model_id: "fixture/model",
        directory_check_id: "test-check",
        capability: "text.generate",
      }),
    ),
  );
  expect(api.key).not.toHaveBeenCalled();
});

it("行操作可直接进入发布页，关闭后回到管理按钮", async () => {
  render(<ModelsPage />);
  const trigger = await screen.findByRole("button", {
    name: "管理 当前连接模型",
  });
  fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
  fireEvent.click(await screen.findByRole("menuitem", { name: "发布" }));
  await screen.findByText("release");
  fireEvent.click(screen.getByRole("button", { name: "关闭对话框" }));
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "管理 当前连接模型" }),
    ),
  );
});

it("组合筛选回到第一页并清除选择，重置恢复默认查询", async () => {
  api.models
    .mockResolvedValueOnce({
      items: [{ id: "a", display_name: "第一页模型" }],
      next_cursor: "page-2",
    })
    .mockResolvedValueOnce({
      items: [{ id: "b", display_name: "第二页模型" }],
    })
    .mockResolvedValueOnce({ items: [] });
  render(<ModelsPage />);
  await screen.findByText("第一页模型");
  fireEvent.click(screen.getByRole("button", { name: "下一页" }));
  await screen.findByText("第二页模型");
  fireEvent.click(screen.getByRole("checkbox", { name: "选择 第二页模型" }));
  expect(screen.getByRole("button", { name: "停用模型" })).toBeTruthy();

  const filter = within(screen.getByRole("form", { name: "模型筛选" }));
  fireEvent.change(filter.getByLabelText("模型名称"), {
    target: { value: "  Seed  " },
  });
  fireEvent.change(filter.getByLabelText("服务连接"), {
    target: { value: "connection" },
  });
  fireEvent.change(filter.getByLabelText("状态"), {
    target: { value: "unavailable" },
  });
  expect(api.models).toHaveBeenCalledTimes(2);
  fireEvent.click(filter.getByRole("button", { name: "筛选" }));

  await screen.findByText("没有匹配的模型");
  expect(api.models.mock.lastCall?.[0]).toEqual({
    limit: 20,
    cursor: undefined,
    name: "Seed",
    provider_id: "connection",
    status: "unavailable",
  });
  expect(screen.queryByRole("button", { name: "停用模型" })).toBeNull();

  fireEvent.click(filter.getByRole("button", { name: "重置" }));
  await screen.findByText("当前连接模型");
  expect(api.models.mock.lastCall?.[0]).toEqual({
    limit: 20,
    cursor: undefined,
    provider_id: undefined,
  });
  expect((filter.getByLabelText("模型名称") as HTMLInputElement).value).toBe(
    "",
  );
});

it("计价审核使用服务端筛选并在表格下打开价格页", async () => {
  api.models.mockResolvedValue({
    items: [
      {
        id: "a",
        provider_id: "connection",
        display_name: "计价样本",
        billing: {
          unit: "per_image",
          procurement_ready: true,
          sale_ready: true,
          sale_published: false,
          status: "usage_pending",
        },
      },
    ],
  });
  render(<ModelsPage />);
  await screen.findByRole("button", { name: "查看 计价样本" });
  fireEvent.click(screen.getByRole("button", { name: "计价审核" }));
  await screen.findByRole("button", { name: "审核计价 计价样本" });
  const table = within(screen.getByRole("table", { name: "预设计价列表" }));
  expect(table.getByText("待验证或发布")).toBeTruthy();
  expect(table.queryByText("可计费")).toBeNull();
  const filters = within(screen.getByRole("form", { name: "计价筛选" }));
  fireEvent.change(filters.getByLabelText("计价状态"), {
    target: { value: "usage_pending" },
  });
  fireEvent.click(filters.getByRole("button", { name: "筛选" }));
  await waitFor(() =>
    expect(api.models.mock.lastCall?.[0]).toMatchObject({
      pricing_review: true,
      billing_status: "usage_pending",
    }),
  );
  const review = await screen.findByRole("button", {
    name: "审核计价 计价样本",
  });
  fireEvent.click(review);
  expect(
    within(screen.getByRole("region", { name: "审核价目" })).getByText(
      "pricing",
    ),
  ).toBeTruthy();
  expect(screen.queryByRole("dialog", { name: "模型详情" })).toBeNull();
});
