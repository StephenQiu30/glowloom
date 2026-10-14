import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { HomePage } from "./home-page";
import { ProductShell } from "@/components/layout/product-shell";
import { TooltipProvider } from "@/components/ui/tooltip";
const router = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => router }));
beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.clearAllMocks();
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ code: "session_invalid" }), {
        status: 401,
      }),
    ),
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("游客可浏览首页，提交时打开登录弹窗，关闭和切换模式均保留创作草稿", async () => {
  render(
    <TooltipProvider>
      <HomePage />
    </TooltipProvider>,
  );
  const prompt = await screen.findByLabelText("创作提示词");
  expect(screen.getByRole("button", { name: "开始生成" })).toHaveProperty(
    "disabled",
    true,
  );
  fireEvent.change(prompt, { target: { value: "保留这份创作想法" } });
  fireEvent.click(screen.getByRole("button", { name: "开始生成" }));
  expect(screen.getByRole("dialog", { name: "登录织光" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "暂不登录，继续浏览" }));
  expect(
    (screen.getByLabelText("创作提示词") as HTMLTextAreaElement).value,
  ).toBe("保留这份创作想法");
  fireEvent.click(screen.getByRole("radio", { name: "画布" }));
  expect(
    (screen.getByLabelText("画布创作想法") as HTMLInputElement).value,
  ).toBe("保留这份创作想法");
  fireEvent.click(screen.getByRole("radio", { name: "生成" }));
  expect(
    (screen.getByLabelText("创作提示词") as HTMLTextAreaElement).value,
  ).toBe("保留这份创作想法");
  expect(router.replace).not.toHaveBeenCalled();
  expect(
    vi
      .mocked(fetch)
      .mock.calls.every(([path]) => String(path) === "/api/session"),
  ).toBe(true);
});
it("会话服务不可用时仍可浏览公共灵感，但资产页面不会因 allowGuest 暴露内容", async () => {
  vi.mocked(fetch).mockImplementation(
    async () =>
      new Response(JSON.stringify({ code: "unavailable" }), { status: 503 }),
  );
  const home = render(
    <TooltipProvider>
      <HomePage />
    </TooltipProvider>,
  );
  await screen.findByLabelText("创作提示词");
  expect(screen.getByRole("heading", { name: "最近上新" })).toBeTruthy();
  home.unmount();
  vi.mocked(fetch).mockImplementation(
    async () =>
      new Response(JSON.stringify({ code: "session_invalid" }), {
        status: 401,
      }),
  );
  render(
    <ProductShell screen="assets" allowGuest>
      <p>私有资产数据</p>
    </ProductShell>,
  );
  await waitFor(() => expect(router.replace).toHaveBeenCalledWith("/login"));
  expect(screen.queryByText("私有资产数据")).toBeNull();
});
