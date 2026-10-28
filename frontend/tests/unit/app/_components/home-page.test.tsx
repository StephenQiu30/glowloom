import { ApplicationLayout } from "@/components/layout/application-layout";
import { SessionProvider } from "@/components/auth/session";
import {
  cleanup,
  fireEvent,
  render as renderUI,
  screen,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { HomePage } from "@/app/_components/home-page";
import { request, RequestError } from "@/lib/request";
vi.mock("@/lib/request", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/request")>();
  return { ...actual, request: vi.fn() };
});
import { ReferenceAssetsPage as AssetsPage } from "@/app/assets/_components/assets-page";
import { TooltipProvider } from "@/components/ui/tooltip";
const router = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }));
vi.mock("next/navigation", () => ({
  usePathname: () => "/",
  useRouter: () => router,
}));
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
  vi.mocked(request)
    .mockReset()
    .mockRejectedValue(new RequestError("session_invalid", 401));
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
  fireEvent.paste(prompt, {
    clipboardData: { files: [], getData: () => "保留这份创作想法" },
  });
  fireEvent.click(screen.getByRole("button", { name: "开始生成" }));
  expect(screen.getByRole("dialog", { name: "登录织光" })).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: "暂不登录，继续浏览" }),
  ).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "关闭登录注册弹窗" }));
  expect(screen.getByLabelText("创作提示词").textContent).toBe(
    "保留这份创作想法",
  );
  fireEvent.click(screen.getByRole("radio", { name: "画布" }));
  expect(screen.getByLabelText("画布创作想法").textContent).toBe(
    "保留这份创作想法",
  );
  fireEvent.click(screen.getByRole("radio", { name: "生成" }));
  expect(screen.getByLabelText("创作提示词").textContent).toBe(
    "保留这份创作想法",
  );
  expect(router.replace).not.toHaveBeenCalled();
  expect(
    vi
      .mocked(request)
      .mock.calls.every(([path]) => String(path) === "/api/session"),
  ).toBe(true);
});
it("会话服务不可用时仍可浏览公共灵感，未登录的素材库只展示登录引导且不读取私人素材", async () => {
  vi.mocked(request).mockRejectedValue(new RequestError("unavailable", 503));
  const home = render(
    <TooltipProvider>
      <HomePage />
    </TooltipProvider>,
  );
  await screen.findByLabelText("创作提示词");
  expect(screen.getByRole("heading", { name: "创作精选" })).toBeTruthy();
  home.unmount();
  vi.mocked(request).mockRejectedValue(
    new RequestError("session_invalid", 401),
  );
  render(
    <TooltipProvider>
      <AssetsPage />
    </TooltipProvider>,
  );
  await screen.findByText("登录查看你的素材");
  expect(router.replace).not.toHaveBeenCalled();
  expect(
    vi
      .mocked(request)
      .mock.calls.every(([path]) => String(path) === "/api/session"),
  ).toBe(true);
});

function render(ui: React.ReactNode) {
  return renderUI(
    <TooltipProvider>
      <ApplicationLayout>{ui}</ApplicationLayout>
    </TooltipProvider>,
    {
      wrapper: SessionProvider,
    },
  );
}
