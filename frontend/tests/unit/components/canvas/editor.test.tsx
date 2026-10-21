import { fireEvent, render, screen, cleanup } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { CanvasEditor } from "@/components/canvas/editor";
afterEach(cleanup);
vi.stubGlobal(
  "ResizeObserver",
  class {
    observe() {}
    disconnect() {}
  },
);
it("文本编辑、复制、撤销与重做在独立画布内工作", () => {
  render(<CanvasEditor />);
  fireEvent.click(screen.getByRole("button", { name: "添加文本" }));
  const node = screen.getByRole("group", { name: "节点：文本" });
  fireEvent.doubleClick(node);
  const input = screen.getByRole("textbox", { name: "文本内容" });
  fireEvent.change(input, { target: { value: "hello canvas" } });
  fireEvent.blur(input);
  expect(screen.getByText("hello canvas")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "复制选中节点" }));
  expect(screen.getAllByText("hello canvas")).toHaveLength(2);
  fireEvent.click(screen.getByRole("button", { name: "撤销" }));
  expect(screen.getAllByText("hello canvas")).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "重做" }));
  expect(screen.getAllByText("hello canvas")).toHaveLength(2);
  fireEvent.keyDown(screen.getByLabelText("独立画布编辑器"), {
    key: "a",
    ctrlKey: true,
  });
  fireEvent.click(screen.getByRole("button", { name: "删除选中内容" }));
  expect(screen.queryByText("hello canvas")).toBeNull();
});
