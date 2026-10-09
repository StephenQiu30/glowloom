import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ModelsPage } from "./models-page";
const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: mocks.push }) }));
vi.mock("sonner", () => ({
  toast: { success: mocks.success, error: mocks.error },
}));
vi.mock("@/hooks/use-mobile", () => ({ useIsMobile: () => false }));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
beforeEach(() => vi.clearAllMocks());
it("模型运动幅度支持键盘调整，参数预览与 JSON 保持一致", () => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  render(<ModelsPage />);
  const slider = screen.getByRole("slider", { name: "运动幅度" });
  fireEvent.keyDown(slider, { key: "ArrowRight" });
  fireEvent.click(screen.getByRole("switch", { name: "固定镜头" }));
  fireEvent.mouseDown(screen.getByRole("tab", { name: "JSON" }), {
    button: 0,
    ctrlKey: false,
  });
  const preview = screen.getByRole("tabpanel", { name: "JSON" });
  expect(preview.textContent).toContain('"motionAmount": 61');
  expect(preview.textContent).toContain('"fixedCamera": true');
});

// 本测试仅覆盖页面交互；真实身份守卫由product-shell.test.tsx与浏览器核验。
vi.mock("@/components/layout/product-shell", () => ({
  ProductShell: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
