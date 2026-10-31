import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { CanvasNodeContextMenu } from "@/components/canvas/controls/canvas-context-menu";

function props() {
  return {
    menu: { type: "node" as const, nodeId: "a", x: 20, y: 30 },
    canCaptureVideoFrame: false,
    canUndo: true,
    canRedo: false,
    onClose: vi.fn(),
    onCopy: vi.fn(),
    onPaste: vi.fn(),
    onUndo: vi.fn(),
    onRedo: vi.fn(),
    onCreate: vi.fn(),
    onCaptureVideoFrame: vi.fn(),
    onDuplicate: vi.fn(),
    onDelete: vi.fn(),
  };
}
describe("canvas context actions", () => {
  it("copies without invoking duplicate and exposes history availability", () => {
    const p = props();
    render(<CanvasNodeContextMenu {...p} />);
    expect(screen.getByRole("menuitem", { name: /^复制副本/ })).toBeTruthy();
    expect(
      screen
        .getByRole("menuitem", { name: /^重做/ })
        .getAttribute("aria-disabled"),
    ).toBe("true");
    fireEvent.click(screen.getByRole("menuitem", { name: /^复制\s*⌘C/ }));
    expect(p.onCopy).toHaveBeenCalledOnce();
    expect(p.onDuplicate).not.toHaveBeenCalled();
  });
  it("offers creation and paste on the blank canvas without node actions", () => {
    const p = props();
    render(
      <CanvasNodeContextMenu {...p} menu={{ type: "canvas", x: 40, y: 50 }} />,
    );
    expect(screen.getByRole("menuitem", { name: "新建节点" })).toBeTruthy();
    expect(screen.queryByRole("menuitem", { name: /^删除/ })).toBeNull();
    fireEvent.click(screen.getByRole("menuitem", { name: /^粘贴/ }));
    expect(p.onPaste).toHaveBeenCalledOnce();
  });
  it("blocks mutations while frozen and leaves readable copy available", () => {
    const p = props();
    render(<CanvasNodeContextMenu {...p} frozen />);
    for (const name of [/^复制副本/, /^粘贴/, /^撤销/, /^删除/]) {
      const item = screen.getByRole("menuitem", { name });
      expect(item.getAttribute("aria-disabled")).toBe("true");
      fireEvent.click(item);
    }
    expect(p.onDuplicate).not.toHaveBeenCalled();
    expect(p.onDelete).not.toHaveBeenCalled();
    expect(
      screen
        .getByRole("menuitem", { name: /^复制\s*⌘C/ })
        .getAttribute("aria-disabled"),
    ).toBeNull();
  });
});
