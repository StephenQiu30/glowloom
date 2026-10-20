import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ConnectionPort } from "@/app/canvas/node/connection-port";
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
function setup() {
  const connect = vi.fn(),
    add = vi.fn();
  render(
    <div data-node-id="source">
      <ConnectionPort side="right" onAdd={add} onConnect={connect} />
    </div>,
  );
  const port = screen.getByRole("button", { name: "在右侧添加节点" });
  port.setPointerCapture = vi.fn();
  port.hasPointerCapture = () => false;
  return { port, add, connect };
}
it("单击仅展开菜单，选择类型后才新建", () => {
  const { port, add } = setup();
  fireEvent.click(port);
  expect(add).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("option", { name: "视频" }));
  expect(add).toHaveBeenCalledWith("video_generation", undefined);
});
it("拖到现有节点只连接，Escape取消不会创建", () => {
  const { port, add, connect } = setup();
  const target = document.createElement("div");
  target.dataset.nodeId = "target";
  target.dataset.nodeKind = "image";
  document.body.append(target);
  document.elementFromPoint = vi.fn(() => target);
  fireEvent.pointerDown(port, {
    button: 0,
    pointerId: 1,
    clientX: 10,
    clientY: 10,
  });
  fireEvent.pointerMove(port, { pointerId: 1, clientX: 100, clientY: 50 });
  fireEvent.pointerUp(port, { pointerId: 1, clientX: 100, clientY: 50 });
  expect(connect).toHaveBeenCalledWith("target");
  expect(add).not.toHaveBeenCalled();
  connect.mockClear();
  fireEvent.pointerDown(port, {
    button: 0,
    pointerId: 2,
    clientX: 10,
    clientY: 10,
  });
  fireEvent.pointerMove(port, { pointerId: 2, clientX: 100, clientY: 50 });
  fireEvent.keyDown(window, { key: "Escape" });
  fireEvent.pointerUp(port, { pointerId: 2, clientX: 100, clientY: 50 });
  expect(connect).not.toHaveBeenCalled();
  expect(target.dataset.connectionTarget).toBeUndefined();
  target.remove();
});
