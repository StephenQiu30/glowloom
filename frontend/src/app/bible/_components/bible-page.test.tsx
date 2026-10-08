import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { BiblePage } from "./bible-page";
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
it("设定集未确认筛选与搜索共同作用", () => {
  render(<BiblePage />);
  const list = screen.getByRole("navigation", { name: "设定条目" });
  fireEvent.click(screen.getByRole("switch", { name: "只看未确认" }));
  expect(within(list).queryByRole("button", { name: /^林舟/ })).toBeNull();
  fireEvent.change(screen.getByRole("textbox", { name: "搜索名称或别名" }), {
    target: { value: "周警官" },
  });
  expect(within(list).getAllByRole("button")).toHaveLength(1);
  fireEvent.click(within(list).getByRole("button", { name: /周警官/ }));
  expect(
    within(list)
      .getByRole("button", { name: /周警官/ })
      .getAttribute("aria-current"),
  ).toBe("true");
});
