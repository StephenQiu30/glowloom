import { cleanup, screen, fireEvent, waitFor } from "@testing-library/react";
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
  ModelDetail: () => null,
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
  await screen.findByRole("button", { name: /当前连接模型/ });
  expect(screen.getByRole("cell", { name: "OpenAI" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: /其他连接模型/ })).toBeNull();
  expect(
    screen.getByRole("link", { name: "查看全部模型" }).getAttribute("href"),
  ).toBe("/models");
  fireEvent.click(screen.getByRole("button", { name: "添加模型" }));
  expect(screen.getByRole("combobox", { name: "供应商" }).textContent).toBe(
    "OpenAI",
  );
  expect(api.models.mock.calls[0][0]).toHaveProperty(
    "provider_id",
    "connection",
  );
});

it("按真实版本区分未配置草稿与已有版本的停用模型", async () => {
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
  expect(screen.getByText("待配置")).toBeTruthy();
  expect(screen.getByText("已停用")).toBeTruthy();
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
