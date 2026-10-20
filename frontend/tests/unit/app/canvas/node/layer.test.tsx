import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { createRef } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { createResourceNode } from "@/app/canvas/document";
import { CanvasNodeLayer } from "@/app/canvas/node/layer";
import { CANVAS_VIEWPORT_EVENT } from "@/app/canvas/viewport/transform";
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});
it("连续平移更新可见窗口，焦点节点不卸载，卸载清理延迟更新", () => {
  vi.useFakeTimers();
  const board = createRef<HTMLDivElement>();
  const near = createResourceNode("text", { x: 0, y: 0 });
  const far = {
    ...createResourceNode("text", { x: 10000, y: 0 }),
    title: "远处",
  };
  const view = render(
    <div ref={board}>
      <CanvasNodeLayer
        containerRef={board}
        nodes={[near, far]}
        edges={[]}
        viewport={{ x: 0, y: 0, k: 1 }}
        size={{ width: 500, height: 500 }}
        selectedIDs={[]}
        renderNode={(node) => (
          <button data-node-id={node.id}>{node.title}</button>
        )}
      />
    </div>,
  );
  expect(screen.queryByText("远处")).toBeNull();
  act(() => screen.getByRole("button", { name: "文本" }).focus());
  act(() =>
    board.current!.dispatchEvent(
      new CustomEvent(CANVAS_VIEWPORT_EVENT, {
        detail: { x: -10000, y: 0, k: 1 },
      }),
    ),
  );
  act(() => vi.advanceTimersByTime(90));
  expect(screen.getByText("远处")).toBeTruthy();
  expect(screen.getByText("文本")).toBe(document.activeElement);
  fireEvent.blur(document.activeElement!);
  act(() =>
    board.current!.dispatchEvent(
      new CustomEvent(CANVAS_VIEWPORT_EVENT, { detail: { x: 0, y: 0, k: 1 } }),
    ),
  );
  view.unmount();
  expect(vi.getTimerCount()).toBe(0);
});
it("拖动预览同步标注线的世界端点，不等待图保存", () => {
  const board = createRef<HTMLDivElement>();
  const a = createResourceNode("text", { x: 0, y: 0 }),
    b = createResourceNode("text", { x: 600, y: 0 });
  render(
    <div ref={board}>
      <CanvasNodeLayer
        containerRef={board}
        nodes={[a, b]}
        edges={[
          {
            id: "edge",
            edge_type: "annotation",
            source_node_id: a.id,
            target_node_id: b.id,
          },
        ]}
        viewport={{ x: 0, y: 0, k: 1 }}
        size={{ width: 1000, height: 700 }}
        selectedIDs={[]}
        renderNode={(node) => (
          <button
            data-node-id={node.id}
            style={{ left: node.x, top: node.y, width: 288 }}
          >
            {node.title}
          </button>
        )}
      />
    </div>,
  );
  const source = board.current!.querySelector<HTMLElement>(
    `[data-node-id="${a.id}"]`,
  )!;
  source.style.left = "100px";
  act(() =>
    board.current!.dispatchEvent(new Event("canvas:node-drag-preview")),
  );
  expect(board.current!.querySelector("path")?.getAttribute("d")).toMatch(
    /^M 388 160 /,
  );
});
