import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ComponentProps } from "react";
import { CanvasNode } from "@/components/canvas/nodes/canvas-node";
import { canvasNodeAccessibility } from "@/components/canvas/lib/canvas-node-accessibility";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";
vi.mock("@/components/canvas/runtime/store/use-theme-store", () => ({
  useThemeStore: (select: (state: { theme: "dark" }) => unknown) =>
    select({ theme: "dark" }),
}));
afterEach(cleanup);
const node: CanvasNodeData = {
  id: "accessible-node",
  title: "可读文本",
  type: "text",
  position: { x: 0, y: 0 },
  width: 320,
  height: 200,
  metadata: { content: "秘密正文" },
};
function props(): ComponentProps<typeof CanvasNode> {
  return {
    data: node,
    scale: 1,
    isSelected: false,
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
it("exposes an associated node summary and updates selection/read-only semantics without geometry changes", () => {
  const p = props(),
    view = render(<CanvasNode {...p} />);
  const group = screen.getByRole("group", { name: "文本节点：可读文本" });
  const description = () =>
    document.getElementById(group.getAttribute("aria-describedby")!)!
      .textContent;
  expect(description()).toContain("未选中。 可编辑。 无媒体资源。");
  expect(description()).toContain("Enter");
  expect(description()).not.toContain("秘密正文");
  const geometry = group.getAttribute("style");
  view.rerender(<CanvasNode {...p} isSelected readOnly />);
  expect(description()).toContain("已选中。 只读。");
  expect(description()).not.toContain("Enter");
  expect(screen.queryByRole("button", { name: "编辑文本节点内容" })).toBeNull();
  expect(group.getAttribute("style")).toBe(geometry);
});
it("counts known candidate resource states but never treats a fixed source or URL as ready", () => {
  const selection = { selected: false, readOnly: false };
  const image = {
    ...node,
    type: "image",
    metadata: {
      images: [
        {
          id: "a",
          status: "success",
          content: "",
          naturalWidth: 1,
          naturalHeight: 1,
        },
        {
          id: "b",
          status: "loading",
          content: "",
          naturalWidth: 1,
          naturalHeight: 1,
        },
        {
          id: "c",
          status: "error",
          content: "",
          naturalWidth: 1,
          naturalHeight: 1,
        },
      ],
    },
  } as CanvasNodeData;
  expect(canvasNodeAccessibility(image, selection).description).toContain(
    "1 项就绪，1 项处理中，1 项失败",
  );
  expect(
    canvasNodeAccessibility(
      {
        ...node,
        type: "image",
        metadata: { content: "https://private.test/preview" },
      },
      selection,
    ).description,
  ).toContain("0 项就绪");
  expect(
    canvasNodeAccessibility(
      {
        ...node,
        type: "image",
        serviceData: { media_ref: { asset_id: "asset" } },
      },
      selection,
    ).description,
  ).toContain("1 项状态未核验");
  expect(
    canvasNodeAccessibility(
      {
        ...node,
        type: "subject",
        serviceData: {
          config: { subject: { version: 1, subject_id: "s", version_id: "v" } },
        },
      },
      selection,
    ).description,
  ).toContain("已固定主体版本；媒体状态未核验");
});
it("describes reference selection availability and only meaningful editing hints", () => {
  expect(
    canvasNodeAccessibility(node, {
      selected: true,
      readOnly: false,
      editing: true,
    }).description,
  ).toContain("正在编辑");
  expect(
    canvasNodeAccessibility(
      { ...node, type: "director", title: "导演台 1" },
      { selected: false, readOnly: false },
    ).label,
  ).toBe("导演台节点：导演台 1");
  const unavailable = canvasNodeAccessibility(node, {
    selected: false,
    readOnly: false,
    reference: "disabled",
  });
  expect(unavailable.description).toContain("不可选择为引用");
  expect(unavailable.description).not.toContain("Enter");
  expect(
    canvasNodeAccessibility(
      { ...node, type: "unknown", title: "  " },
      { selected: false, readOnly: true },
    ).label,
  ).toBe("其他节点：未命名节点");
});

it("activates visible connection handles with a keyboard click and keeps hidden ports out of tab order", () => {
  const p = props();
  const view = render(<CanvasNode {...p} isSelected />);
  fireEvent.click(screen.getByRole("button", { name: "连接输出节点" }), {
    detail: 0,
  });
  expect(p.onConnectStart).toHaveBeenCalledOnce();
  expect(vi.mocked(p.onConnectStart).mock.calls[0].slice(1)).toEqual([
    node.id,
    "source",
  ]);
  view.rerender(<CanvasNode {...p} />);
  expect(screen.getByRole("button", { name: "连接输出节点" }).tabIndex).toBe(
    -1,
  );
});

it("names typed stacks separately from legacy collapsed groups", () => {
  const group = {
    ...node,
    type: "group",
    title: "容器",
    serviceData: {
      config: {
        container: {
          version: 1,
          kind: "stack" as const,
          background: "none" as const,
        },
      },
    },
  };
  expect(
    canvasNodeAccessibility(group, { selected: false, readOnly: false }).label,
  ).toBe("堆叠节点：容器");
  expect(
    canvasNodeAccessibility(
      { ...group, serviceData: { config: { collapsed: true } } },
      { selected: false, readOnly: false },
    ).label,
  ).toBe("组节点：容器");
});

it("keeps a world-space hover field around each port rather than a fixed square hit box", () => {
  const view = render(<CanvasNode {...props()} scale={0.71} />);
  const button = screen.getByRole("button", { name: "连接输出节点" });
  const field = button.parentElement!;
  expect(field.style.width).toBe("60px");
  expect(field.style.height).toBe("120px");
  expect(button.style.width).toBe("20px");
  view.unmount();
});
