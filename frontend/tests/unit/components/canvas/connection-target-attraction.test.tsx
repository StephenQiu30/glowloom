import { cleanup, render } from "@testing-library/react";
import { useRef } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { ActiveConnectionPath } from "@/components/canvas/connections/canvas-connections";
import {
  ConnectionFlowLayer,
  ConnectionFlowProvider,
} from "@/components/canvas/connections/connection-flow-layer";
import type {
  CanvasNodeData,
  ConnectionHandle,
} from "@/components/canvas/types/canvas";

const surface = vi.hoisted(() => ({
  upload: vi.fn(),
  render: vi.fn(),
  clear: vi.fn(),
  dispose: vi.fn(),
}));
vi.mock("@/components/canvas/connections/connection-flow-surface", () => ({
  createConnectionFlowSurface: () => surface,
}));
const from: CanvasNodeData = {
  id: "from",
  type: "text",
  title: "A",
  position: { x: 10, y: 20 },
  width: 320,
  height: 320,
};
const to: CanvasNodeData = { ...from, id: "to", position: { x: 500, y: 300 } };
function endpoints(container: HTMLElement) {
  const d = container
    .querySelector(".canvas-connection-line")!
    .getAttribute("d")!;
  const values = d.match(/-?\d+(?:\.\d+)?(?:e[+-]?\d+)?/gi)!.map(Number);
  return { start: values.slice(0, 2), end: values.slice(-2) };
}
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  delete (SVGElement.prototype as unknown as { getScreenCTM?: unknown })
    .getScreenCTM;
});
it.each([
  {
    side: "source" as const,
    node: from,
    target: to,
    mouse: { x: 460, y: 420 },
    start: [334, 180],
    end: [475.2, 435.2],
  },
  {
    side: "target" as const,
    node: to,
    target: from,
    mouse: { x: 400, y: 220 },
    start: [373.4, 204.8],
    end: [496, 460],
  },
])(
  "attracts only the free $side endpoint 38% toward the candidate edge midpoint",
  ({ side, node, target, mouse, start, end }) => {
    const view = render(
      <svg>
        <ActiveConnectionPath
          node={node}
          handle={{ nodeId: node.id, handleType: side }}
          mouseWorld={mouse}
          target={target}
          targetAttraction={0.38}
          anchorOffset={4}
        />
      </svg>,
    );
    const actual = endpoints(view.container);
    actual.start.forEach((value, index) =>
      expect(value).toBeCloseTo(start[index], 8),
    );
    actual.end.forEach((value, index) =>
      expect(value).toBeCloseTo(end[index], 8),
    );
  },
);
it.each(["source", "target"] as const)(
  "fully snaps %s at attraction 1 and retains the same default behavior",
  (side) => {
    const node = side === "source" ? from : to,
      target = side === "source" ? to : from;
    const props = {
      node,
      target,
      handle: { nodeId: node.id, handleType: side },
      mouseWorld: { x: 400, y: 220 },
      anchorOffset: 4,
    };
    const view = render(
      <svg>
        <ActiveConnectionPath {...props} targetAttraction={1} />
      </svg>,
    );
    expect(endpoints(view.container)).toEqual({
      start: [334, 180],
      end: [496, 460],
    });
    view.rerender(
      <svg>
        <ActiveConnectionPath {...props} />
      </svg>,
    );
    expect(endpoints(view.container)).toEqual({
      start: [334, 180],
      end: [496, 460],
    });
  },
);
it.each(["source", "target"] as const)(
  "keeps the unsnapped %s pointer coordinates when the target disappears",
  (side) => {
    const node = side === "source" ? from : to,
      target = side === "source" ? to : from;
    const props = {
      node,
      handle: { nodeId: node.id, handleType: side },
      mouseWorld: { x: 800, y: 700 },
      targetAttraction: 0.38,
      anchorOffset: 4,
    };
    const view = render(
      <svg>
        <ActiveConnectionPath {...props} target={target} />
      </svg>,
    );
    view.rerender(
      <svg>
        <ActiveConnectionPath {...props} />
      </svg>,
    );
    expect(endpoints(view.container)).toEqual(
      side === "source"
        ? { start: [334, 180], end: [800, 700] }
        : { start: [800, 700], end: [496, 460] },
    );
  },
);
function FanoutFixture({ fanout }: { fanout: boolean }) {
  const ref = useRef<HTMLDivElement>(null);
  return (
    <div ref={ref}>
      <ConnectionFlowProvider>
        <svg>
          {[from, { ...from, id: "second", position: { x: 10, y: 380 } }].map(
            (node) => (
              <ActiveConnectionPath
                key={node.id}
                node={node}
                handle={
                  {
                    nodeId: node.id,
                    handleType: "source",
                  } satisfies ConnectionHandle
                }
                mouseWorld={{ x: 460, y: 420 }}
                target={to}
                targetAttraction={0.38}
                fanout={fanout}
              />
            ),
          )}
          <ConnectionFlowLayer containerRef={ref} />
        </svg>
      </ConnectionFlowProvider>
    </div>
  );
}
it("uploads 16 segments per fanout preview and returns to 64 when the preview becomes detailed", () => {
  const pending = new Map<number, FrameRequestCallback>();
  let sequence = 0;
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    pending.set(++sequence, callback);
    return sequence;
  });
  vi.stubGlobal("cancelAnimationFrame", (id: number) => pending.delete(id));
  Object.defineProperty(SVGElement.prototype, "getScreenCTM", {
    configurable: true,
    value: () => ({ a: 1, e: 0, f: 0 }),
  });
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    left: 0,
    top: 0,
    width: 900,
    height: 700,
  } as DOMRect);
  const draw = () => {
    const [id, callback] = pending.entries().next().value!;
    pending.delete(id);
    callback(1300);
  };
  const view = render(<FanoutFixture fanout />);
  draw();
  expect(surface.upload.mock.calls.at(-1)![0]).toHaveLength(2 * 16 * 54);
  expect(
    view.container.querySelectorAll("[data-canvas-connection-preview]"),
  ).toHaveLength(2);
  view.rerender(<FanoutFixture fanout={false} />);
  draw();
  expect(surface.upload.mock.calls.at(-1)![0]).toHaveLength(2 * 64 * 54);
  view.unmount();
  expect(pending.size).toBe(0);
  expect(surface.dispose).toHaveBeenCalledOnce();
});
