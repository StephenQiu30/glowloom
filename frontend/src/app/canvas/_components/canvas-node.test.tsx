import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CanvasNode } from "./canvas-node";
import { createDraftNode } from "./editor-state";
beforeEach(() => {
  vi.stubGlobal(
    "PointerEvent",
    class extends MouseEvent {
      pointerId = 1;
    },
  );
  HTMLElement.prototype.setPointerCapture = vi.fn();
  HTMLElement.prototype.hasPointerCapture = vi.fn(() => true);
  HTMLElement.prototype.releasePointerCapture = vi.fn();
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
function node() {
  const value = createDraftNode("image_generation", "1:1", 0),
    resized = vi.fn(),
    preview = vi.fn();
  render(
    <CanvasNode
      node={value}
      selected
      zoom={2}
      onResize={resized}
      onResizePreview={preview}
      onSelect={vi.fn()}
      onEditTitle={vi.fn()}
      onMove={vi.fn()}
      onDragStart={vi.fn()}
      onDragMove={vi.fn()}
      onDragEnd={vi.fn()}
    />,
  );
  return {
    value,
    resized,
    preview,
    card: document.querySelector<HTMLElement>("[data-node-id]")!,
  };
}
it("四角尺寸预览使用世界坐标，取消恢复，结束只提交一次", () => {
  const { value, resized, preview, card } = node();
  const corner = screen.getByRole("button", {
    name: `调整 ${value.title} 左上尺寸`,
  });
  fireEvent.pointerDown(corner, { button: 0, clientX: 100, clientY: 100 });
  fireEvent.pointerMove(corner, { clientX: 120, clientY: 140 });
  expect(card.style.width).toBe("278px");
  expect(card.style.left).toBe("90px");
  expect(card.style.top).toBe("100px");
  expect(preview).toHaveBeenCalled();
  expect(resized).not.toHaveBeenCalled();
  fireEvent.pointerCancel(corner);
  expect(card.style.width).toBe("288px");
  expect(card.style.left).toBe("80px");
  fireEvent.pointerDown(corner, { button: 0, clientX: 100, clientY: 100 });
  fireEvent.pointerMove(corner, { clientX: 120, clientY: 140 });
  fireEvent.pointerUp(corner);
  fireEvent.lostPointerCapture(corner);
  expect(resized).toHaveBeenCalledExactlyOnceWith({
    x: 90,
    y: 100,
    width: 278,
    height: 180,
  });
});
it("手柄提供键盘等价操作，Shift指针调整保持比例", () => {
  const { value, resized, card } = node();
  const corner = screen.getByRole("button", {
    name: `调整 ${value.title} 右下尺寸`,
  });
  fireEvent.keyDown(corner, { key: "ArrowRight", shiftKey: true });
  expect(resized).toHaveBeenCalledWith({
    x: 80,
    y: 80,
    width: 298,
    height: 200,
  });
  fireEvent.pointerDown(corner, { button: 0, clientX: 0, clientY: 0 });
  fireEvent.pointerMove(corner, { clientX: 100, clientY: 0, shiftKey: true });
  expect(
    parseFloat(card.style.width) /
      parseFloat(card.style.getPropertyValue("--canvas-node-height")),
  ).toBeCloseTo(288 / 200);
});

it("节点媒体区域与标题一样支持拖动和取消", () => {
  const value = createDraftNode("video_generation", "1:1", 0),
    start = vi.fn(),
    move = vi.fn(),
    end = vi.fn();
  render(
    <CanvasNode
      node={value}
      selected
      zoom={1}
      onResize={vi.fn()}
      onResizePreview={vi.fn()}
      onSelect={vi.fn()}
      onEditTitle={vi.fn()}
      onMove={vi.fn()}
      onDragStart={start}
      onDragMove={move}
      onDragEnd={end}
    />,
  );
  const body = screen.getByRole("button", { name: `选择 ${value.title}` });
  fireEvent.pointerDown(body, { button: 0 });
  fireEvent.pointerMove(body, { clientX: 100 });
  fireEvent.pointerCancel(body);
  expect(start).toHaveBeenCalledTimes(1);
  expect(move).toHaveBeenCalledTimes(1);
  expect(end).toHaveBeenCalledWith(true);
});
