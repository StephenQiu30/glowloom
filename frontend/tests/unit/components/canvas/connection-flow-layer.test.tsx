import { cleanup, render } from "@testing-library/react";
import { useRef } from "react";
import { afterEach, expect, it, vi } from "vitest";
import {
  ConnectionFlowLayer,
  ConnectionFlowProvider,
  useConnectionFlow,
} from "@/components/canvas/connections/connection-flow-layer";

const surface = vi.hoisted(() => ({
  upload: vi.fn(),
  render: vi.fn(),
  clear: vi.fn(),
  dispose: vi.fn(),
}));
vi.mock("@/components/canvas/connections/connection-flow-surface", () => ({
  createConnectionFlowSurface: () => surface,
}));
function Entry({ path, fanout = false }: { path: string; fanout?: boolean }) {
  const ref = useRef<SVGPathElement>(null);
  useConnectionFlow(path, true, ref, fanout);
  return <path ref={ref} d={path} />;
}
function Fixture({
  count = 2,
  fanout = false,
}: {
  count?: number;
  fanout?: boolean;
}) {
  const ref = useRef<HTMLDivElement>(null);
  return (
    <div ref={ref}>
      <ConnectionFlowProvider>
        <svg>
          {Array.from({ length: count }, (_, i) => (
            <Entry
              fanout={fanout}
              key={i}
              path={`M 0 ${i} C 100 ${i}, 200 ${i}, 300 ${i}`}
            />
          ))}
          <ConnectionFlowLayer containerRef={ref} />
        </svg>
      </ConnectionFlowProvider>
    </div>
  );
}
afterEach(() => {
  cleanup();
  delete (SVGElement.prototype as unknown as { getScreenCTM?: unknown })
    .getScreenCTM;
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});
it("shares one frame and surface, follows the viewport transform, then suspends and disposes", () => {
  const pending = new Map<number, FrameRequestCallback>();
  let sequence = 0;
  vi.stubGlobal(
    "requestAnimationFrame",
    vi.fn((callback: FrameRequestCallback) => {
      pending.set(++sequence, callback);
      return sequence;
    }),
  );
  vi.stubGlobal(
    "cancelAnimationFrame",
    vi.fn((id: number) => pending.delete(id)),
  );
  let onMotion = () => {};
  const motion = {
    matches: false,
    addEventListener: vi.fn(
      (_type: string, callback: () => void) => (onMotion = callback),
    ),
    removeEventListener: vi.fn(),
  };
  vi.stubGlobal("matchMedia", () => motion);
  Object.defineProperty(SVGElement.prototype, "getScreenCTM", {
    configurable: true,
    value: () => ({ a: 1.36, e: 20, f: 30 }),
  });
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    left: 5,
    top: 10,
    width: 800,
    height: 600,
  } as DOMRect);
  const view = render(<Fixture />);
  expect(pending.size).toBe(1);
  const [id, draw] = pending.entries().next().value!;
  pending.delete(id);
  draw(1300);
  expect(surface.upload).toHaveBeenCalledOnce();
  expect(surface.upload.mock.calls[0][0].length).toBe(2 * 64 * 54);
  expect(surface.render).toHaveBeenCalledWith(
    expect.objectContaining({
      x: 15,
      y: 20,
      zoom: 1.36,
      width: 800,
      height: 600,
    }),
    1300,
  );
  expect(pending.size).toBe(1);
  view.rerender(<Fixture fanout />);
  const [nextId, nextDraw] = pending.entries().next().value!;
  pending.delete(nextId);
  nextDraw(1350);
  expect(surface.upload.mock.calls.at(-1)![0].length).toBe(2 * 16 * 54);
  motion.matches = true;
  onMotion();
  expect(pending.size).toBe(0);
  expect(view.container.querySelector("canvas")?.dataset.rendering).toBe(
    "suspended",
  );
  expect(surface.clear).toHaveBeenCalled();
  view.unmount();
  expect(surface.dispose).toHaveBeenCalledOnce();
  expect(motion.removeEventListener).toHaveBeenCalledWith(
    "change",
    expect.any(Function),
  );
});
