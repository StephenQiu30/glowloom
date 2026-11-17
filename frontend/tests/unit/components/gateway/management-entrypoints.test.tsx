import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import Models from "@/app/models/page";
import Providers from "@/app/providers/page";
import { ModelsPage } from "@/app/models/_components/models-page";
import { ProvidersPage } from "@/app/providers/_components/providers-page";
vi.mock("@/components/layout/page-layout", () => ({
  PageLayout: ({ children }: { children: React.ReactNode }) => children,
}));
vi.mock("@/components/gateway/gateway-admin", () => ({
  GatewayAdmin: () => <div>当前统一网关管理</div>,
}));
afterEach(cleanup);
it.each([Models, Providers, ModelsPage, ProvidersPage])(
  "routes existing management entrypoints to the rebuilt admin",
  (Page) => {
    render(<Page />);
    expect(screen.getByText("当前统一网关管理")).toBeTruthy();
  },
);
