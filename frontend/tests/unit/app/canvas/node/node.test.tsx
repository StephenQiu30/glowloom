import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CanvasNode } from "@/app/canvas/node/node";
import { createDraftNode, createResourceNode } from "@/app/canvas/document";
const media = vi.hoisted(() => vi.fn());
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: (props: unknown) => {
    media(props);
    return <video aria-label="视频素材" controls />;
  },
}));
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
it("只保留内侧尺寸手柄，预览取消恢复，结束只提交一次", () => {
  const { value, resized, preview, card } = node();
  const corner = screen.getByRole("button", {
    name: `调整 ${value.title} 右下尺寸`,
  });
  expect(screen.getAllByRole("button", { name: /^调整/ })).toHaveLength(1);
  fireEvent.pointerDown(corner, { button: 0, clientX: 100, clientY: 100 });
  fireEvent.pointerMove(corner, { clientX: 120, clientY: 140 });
  expect(card.style.width).toBe("330px");
  expect(card.style.left).toBe("80px");
  expect(card.style.top).toBe("80px");
  expect(preview).toHaveBeenCalled();
  expect(resized).not.toHaveBeenCalled();
  fireEvent.pointerCancel(corner);
  expect(card.style.width).toBe("320px");
  expect(card.style.left).toBe("80px");
  fireEvent.pointerDown(corner, { button: 0, clientX: 100, clientY: 100 });
  fireEvent.pointerMove(corner, { clientX: 120, clientY: 140 });
  fireEvent.pointerUp(corner);
  fireEvent.lostPointerCapture(corner);
  expect(resized).toHaveBeenCalledExactlyOnceWith({
    x: 80,
    y: 80,
    width: 330,
    height: 340,
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
    width: 330,
    height: 320,
  });
  fireEvent.pointerDown(corner, { button: 0, clientX: 0, clientY: 0 });
  fireEvent.pointerMove(corner, { clientX: 100, clientY: 0, shiftKey: true });
  expect(
    parseFloat(card.style.width) /
      parseFloat(card.style.getPropertyValue("--canvas-node-height")),
  ).toBeCloseTo(1);
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
  expect(end).toHaveBeenCalledWith(
    true,
    expect.objectContaining({ pointerId: 1 }),
  );
});

it("未选中的视频也复用媒体预览，选中启用播放而不嵌套按钮", () => {
  const value = {
    ...createResourceNode("text", { x: 0, y: 0 }),
    node_type: "video",
    ref_type: "media_asset",
    ref_id: "asset",
    config: {},
  };
  const props = {
    node: value,
    projectID: "project",
    selected: false,
    zoom: 1,
    onResize: vi.fn(),
    onResizePreview: vi.fn(),
    onSelect: vi.fn(),
    onEditTitle: vi.fn(),
    onMove: vi.fn(),
    onDragStart: vi.fn(),
    onDragMove: vi.fn(),
    onDragEnd: vi.fn(),
  };
  const view = render(<CanvasNode {...props} />);
  expect(media).toHaveBeenLastCalledWith(
    expect.objectContaining({
      kind: "video",
      fit: "contain",
      interactive: false,
    }),
  );
  expect(screen.getByLabelText("视频素材").closest("button")).toBeNull();
  view.rerender(<CanvasNode {...props} selected />);
  expect(media).toHaveBeenLastCalledWith(
    expect.objectContaining({ interactive: true }),
  );
});
