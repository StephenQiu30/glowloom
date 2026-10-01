import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocalUploadPreview } from "./local-upload-preview";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
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
