import { cleanup, render } from "@testing-library/react";
import { createRef } from "react";
import { afterEach, expect, it } from "vitest";
import { CanvasNodePanel } from "./panel";
import { createDraftNode } from "../document";
import {
  applyCanvasLiveViewport,
  CANVAS_NODE_DRAG_EVENT,
} from "../viewport/transform";
afterEach(cleanup);
it("提示词浮层逐帧跟随真实媒体矩形，切换节点更换锚点", () => {
  const container = createRef<HTMLDivElement>();
  container.current = document.createElement("div");
  Object.defineProperty(container.current, "clientWidth", { value: 1280 });
  const n = createDraftNode("image_generation", "1:1", 0),
    other = createDraftNode("video_generation", "1:1", 1);
  container.current.innerHTML = `<div data-canvas-world-layer><div data-node-id="${n.id}"><button class="canvas-node-shell"></button></div><div data-node-id="${other.id}"><button class="canvas-node-shell"></button></div></div>`;
  let bottom = 300,
    left = 100;
  const shell = container.current.querySelector<HTMLElement>(
    `[data-node-id="${n.id}"] .canvas-node-shell`,
  )!;
  shell.getBoundingClientRect = () =>
    ({ top: bottom - 200, bottom, left, width: 288, height: 200 }) as DOMRect;
  container.current.querySelector<HTMLElement>(
    `[data-node-id="${other.id}"] .canvas-node-shell`,
  )!.getBoundingClientRect = () =>
    ({ top: 300, bottom: 500, left: 600, width: 288, height: 200 }) as DOMRect;
  const view = render(
    <CanvasNodePanel
      node={n}
      viewport={{ x: 0, y: 0, k: 1 }}
      containerRef={container}
    >
      Prompt
    </CanvasNodePanel>,
  );
  const panel = document.querySelector<HTMLElement>(
    "[data-canvas-node-panel]",
  )!;
  expect([panel.style.left, panel.style.top]).toEqual(["76px", "320px"]);
  expect(panel.style.transform).toBe("");
  bottom = 380;
  left = 400;
  container.current.dispatchEvent(new Event(CANVAS_NODE_DRAG_EVENT));
  expect([panel.style.left, panel.style.top]).toEqual(["204px", "400px"]);
  expect(panel.style.transform).toBe("");
  bottom = 240;
  left = 500;
  applyCanvasLiveViewport(container.current, { x: 100, y: -100, k: 0.5 });
  expect([panel.style.left, panel.style.top]).toEqual(["304px", "260px"]);
  expect(panel.style.transform).toBe("");
  view.rerender(
    <CanvasNodePanel
      node={other}
      viewport={{ x: 0, y: 0, k: 1 }}
      containerRef={container}
    >
      Other
    </CanvasNodePanel>,
  );
  expect(panel.dataset.canvasNodePanel).toBe(other.id);
  expect([panel.style.left, panel.style.top]).toEqual(["404px", "520px"]);
  expect(panel.style.transform).toBe("");
  view.unmount();
  container.current.dispatchEvent(new Event(CANVAS_NODE_DRAG_EVENT));
});
