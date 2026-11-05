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
it("没有可验证模型时汇总待审核数量", async () => {
  api.models.mockResolvedValue([]);
  render(
    <SWRConfig value={{ provider: () => new Map() }}>
      <ConnectionModels id="provider" revision={1} onSaved={vi.fn()} />
    </SWRConfig>,
  );
  expect(
    await screen.findByText("暂无可验证模型 · 1 个模型价格待审核"),
  ).toBeTruthy();
  expect(screen.queryByText("待审候选模型")).toBeNull();
});

it("已有可验证模型时不逐项展示待审核模型", async () => {
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
  expect(await screen.findByText("1 个模型价格待审核")).toBeTruthy();
  expect(screen.queryByText("待审候选模型")).toBeNull();
});
