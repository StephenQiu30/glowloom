import { cleanup, screen } from "@testing-library/react";
import { afterEach, beforeEach, it, expect, vi } from "vitest";
import { render } from "../../../../tests/feedback";
import { ModelsPage } from "./models-page";
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
vi.mock("./model-detail", () => ({ ModelDetail: () => null }));
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
  expect(screen.getByLabelText("平台连接")).toHaveProperty(
    "value",
    "connection",
  );
  expect(
    screen.getByRole("link", { name: "查看全部模型" }).getAttribute("href"),
  ).toBe("/models");
  expect(api.models.mock.calls[0][0]).not.toHaveProperty("provider_id");
});
