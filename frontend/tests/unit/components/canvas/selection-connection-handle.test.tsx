import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { CanvasSelectionToolbar } from "@/components/canvas/controls/canvas-selection-toolbar";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";
vi.mock("@/components/canvas/runtime/store/use-theme-store", () => ({
  useThemeStore: (selector: (state: { theme: string }) => unknown) =>
    selector({ theme: "dark" }),
}));
const nodes: CanvasNodeData[] = [
  {
    id: "a",
    type: "text",
    title: "A",
    position: { x: 100, y: 50 },
    width: 200,
    height: 100,
  },
  {
    id: "b",
    type: "text",
    title: "B",
    position: { x: 350, y: 300 },
    width: 150,
    height: 200,
  },
];
const props = {
  nodes,
  viewport: { x: 20, y: -10, k: 0.71 },
  showToolbar: true,
  canGroup: false,
  canUngroup: false,
  onGroup: vi.fn(),
  onUngroup: vi.fn(),
};
afterEach(cleanup);
it.each([0.25, 0.71, 1, 1.36])(
  "places the aggregate port outside unpadded union at zoom %s",
  (k) => {
    const { container } = render(
      <CanvasSelectionToolbar
        {...props}
        viewport={{ ...props.viewport, k }}
        onConnectStart={vi.fn()}
      />,
    );
    const host = container.querySelector<HTMLElement>(
      "[data-canvas-selection-connection]",
    )!;
    expect(host).not.toBeNull();
    expect(parseFloat(host.style.left)).toBeCloseTo(20 + 500 * k + 40 * k + 16);
    expect(parseFloat(host.style.top)).toBeCloseTo(-10 + 275 * k);
    expect(host.style.width).toBe("0px");
    expect(host.style.height).toBe("0px");
    const field = host.querySelector<HTMLElement>(
      "[data-canvas-connection-handle]",
    )!;
    expect(field.dataset.variant).toBe("aggregate");
    expect(field.style.width).toBe("60px");
    expect(field.style.height).toBe("120px");
    expect(field.style.getPropertyValue("--canvas-handle-counter-scale")).toBe(
      "1",
    );
    const glyph = field.querySelector<HTMLElement>(
      "[data-canvas-connection-glyph]",
    )!;
    expect(glyph.style.transform).toBe(
      "translate3d(30px, 60px, 0) scale(1) translate(-50%, -50%)",
    );
    expect(glyph.style.width).toBe("20px");
    const button = screen.getByRole("button", {
      name: "为选中节点创建后接节点",
    });
    expect(button.style.width).toBe("36px");
    expect(button.style.height).toBe("36px");
  },
);
it("dispatches mouse once, supports keyboard, and prefers start over legacy click", () => {
  const start = vi.fn(),
    legacy = vi.fn(),
    canvasDown = vi.fn();
  const { container } = render(
    <div onMouseDown={canvasDown}>
      <CanvasSelectionToolbar
        {...props}
        onConnectStart={start}
        onConnectAfter={legacy}
      />
    </div>,
  );
  const field = container.querySelector<HTMLElement>(
    "[data-variant=aggregate]",
  )!;
  const button = screen.getByRole("button", { name: "为选中节点创建后接节点" });
  fireEvent.mouseDown(field, { button: 2 });
  expect(start).not.toHaveBeenCalled();
  fireEvent.mouseDown(field, { button: 0, clientX: 420, clientY: 180 });
  fireEvent.click(button, { detail: 1 });
  expect(start).toHaveBeenCalledTimes(1);
  expect(canvasDown).toHaveBeenCalledTimes(1);
  fireEvent.click(button, { detail: 0 });
  expect(start).toHaveBeenCalledTimes(2);
  expect(legacy).not.toHaveBeenCalled();
});
it("retains legacy activation without a second mouse click callback", () => {
  const legacy = vi.fn();
  const { container } = render(
    <CanvasSelectionToolbar {...props} onConnectAfter={legacy} />,
  );
  fireEvent.mouseDown(container.querySelector("[data-variant=aggregate]")!, {
    button: 0,
  });
  fireEvent.click(
    screen.getByRole("button", { name: "为选中节点创建后接节点" }),
    { detail: 1 },
  );
  expect(legacy).toHaveBeenCalledTimes(1);
});
it.each([{ readOnly: true }, { showToolbar: false }])(
  "hides aggregate when unavailable %j",
  (state) => {
    render(
      <CanvasSelectionToolbar {...props} {...state} onConnectStart={vi.fn()} />,
    );
    expect(
      screen.queryByRole("button", { name: "为选中节点创建后接节点" }),
    ).toBeNull();
  },
);
it.each([0.25, 0.71, 1, 1.36])(
  "renders the source content-box selection frame in world coordinates at zoom %s",
  (k) => {
    const { container } = render(
      <CanvasSelectionToolbar
        {...props}
        viewport={{ ...props.viewport, k }}
        onConnectStart={vi.fn()}
      />,
    );
    const host = container.querySelector<HTMLElement>(
      "[data-canvas-drag-selection]",
    )!;
    const box = container.querySelector<HTMLElement>(
      "[data-canvas-selection-frame]",
    )!;
    expect(box).not.toBeNull();
    expect(host.style.left).toBe(`${20 + 100 * k}px`);
    expect(host.style.top).toBe(`${-10 + 50 * k}px`);
    expect(host.style.transform).toBe(""); // drag presentation owns this transform
    expect(box.style.left).toBe(`${-40 * k}px`);
    expect(box.style.top).toBe(`${-40 * k}px`);
    expect(box.style.width).toBe("400px");
    expect(box.style.height).toBe("450px");
    expect(box.style.boxSizing).toBe("content-box");
    expect(box.style.padding).toBe("40px");
    expect(box.style.borderRadius).toBe("40px");
    expect(box.style.borderStyle).toBe("dashed");
    expect(parseFloat(box.style.borderWidth)).toBeCloseTo(1 / k);
    expect(box.style.transform).toBe(`scale(${k})`);
    expect(box.style.background).toBe(
      "color-mix(in srgb, var(--canvas-reference-foreground) 3.9215686274509802%, transparent)",
    );
    expect(box.style.borderColor).toBe(
      "color-mix(in srgb, var(--canvas-reference-foreground) 20%, transparent)",
    );
    expect(host.querySelector("rect")).toBeNull();
  },
);
