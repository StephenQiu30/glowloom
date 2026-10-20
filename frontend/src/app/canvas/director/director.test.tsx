import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CanvasDirector } from "./director";
import { createDirectorConfig } from "./model";
import type { Node } from "../document";

vi.mock("three", async (original) => {
  const actual = await original<typeof import("three")>();
  return {
    ...actual,
    WebGLRenderer: class {
      constructor() {
        throw new Error("WebGL unavailable in unit test");
      }
    },
  };
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
beforeEach(() => {
  // jsdom intentionally has no GPU. The unit suite verifies the editable fallback;
  // real Three.js rendering is checked by the parent browser acceptance.
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);
});
function node(): Node {
  return {
    id: crypto.randomUUID(),
    title: "导演台",
    node_type: "tool",
    node_action: "director",
    config: { director: createDirectorConfig() },
    x: 0,
    y: 0,
    z_index: 0,
  };
}
describe("director editing", () => {
  it("persists real scene objects and camera edits through the supplied update contract", () => {
    const onUpdate = vi.fn();
    render(
      <CanvasDirector
        node={node()}
        nodes={[]}
        onUpdate={onUpdate}
        onClose={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "添加立方体" }));
    const config = onUpdate.mock.lastCall![0] as API.DirectorConfig;
    expect(config.objects).toHaveLength(1);
    expect(config.objects![0]).toMatchObject({
      primitive: "box",
      kind: "primitive",
    });
    fireEvent.change(screen.getByRole("textbox", { name: "对象名称" }), {
      target: { value: "主角位置" },
    });
    expect(onUpdate.mock.lastCall![0].objects[0].name).toBe("主角位置");
    fireEvent.click(screen.getByRole("button", { name: "添加关键帧" }));
    expect(onUpdate.mock.lastCall![0].objects[0].keyframes).toHaveLength(1);
  });
  it("surfaces the parent save result and failure message inside the director", async () => {
    const save = vi.fn().mockResolvedValue(false);
    render(
      <CanvasDirector
        node={node()}
        nodes={[]}
        onUpdate={vi.fn()}
        onClose={vi.fn()}
        onSave={save}
        saveMessage="服务端拒绝保存，本地编辑仍保留。"
      />,
    );
    expect(screen.getByText("服务端拒绝保存，本地编辑仍保留。")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "保存画布" }));
    await waitFor(() => expect(save).toHaveBeenCalledOnce());
    expect(
      await screen.findByText("尚未确认保存，请检查保存状态。"),
    ).toBeTruthy();
  });
  it("reports unavailable WebGL while retaining editable scene settings", async () => {
    render(
      <CanvasDirector
        node={node()}
        nodes={[]}
        onUpdate={vi.fn()}
        onClose={vi.fn()}
      />,
    );
    expect(await screen.findByText(/无法初始化真实 3D 视口/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "重新加载视口" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "添加人偶" })).toBeTruthy();
  });
  it("creates new shots and can close through its accessible controls", () => {
    const close = vi.fn(),
      update = vi.fn();
    render(
      <CanvasDirector
        node={node()}
        nodes={[]}
        onUpdate={update}
        onClose={close}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "添加镜头" }));
    expect(update.mock.lastCall![0].shots).toHaveLength(2);
    fireEvent.click(screen.getByRole("button", { name: "关闭导演台" }));
    expect(close).toHaveBeenCalledOnce();
  });
});
