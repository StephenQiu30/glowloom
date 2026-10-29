import { cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  ActiveConnectionPath,
  ConnectionPath,
} from "@/components/canvas/connections/canvas-connections";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";

vi.mock("@/components/canvas/runtime/store/use-theme-store", () => ({
  useThemeStore: (select: (state: { theme: string }) => unknown) =>
    select({ theme: "dark" }),
}));

afterEach(cleanup);
const from: CanvasNodeData = {
  id: "from",
  type: "text",
  title: "文本",
  position: { x: 10, y: 20 },
  width: 320,
  height: 320,
};
const to: CanvasNodeData = { ...from, id: "to", position: { x: 500, y: 300 } };
const connection = { id: "edge", fromNodeId: from.id, toNodeId: to.id };

it("renders a continuous thin blue preview with a meteor, including reverse input dragging", () => {
  const view = render(
    <svg>
      <ActiveConnectionPath
        node={from}
        handle={{ nodeId: from.id, handleType: "source" }}
        mouseWorld={{ x: 500, y: 460 }}
        target={to}
      />
    </svg>,
  );
  const line = () => view.container.querySelector(".canvas-connection-line")!;
  expect(line().getAttribute("stroke-dasharray")).toBeNull();
  expect(line().getAttribute("stroke")).toBe("#008EE5");
  expect(line().getAttribute("stroke-width")).toBe("1");
  expect(
    view.container.querySelector(".canvas-connection-meteor"),
  ).not.toBeNull();
  expect(line().getAttribute("stroke-opacity")).toBe("1");
  expect(view.container.querySelector(".canvas-connection-head")).toBeNull();
  const path = line().getAttribute("d");
  view.rerender(
    <svg>
      <ActiveConnectionPath
        node={to}
        handle={{ nodeId: to.id, handleType: "target" }}
        mouseWorld={{ x: 0, y: 0 }}
        target={from}
      />
    </svg>,
  );
  expect(line().getAttribute("d")).toBe(path);
});

it("keeps a separate zoom-independent hit area and animates selected generating edges", () => {
  const onSelect = vi.fn();
  const onContextMenu = vi.fn();
  const view = render(
    <svg>
      <ConnectionPath
        connection={connection}
        from={from}
        to={to}
        active={false}
        selected
        generating
        onSelect={onSelect}
        onContextMenu={onContextMenu}
      />
    </svg>,
  );
  expect(
    view.container
      .querySelector(".canvas-connection-line")
      ?.getAttribute("stroke-width"),
  ).toBe("1");
  const hit = view.container.querySelector('[data-connection-id="edge"]')!;
  expect(hit.getAttribute("stroke-width")).toBe("16");
  expect(hit.getAttribute("vector-effect")).toBe("non-scaling-stroke");
  fireEvent.click(hit);
  fireEvent.contextMenu(hit);
  expect(onSelect).toHaveBeenCalledOnce();
  expect(onContextMenu).toHaveBeenCalledOnce();
  expect(
    view.container.querySelector(".canvas-connection-meteor"),
  ).not.toBeNull();
});

it("does not animate idle connections and cleans up their hover meteor", () => {
  const view = render(
    <svg>
      <ConnectionPath
        connection={connection}
        from={from}
        to={to}
        active={false}
        onSelect={vi.fn()}
      />
    </svg>,
  );
  const edge = view.container.querySelector('[data-canvas-edge-id="edge"]')!;
  expect(view.container.querySelector(".canvas-connection-meteor")).toBeNull();
  fireEvent.mouseEnter(edge);
  expect(
    view.container.querySelector(".canvas-connection-meteor"),
  ).not.toBeNull();
  fireEvent.mouseLeave(edge);
  expect(view.container.querySelector(".canvas-connection-meteor")).toBeNull();
});

it("offsets only dragging anchors outside the node and keeps pending anchors centered", () => {
  const view = render(
    <svg>
      <ActiveConnectionPath
        node={from}
        handle={{ nodeId: from.id, handleType: "source" }}
        mouseWorld={{ x: 500, y: 460 }}
        target={to}
        anchorOffset={4 / 0.6}
      />
    </svg>,
  );
  const path = () =>
    view.container.querySelector(".canvas-connection-line")!.getAttribute("d")!;
  expect(Number(path().split(" ")[1])).toBeCloseTo(
    from.position.x + from.width + 4 / 0.6,
  );
  view.rerender(
    <svg>
      <ActiveConnectionPath
        node={from}
        handle={{ nodeId: from.id, handleType: "source" }}
        mouseWorld={{ x: 500, y: 460 }}
      />
    </svg>,
  );
  expect(Number(path().split(" ")[1])).toBe(from.position.x + from.width);
});
