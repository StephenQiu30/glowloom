import { useState, type ReactNode } from "react";
import {
  cleanup,
  fireEvent,
  render as renderUI,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ProductSearch } from "@/components/layout/product-search";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { TooltipProvider } from "@/components/ui/tooltip";

function render(children: ReactNode) {
  return renderUI(children, { wrapper: TooltipProvider });
}

const push = vi.hoisted(() => vi.fn());
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));
beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  Element.prototype.scrollIntoView = vi.fn();
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});
function SearchFixture({
  mobile = false,
  onSearch = vi.fn(),
}: {
  mobile?: boolean;
  onSearch?: (value: string) => void;
}) {
  const [value, setValue] = useState("");
  return (
    <ProductSearch
      isMobile={mobile}
      search={{ value, onValueChange: setValue, onSearch, label: "搜索画布" }}
    />
  );
}
async function openSearch() {
  fireEvent.click(screen.getByRole("button", { name: "搜索画布" }));
  return await screen.findByRole("combobox");
}
it.each([false, true])(
  "搜索入口只显示官方图标，保留可访问名称（手机=%s）",
  (mobile) => {
    render(<SearchFixture mobile={mobile} />);
    const trigger = screen.getByRole("button", { name: "搜索画布" });
    expect(trigger.textContent).toBe("");
    expect(trigger.querySelector("svg.lucide-search")).toBeTruthy();
    expect(trigger.getAttribute("aria-keyshortcuts")).toBe("Meta+K Control+K");
  },
);
it("官方 Command 输入不请求，选择搜索去除空白，清除恢复列表", async () => {
  const onSearch = vi.fn();
  render(<SearchFixture onSearch={onSearch} />);
  const input = await openSearch();
  fireEvent.change(input, { target: { value: "  真实画布  " } });
  expect(onSearch).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("option", { name: /搜索画布/ }));
  expect(onSearch).toHaveBeenLastCalledWith("真实画布");
  await openSearch();
  fireEvent.click(screen.getByRole("option", { name: "清除搜索" }));
  expect(onSearch).toHaveBeenLastCalledWith("");
});
it("手机弹窗提交后返回按钮焦点，再打开保留输入，Escape 关闭", async () => {
  const onSearch = vi.fn();
  render(<SearchFixture mobile onSearch={onSearch} />);
  const trigger = screen.getByRole("button", { name: "搜索画布" });
  trigger.focus();
  const input = await openSearch();
  await waitFor(() => expect(document.activeElement).toBe(input));
  fireEvent.change(input, { target: { value: "手机画布" } });
  fireEvent.keyDown(input, { key: "Enter" });
  await waitFor(() => expect(onSearch).toHaveBeenCalledWith("手机画布"));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  await openSearch();
  expect((screen.getByRole("combobox") as HTMLInputElement).value).toBe(
    "手机画布",
  );
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});
it.each(["metaKey", "ctrlKey"])(
  "%s + K 唤起；组合输入不提交，卸载后移除监听",
  async (modifier) => {
    const onSearch = vi.fn();
    const view = render(<SearchFixture onSearch={onSearch} />);
    fireEvent.keyDown(document, { key: "k", [modifier]: true });
    const input = await screen.findByRole("combobox");
    fireEvent.change(input, { target: { value: "中文" } });
    expect(fireEvent.keyDown(input, { key: "Enter", isComposing: true })).toBe(
      false,
    );
    expect(onSearch).not.toHaveBeenCalled();
    fireEvent.keyDown(input, { key: "k", [modifier]: true });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    view.unmount();
    expect(fireEvent.keyDown(document, { key: "k", [modifier]: true })).toBe(
      true,
    );
  },
);
it("其他弹窗打开或搜索禁用时不抢占快捷键", () => {
  const view = render(
    <>
      <SearchFixture />
      <Dialog open>
        <DialogContent>
          <DialogTitle>登录</DialogTitle>
        </DialogContent>
      </Dialog>
    </>,
  );
  fireEvent.keyDown(document, { key: "k", ctrlKey: true });
  expect(screen.getAllByRole("dialog")).toHaveLength(1);
  expect(screen.queryByRole("combobox")).toBeNull();
  view.rerender(
    <ProductSearch
      search={{
        value: "",
        onValueChange: vi.fn(),
        onSearch: vi.fn(),
        label: "搜索资产",
        disabled: true,
      }}
    />,
  );
  fireEvent.keyDown(document, { key: "k", ctrlKey: true });
  expect(screen.queryByRole("dialog")).toBeNull();
});
it("无页面搜索时转探索，安全编码中文关键词，并支持页面跳转", async () => {
  render(<ProductSearch />);
  fireEvent.click(screen.getByRole("button", { name: "搜索灵感" }));
  fireEvent.change(await screen.findByRole("combobox"), {
    target: { value: "蓝蝶 & 光" },
  });
  fireEvent.click(screen.getByRole("option", { name: /搜索灵感/ }));
  expect(push).toHaveBeenCalledWith(
    `/explore?q=${encodeURIComponent("蓝蝶 & 光")}`,
  );
});
