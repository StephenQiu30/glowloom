import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  CanvasContainerToolbar,
  CanvasStackGallery,
  CanvasStackCard,
} from "@/components/canvas/containers/canvas-container-ui";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import { createContainer } from "@/components/canvas/lib/canvas-containers";
afterEach(cleanup);
const a = createCanvasNode(
  "text",
  { x: 20, y: 30 },
  { content: "原始多行\n内容" },
);
const b = createCanvasNode(
  "image",
  { x: 200, y: 50 },
  { content: "https://media.example/authorized.png" },
);
function fixture(kind: "group" | "stack") {
  return createContainer(
    [a, b],
    new Set([a.id, b.id]),
    createCanvasNode("group", { x: 0, y: 0 }),
    kind,
  )[0];
}
it("ordinary group exposes operations independent of workflow and preserves the source storyboard restriction", () => {
  const release = vi.fn();
  render(
    <CanvasContainerToolbar
      node={fixture("group")}
      members={[a, b]}
      viewport={{ x: 0, y: 0, k: 1 }}
      readOnly={false}
      onRelease={release}
      onLayout={vi.fn()}
      onBackground={vi.fn()}
      onDownload={vi.fn()}
    />,
  );
  expect(screen.getByRole("toolbar", { name: "组操作" })).toBeTruthy();
  expect(
    screen.getByRole("button", { name: /转为分镜组/ }).getAttribute("title"),
  ).toBe("分镜组仅支持图片，最多支持49个图片节点");
  fireEvent.click(screen.getByRole("button", { name: "解除编组" }));
  expect(release).toHaveBeenCalledOnce();
  expect(screen.getByRole("button", { name: "布局" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "背景色" })).toBeTruthy();
});
it("stack has an independent read-only toolbar and leaves authorized downloading available", () => {
  const download = vi.fn();
  render(
    <CanvasContainerToolbar
      node={fixture("stack")}
      members={[a, b]}
      viewport={{ x: 0, y: 0, k: 1 }}
      readOnly
      onRelease={vi.fn()}
      onLayout={vi.fn()}
      onBackground={vi.fn()}
      onDownload={download}
    />,
  );
  expect(screen.getByRole("toolbar", { name: "堆叠操作" })).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "解除堆叠" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(screen.queryByRole("button", { name: "布局" })).toBeNull();
  expect(screen.queryByRole("button", { name: /分镜/ })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "下载" }));
  expect(download).toHaveBeenCalledOnce();
});
it("gallery renders original text and authorized image content in a full-screen dialog", () => {
  render(
    <CanvasStackGallery
      container={fixture("stack")}
      members={[a, b]}
      onClose={vi.fn()}
    />,
  );
  expect(screen.getByText("原始多行 内容")).toBeTruthy();
  expect(screen.getByRole("img", { name: b.title }).getAttribute("src")).toBe(
    "https://media.example/authorized.png",
  );
  expect(screen.getByRole("dialog").className).toContain("translate-x-0");
  expect(screen.getByRole("dialog").className).toContain("max-h-none");
});
it("stack card presents a preview and opens the gallery without changing the scene", () => {
  const open = vi.fn();
  render(<CanvasStackCard members={[a, b]} onOpen={open} />);
  expect(screen.getByText("堆叠 · 2 个节点")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "打开堆叠画廊" }));
  expect(open).toHaveBeenCalledOnce();
});
