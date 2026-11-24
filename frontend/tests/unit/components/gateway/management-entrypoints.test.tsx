import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import Models from "@/app/models/page";
import Providers from "@/app/providers/page";
import { ModelsPage } from "@/app/models/_components/models-page";
import { ProvidersPage } from "@/app/providers/_components/providers-page";

const { redirect } = vi.hoisted(() => ({ redirect: vi.fn() }));
vi.mock("next/navigation", () => ({ redirect }));
vi.mock("@/components/layout/page-layout", () => ({
  PageLayout: ({ children }: { children: React.ReactNode }) => children,
}));
vi.mock("@/components/gateway/gateway-admin", () => ({
  GatewayAdmin: ({ initialTab }: { initialTab?: string }) => (
    <div>{initialTab ?? "当前统一网关管理"}</div>
  ),
}));
afterEach(cleanup);

it("preserves links to the former provider list and selects its channel tab", async () => {
  Providers();
  expect(redirect).toHaveBeenCalledWith("/models?tab=routes");
  render(await Models({ searchParams: Promise.resolve({ tab: "routes" }) }));
  expect(screen.getByText("routes")).toBeTruthy();
});

it.each([ModelsPage, ProvidersPage])("uses the shared admin", (Page) => {
  render(<Page />);
  expect(screen.getByText("当前统一网关管理")).toBeTruthy();
});
