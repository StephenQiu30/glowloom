import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import Models from "@/app/models/page";
import Providers from "@/app/providers/page";
import PlatformBilling from "@/app/platform/billing/page";

vi.mock("@/components/layout/page-layout", () => ({
  PageLayout: ({ children }: { children: React.ReactNode }) => children,
}));
vi.mock("@/components/ai-service/ai-service-admin", () => ({
  AiServiceAdmin: ({
    entry = "models",
    initialProvider,
  }: {
    entry?: string;
    initialProvider?: string;
  }) => <div>{initialProvider ? `${entry}:${initialProvider}` : entry}</div>,
}));
afterEach(cleanup);

it("opens each management entry at its own canonical route", async () => {
  render(<Models />);
  render(
    await Providers({ searchParams: Promise.resolve({ provider: "openai" }) }),
  );
  render(<PlatformBilling />);
  expect(screen.getByText("models")).toBeTruthy();
  expect(screen.getByText("providers:openai")).toBeTruthy();
  expect(screen.getByText("billing")).toBeTruthy();
});
