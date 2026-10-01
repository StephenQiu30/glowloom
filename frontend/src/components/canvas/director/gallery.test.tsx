import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DirectorGallery } from "./gallery";
import { createDirectorConfig } from "./model";
vi.mock("../nodes/node-content", () => ({
  NodeContent: () => <div>授权预览占位</div>,
}));
vi.mock("../media-preview-dialog", () => ({
  MediaPreviewDialog: ({ node }: { node: { title: string } }) => (
    <div role="dialog" aria-label={node.title} />
  ),
}));
afterEach(cleanup);
describe("机位图库操作", () => {
  it("按机位显示多分镜截图，预览、选封面及移除仅修改闭合引用", () => {
    const scene = createDirectorConfig();
    const screenshot = {
      id: crypto.randomUUID(),
      assetId: crypto.randomUUID(),
      name: "合成图片",
      createdAt: "2026-10-02T00:00:00Z",
    };
    scene.shots[0].screenshots = [screenshot];
    const change = vi.fn();
    render(
      <DirectorGallery
        projectId={crypto.randomUUID()}
        scene={scene}
        disabled={false}
        onChange={change}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "查看" }));
    expect(screen.getByRole("dialog", { name: screenshot.name })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "设为封面" }));
    expect(change.mock.calls[0][0].cover).toEqual({
      assetId: screenshot.assetId,
      shotId: scene.activeShotId,
    });
    fireEvent.click(
      screen.getByRole("button", { name: `移除截图 ${screenshot.name}` }),
    );
    expect(change.mock.calls[1][0].shots[0].screenshots).toEqual([]);
  });
  it("只读图库仍可预览，但不能改变封面或解除引用", () => {
    const scene = createDirectorConfig();
    const screenshot = {
      id: crypto.randomUUID(),
      assetId: crypto.randomUUID(),
      name: "合成图片",
      createdAt: "2026-10-02T00:00:00Z",
    };
    scene.shots[0].screenshots = [screenshot];
    render(
      <DirectorGallery
        projectId={crypto.randomUUID()}
        scene={scene}
        disabled
        onChange={vi.fn()}
      />,
    );
    expect(
      screen.getByRole("button", { name: "设为封面" }).hasAttribute("disabled"),
    ).toBe(true);
    expect(
      screen
        .getByRole("button", { name: `移除截图 ${screenshot.name}` })
        .hasAttribute("disabled"),
    ).toBe(true);
    expect(
      screen.getByRole("button", { name: "查看" }).hasAttribute("disabled"),
    ).toBe(false);
  });
});
