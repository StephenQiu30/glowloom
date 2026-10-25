import { cleanup, screen, fireEvent } from "@testing-library/react";
import { afterEach, beforeEach, it, expect, vi } from "vitest";
import { render } from "../../../../feedback";
import { ModelsPage } from "@/app/models/_components/models-page";
const api = vi.hoisted(() => ({ models: vi.fn(), providers: vi.fn() }));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({
    actor: { id: "admin", platform_role: "platform_admin" },
  }),
}));
vi.mock("@/api/settings", () => ({
  listAdminModels: api.models,
  listAdminProviders: api.providers,
  listAdminCapabilities: async () => [],
  createAdminModel: vi.fn(),
  getConfigReceipt: vi.fn(),
}));
vi.mock("@/app/models/_components/model-detail", () => ({
  ModelDetail: () => null,
}));
beforeEach(() => {
  localStorage.clear();
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
it("连接详情跳转后预选连接，只展示该连接的已加载模型并保留全部入口", async () => {
  render(<ModelsPage initialProvider="connection" />);
  await screen.findByRole("button", { name: /当前连接模型/ });
  expect(screen.queryByRole("button", { name: /其他连接模型/ })).toBeNull();
  expect(
    screen.getByRole("link", { name: "查看全部模型" }).getAttribute("href"),
  ).toBe("/models");
  fireEvent.click(screen.getByRole("button", { name: "添加模型" }));
  expect(screen.getByLabelText("平台连接")).toHaveProperty(
    "value",
    "connection",
  );
  expect(api.models.mock.calls[0][0]).not.toHaveProperty("provider_id");
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
      },
    ],
  });
  render(<ModelsPage />);
  await screen.findByText("未配置模型");
  expect(screen.getByText("草稿")).toBeTruthy();
  expect(screen.getByText("已停用")).toBeTruthy();
});
