import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ProductShell } from "./product-shell";
const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  replace: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
}));
vi.mock("next/navigation", () => ({ useRouter: () => mocks }));
vi.mock("sonner", () => ({
  toast: { success: mocks.success, error: mocks.error },
}));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            actor: {
              id: "11111111-1111-4111-8111-111111111111",
              login_name: "admin",
              display_name: "Admin",
              role: "admin",
              must_change_password: false,
              credential_revision: 1,
            },
            session_id: "22222222-2222-4222-8222-222222222222",
            last_active_at: new Date().toISOString(),
            absolute_expires_at: new Date(Date.now() + 86400000).toISOString(),
            persistent: false,
          }),
          { status: 200 },
        ),
      ),
    ),
  );
});
it("R6 主导航与画布入口指向真实路由，支持收起和展开", async () => {
  render(
    <ProductShell screen="home">
      <h1>创作工作区</h1>
    </ProductShell>,
  );
  expect(
    (await screen.findByRole("link", { name: "创作" })).getAttribute("href"),
  ).toBe("/");
  expect(screen.getByRole("link", { name: "资产" }).getAttribute("href")).toBe(
    "/assets",
  );
  expect(
    screen.getByRole("link", { name: "查看我的画布" }).getAttribute("href"),
  ).toBe("/projects");
  fireEvent.click(screen.getByRole("button", { name: "收起导航" }));
  expect(screen.getByRole("button", { name: "展开导航" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "展开导航" }));
  expect(screen.queryByRole("button", { name: "展开导航" })).toBeNull();
});

it("会话失效时不渲染工作台内容", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ code: "session_invalid" }), {
        status: 401,
      }),
    ),
  );
  render(
    <ProductShell screen="home">
      <p>私有工作台内容</p>
    </ProductShell>,
  );
  const { waitFor } = await import("@testing-library/react");
  await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith("/login"));
  expect(screen.queryByText("私有工作台内容")).toBeNull();
});
