import { useState } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ProductSearch } from "@/components/layout/product-search";
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});
function Fixture({ onSearch }: { onSearch: (value: string) => void }) {
  const [value, onValueChange] = useState("");
  return (
    <ProductSearch
      search={{
        mode: "inline",
        value,
        onValueChange,
        onSearch,
        label: "搜索主体",
      }}
    />
  );
}
it("内联搜索防抖，清空即时应用，无跳转弹窗", () => {
  vi.useFakeTimers();
  const apply = vi.fn();
  render(<Fixture onSearch={apply} />);
  fireEvent.click(screen.getByRole("button", { name: "搜索主体" }));
  const input = screen.getByRole("textbox", { name: "搜索主体" });
  fireEvent.change(input, { target: { value: "海" } });
  act(() => vi.advanceTimersByTime(150));
  fireEvent.change(input, { target: { value: "海边" } });
  act(() => vi.advanceTimersByTime(299));
  expect(apply).not.toHaveBeenCalled();
  act(() => vi.advanceTimersByTime(1));
  expect(apply).toHaveBeenLastCalledWith("海边");
  fireEvent.click(screen.getByRole("button", { name: "清空搜索" }));
  expect(apply).toHaveBeenLastCalledWith("");
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.queryByText("跳转")).toBeNull();
});
it("组合输入结束前不发查询，卸载取消尚未提交的输入", () => {
  vi.useFakeTimers();
  const apply = vi.fn();
  const view = render(<Fixture onSearch={apply} />);
  fireEvent.click(screen.getByRole("button", { name: "搜索主体" }));
  const input = screen.getByRole("textbox", { name: "搜索主体" });
  fireEvent.compositionStart(input);
  fireEvent.change(input, { target: { value: "xiang" } });
  act(() => vi.advanceTimersByTime(500));
  expect(apply).not.toHaveBeenCalled();
  fireEvent.change(input, { target: { value: "香氛" } });
  fireEvent.compositionEnd(input);
  act(() => vi.advanceTimersByTime(300));
  expect(apply).toHaveBeenLastCalledWith("香氛");
  fireEvent.change(input, { target: { value: "未提交" } });
  view.unmount();
  act(() => vi.advanceTimersByTime(300));
  expect(apply).toHaveBeenCalledTimes(1);
});
