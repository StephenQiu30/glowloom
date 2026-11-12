import { createRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { InfiniteCanvas } from "@/components/canvas/editor/infinite-canvas";

vi.mock("@/components/canvas/runtime/store/use-theme-store", () => ({
  useThemeStore: (select: (state: { theme: string }) => unknown) =>
    select({ theme: "dark" }),
}));
afterEach(cleanup);

function surface(children: ReactNode, tool: "select" | "pan" = "select") {
  const node = vi.fn();
  const background = vi.fn();
  const view = render(
    <InfiniteCanvas
      containerRef={createRef()}
      viewport={{ x: 0, y: 0, k: 0.5 }}
      tool={tool}
      onViewportChange={vi.fn()}
      onNodeDoubleClick={node}
      onCanvasDoubleClick={background}
    >
      {children}
    </InfiniteCanvas>,
  );
  return {
    ...view,
    node,
    background,
    canvas: screen.getByLabelText("创作画布"),
  };
}

it("routes a node body double click to the original node ID without creating background content", () => {
  const { node, background } = surface(
    <div data-node-id="original-director" data-node-type="director">
      <section>
        <strong>导演台预览</strong>
      </section>
    </div>,
  );
  fireEvent.doubleClick(screen.getByText("导演台预览"));
  expect(node).toHaveBeenCalledExactlyOnceWith("original-director");
  expect(background).not.toHaveBeenCalled();
});

it("keeps blank double clicks on the background creation path", () => {
  const { node, background, canvas } = surface(null);
  fireEvent.doubleClick(canvas);
  expect(background).toHaveBeenCalledOnce();
  expect(node).not.toHaveBeenCalled();
});

it("leaves buttons, editable controls, links, excluded regions, and connections to their owners", () => {
  const { node, background } = surface(
    <div data-node-id="director">
      <button>进入导演台</button>
      <input aria-label="名称" />
      <textarea aria-label="说明" />
      <select aria-label="机位">
        <option>正面</option>
      </select>
      <a href="#camera">跳转</a>
      <span role="button" tabIndex={0}>
        其他动作
      </span>
      <div contentEditable suppressContentEditableWarning>
        编辑正文
      </div>
      <section data-canvas-no-zoom>局部独立视口</section>
      <div data-connection-id="connection">连接</div>
    </div>,
  );
  for (const element of [
    screen.getByRole("button", { name: "进入导演台" }),
    screen.getByLabelText("名称"),
    screen.getByLabelText("说明"),
    screen.getByLabelText("机位"),
    screen.getByRole("link"),
    screen.getByRole("button", { name: "其他动作" }),
    screen.getByText("编辑正文"),
    screen.getByText("局部独立视口"),
    screen.getByText("连接"),
  ])
    fireEvent.doubleClick(element);
  expect(node).not.toHaveBeenCalled();
  expect(background).not.toHaveBeenCalled();
});

it.each(["text", "image", "plugin"])(
  "preserves the %s owner's stopped double-click event",
  (kind) => {
    const own = vi.fn();
    const { node, background } = surface(
      <div data-node-id={kind}>
        <section
          onDoubleClick={(event) => {
            event.stopPropagation();
            own();
          }}
        >
          {kind}
        </section>
      </div>,
    );
    fireEvent.doubleClick(screen.getByText(kind));
    expect(own).toHaveBeenCalledOnce();
    expect(node).not.toHaveBeenCalled();
    expect(background).not.toHaveBeenCalled();
  },
);

it("respects a node owner's prevented double click", () => {
  const { node, background } = surface(
    <div
      data-node-id="director"
      onDoubleClick={(event) => event.preventDefault()}
    >
      已处理
    </div>,
  );
  fireEvent.doubleClick(screen.getByText("已处理"));
  expect(node).not.toHaveBeenCalled();
  expect(background).not.toHaveBeenCalled();
});

it.each(["pan", "active-pan", "space", "overlay"])(
  "does not route node double clicks during %s",
  (mode) => {
    const { node, background, canvas } = surface(
      <div data-node-id="director">导演台预览</div>,
      mode === "pan" ? "pan" : "select",
    );
    if (mode === "space")
      fireEvent.keyDown(canvas, { code: "Space", key: " " });
    if (mode === "active-pan")
      fireEvent.pointerDown(canvas, { button: 1, pointerId: 1 });
    const overlay = mode === "overlay" ? document.createElement("div") : null;
    if (overlay) {
      overlay.setAttribute("role", "dialog");
      document.body.append(overlay);
    }
    try {
      fireEvent.doubleClick(screen.getByText("导演台预览"));
    } finally {
      overlay?.remove();
      if (mode === "active-pan") fireEvent.pointerUp(window, { pointerId: 1 });
    }
    expect(node).not.toHaveBeenCalled();
    expect(background).not.toHaveBeenCalled();
  },
);

it("ignores React portals outside the canvas DOM", () => {
  const { node, background } = surface(
    createPortal(<div data-node-id="director">独立浮层</div>, document.body),
  );
  fireEvent.doubleClick(screen.getByText("独立浮层"));
  expect(node).not.toHaveBeenCalled();
  expect(background).not.toHaveBeenCalled();
});

it("does not reinterpret a node as blank canvas when node focus is unavailable", () => {
  const background = vi.fn();
  render(
    <InfiniteCanvas
      containerRef={createRef()}
      viewport={{ x: 0, y: 0, k: 1 }}
      tool="select"
      onViewportChange={vi.fn()}
      onCanvasDoubleClick={background}
    >
      <div data-node-id="director">引用选择中的节点</div>
    </InfiniteCanvas>,
  );
  fireEvent.doubleClick(screen.getByText("引用选择中的节点"));
  expect(background).not.toHaveBeenCalled();
});
