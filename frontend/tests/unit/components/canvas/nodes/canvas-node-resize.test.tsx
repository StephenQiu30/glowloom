import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ComponentProps } from "react";
import { CanvasNode } from "@/components/canvas/nodes/canvas-node";
import { CanvasNodeType } from "@/components/canvas/types/canvas";

vi.mock("@/components/canvas/runtime/store/use-theme-store", () => ({
  useThemeStore: (select: (state: { theme: "dark" }) => unknown) =>
    select({ theme: "dark" }),
}));
afterEach(cleanup);
function props(): ComponentProps<typeof CanvasNode> {
  return {
    data: {
      id: "resize-node",
      type: CanvasNodeType.Text,
      title: "尺寸验收",
      position: { x: 0, y: 0 },
      width: 320,
      height: 200,
      metadata: { content: "正文" },
    },
    scale: 1,
    isSelected: true,
    isRelated: false,
    isFocusRelated: false,
    isConnectionTarget: false,
    isConnecting: false,
    showPanel: false,
    showImageInfo: false,
    onMouseDown: vi.fn(),
    onHoverStart: vi.fn(),
    onHoverEnd: vi.fn(),
    onConnectStart: vi.fn(),
    onResizeStart: vi.fn(),
    onResize: vi.fn(),
    onResizeEnd: vi.fn(),
    onContentChange: vi.fn(),
    onTitleChange: vi.fn(),
    onContextMenu: vi.fn(),
  };
}
it("仅选中可编辑节点显示单个缩放手柄，只读与参考选择不提供修改入口", () => {
  const p = props();
  const view = render(<CanvasNode {...p} />);
  expect(screen.getAllByRole("button", { name: "调整节点大小" })).toHaveLength(
    1,
  );
  view.rerender(<CanvasNode {...p} isSelected={false} />);
  expect(screen.queryByRole("button", { name: "调整节点大小" })).toBeNull();
  view.rerender(<CanvasNode {...p} readOnly />);
  expect(screen.queryByRole("button", { name: "调整节点大小" })).toBeNull();
  view.rerender(<CanvasNode {...p} referenceSelectionState="available" />);
  expect(screen.queryByRole("button", { name: "调整节点大小" })).toBeNull();
});
it("方向键调整节点尺寸，Shift加速，不触发画布移动", () => {
  const p = props();
  const key = vi.fn();
  render(
    <div onKeyDown={key}>
      <CanvasNode {...p} />
    </div>,
  );
  const handle = screen.getByRole("button", { name: "调整节点大小" });
  fireEvent.keyDown(handle, { key: "ArrowRight", shiftKey: true });
  expect(p.onResizeStart).toHaveBeenCalledWith("resize-node");
  expect(p.onResize).toHaveBeenLastCalledWith("resize-node", 330, 200, {
    x: 0,
    y: 0,
  });
  expect(p.onResizeEnd).toHaveBeenCalledWith("resize-node");
  expect(key).not.toHaveBeenCalled();
});
it("视频键盘缩放保持比例，文本缩小不低于最小尺寸", () => {
  const p = props();
  const view = render(
    <CanvasNode
      {...p}
      data={{
        ...p.data,
        type: CanvasNodeType.Video,
        width: 320,
        height: 180,
        metadata: {},
      }}
    />,
  );
  fireEvent.keyDown(screen.getByRole("button", { name: "调整节点大小" }), {
    key: "ArrowRight",
    shiftKey: true,
  });
  expect(p.onResize).toHaveBeenLastCalledWith("resize-node", 330, 185.625, {
    x: 0,
    y: 0,
  });
  view.rerender(
    <CanvasNode {...p} data={{ ...p.data, width: 220, height: 160 }} />,
  );
  fireEvent.keyDown(screen.getByRole("button", { name: "调整节点大小" }), {
    key: "ArrowLeft",
  });
  expect(p.onResize).toHaveBeenCalledTimes(1);
});
