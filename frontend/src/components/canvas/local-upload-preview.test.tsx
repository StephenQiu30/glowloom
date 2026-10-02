import { animatedGIFFile } from "../media/gif-test-fixtures";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocalUploadPreview } from "./local-upload-preview";
const renderedModel = vi.hoisted(() => vi.fn());
vi.mock("next/dynamic", () => ({
  default:
    () =>
    ({
      file,
      byteSize,
      mimeType,
      onError,
    }: {
      file: File;
      byteSize: number;
      mimeType: string;
      onError: () => void;
    }) => {
      renderedModel({ file, byteSize, mimeType });
      return <button onClick={onError}>合成模型解码失败</button>;
    },
}));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
it.each([
  ["原场景.gltf", "model/gltf+json"],
  ["原场景.glb", "model/gltf-binary"],
])(
  "%s模型仅明确打开后传原File，不创建Blob伪GLB，失败/关闭停止预览",
  (name, mime) => {
    const create = vi.fn();
    vi.stubGlobal("URL", { createObjectURL: create, revokeObjectURL: vi.fn() });
    const original = new File(["synthetic original"], name, { type: mime });
    const view = render(<LocalUploadPreview file={original} />);
    expect(
      screen.queryByRole("button", { name: "合成模型解码失败" }),
    ).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "预览本地文件" }));
    expect(renderedModel).toHaveBeenLastCalledWith({
      file: original,
      byteSize: original.size,
      mimeType: mime,
    });
    expect(create).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "合成模型解码失败" }));
    expect(screen.getByRole("alert").textContent).toContain("本地模型无法预览");
    expect(
      screen.queryByRole("button", { name: "合成模型解码失败" }),
    ).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "收起本地预览" }));
    view.unmount();
  },
);
it("仅显式预览时创建本地租约，收起与卸载释放URL，不发正式素材请求", () => {
  const create = vi.fn(() => "blob:owned-preview");
  const revoke = vi.fn();
  const pause = vi
    .spyOn(HTMLMediaElement.prototype, "pause")
    .mockImplementation(() => {});
  const load = vi
    .spyOn(HTMLMediaElement.prototype, "load")
    .mockImplementation(() => {});
  vi.stubGlobal("URL", { createObjectURL: create, revokeObjectURL: revoke });
  const file = new File(["controlled bytes"], "白膜.webm", {
    type: "video/webm",
  });
  const view = render(<LocalUploadPreview file={file} />);
  expect(create).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "预览本地文件" }));
  expect(create).toHaveBeenCalledWith(file);
  expect(screen.getByLabelText("白膜.webm 本地预览").getAttribute("src")).toBe(
    "blob:owned-preview",
  );
  fireEvent.click(screen.getByRole("button", { name: "收起本地预览" }));
  expect(revoke).toHaveBeenCalledWith("blob:owned-preview");
  expect(pause).toHaveBeenCalledOnce();
  expect(load).toHaveBeenCalledOnce();
  fireEvent.click(screen.getByRole("button", { name: "预览本地文件" }));
  view.unmount();
  expect(revoke).toHaveBeenCalledTimes(2);
  expect(pause).toHaveBeenCalledTimes(2);
  expect(load).toHaveBeenCalledTimes(2);
});

it("GIF预览保留原File动画来源，收起及卸载释放每次Blob租约", () => {
  const create = vi.fn(() => "blob:whole-gif");
  const revoke = vi.fn();
  vi.stubGlobal("URL", { createObjectURL: create, revokeObjectURL: revoke });
  const original = animatedGIFFile("动画.GIF");
  const view = render(<LocalUploadPreview file={original} />);
  expect(create).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "预览本地文件" }));
  const image = screen.getByRole("img", { name: "动画.GIF 本地预览" });
  expect(create).toHaveBeenCalledExactlyOnceWith(original);
  expect(image.getAttribute("src")).toBe("blob:whole-gif");
  expect(view.container.querySelector("canvas,video")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "收起本地预览" }));
  expect(image.hasAttribute("src")).toBe(false);
  expect(revoke).toHaveBeenCalledExactlyOnceWith("blob:whole-gif");
  fireEvent.click(screen.getByRole("button", { name: "预览本地文件" }));
  view.unmount();
  expect(revoke).toHaveBeenCalledTimes(2);
});

it("GIF本地解码失败立即释放Blob，明确错误后重新打开才取得新租约", () => {
  const create = vi.fn(() => "blob:gif-failed");
  const revoke = vi.fn();
  vi.stubGlobal("URL", { createObjectURL: create, revokeObjectURL: revoke });
  render(<LocalUploadPreview file={animatedGIFFile()} />);
  fireEvent.click(screen.getByRole("button", { name: "预览本地文件" }));
  const image = screen.getByRole("img");
  fireEvent.error(image);
  expect(screen.getByRole("alert").textContent).toContain("无法预览");
  expect(image.hasAttribute("src")).toBe(false);
  expect(revoke).toHaveBeenCalledExactlyOnceWith("blob:gif-failed");
  expect(create).toHaveBeenCalledOnce();
  fireEvent.click(screen.getByRole("button", { name: "收起本地预览" }));
  fireEvent.click(screen.getByRole("button", { name: "预览本地文件" }));
  expect(create).toHaveBeenCalledTimes(2);
});
