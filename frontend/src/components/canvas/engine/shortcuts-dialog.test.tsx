import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { CanvasShortcutsDialog } from "./shortcuts-dialog";

afterEach(cleanup);
it("快捷键帮助通过真实Dialog展示并以Escape关闭", () => {
  const onOpenChange = vi.fn();
  render(<CanvasShortcutsDialog open onOpenChange={onOpenChange} />);
  const dialog = screen.getByRole("dialog", { name: "画布快捷键" });
  expect(screen.getByText("100% 缩放")).toBeDefined();
  expect(screen.getByText("适应全画布")).toBeDefined();
  expect(screen.getByText(/按钮上的 Tab/)).toBeDefined();
  fireEvent.keyDown(dialog, { key: "Escape" });
  expect(onOpenChange).toHaveBeenCalledExactlyOnceWith(false);
});
