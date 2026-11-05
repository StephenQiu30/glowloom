import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SWRConfig } from "swr";
import { ConnectionModels } from "@/app/providers/_components/connection-models";
const api = vi.hoisted(() => ({ models: vi.fn() }));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ actor: { id: "price-review-admin" } }),
}));
vi.mock("@/api/settings", () => ({
  listProviderTestModels: api.models,
  getProviderOnboarding: async () => ({
    provider_id: "provider",
    models: [
      {
        id: "candidate",
        display_name: "待审候选模型",
        release_id: "00000000-0000-0000-0000-000000000000",
        status: "disabled",
      },
    ],
  }),
}));
vi.mock("@/app/providers/_components/connection-onboarding", () => ({
  ConnectionOnboarding: () => <p>已定价模型验证</p>,
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it("无价格发布的已保存候选仍可见并提示从模型管理补审", async () => {
  api.models.mockResolvedValue([]);
  render(
    <SWRConfig value={{ provider: () => new Map() }}>
      <ConnectionModels id="provider" revision={1} onSaved={vi.fn()} />
    </SWRConfig>,
  );
  await screen.findByText(/待审候选模型/);
  expect(screen.getByText(/价格待审核，尚不可创作/)).toBeTruthy();
  expect(screen.getByText("请先在模型管理中完成发布配置。")).toBeTruthy();
});

it("部分模型已有发布时仍显示其余候选的待审价格", async () => {
  api.models.mockResolvedValue([
    {
      id: "priced",
      display_name: "已定价模型",
      release_id: "release",
      requires_trial: false,
    },
  ]);
  render(
    <SWRConfig value={{ provider: () => new Map() }}>
      <ConnectionModels id="provider" revision={2} onSaved={vi.fn()} />
    </SWRConfig>,
  );
  await screen.findByText("已定价模型验证");
  expect(await screen.findByText(/待审候选模型/)).toBeTruthy();
  expect(screen.getByText(/价格待审核，尚不可创作/)).toBeTruthy();
});
