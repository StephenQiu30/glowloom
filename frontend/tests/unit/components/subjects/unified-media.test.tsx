import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { UnifiedSubjectMedia } from "@/components/subjects/unified-media";
import { setCreationScope } from "@/lib/request";
const api = vi.hoisted(() => ({
  bound: vi.fn(),
  library: vi.fn(),
  source: vi.fn(),
}));
vi.mock("@/api/subjects", () => ({
  getSubjectMedia: api.bound,
  previewSubjectSource: api.source,
}));
vi.mock("@/api/media", () => ({ previewLibraryMediaAsset: api.library }));
const makeURL = vi.fn(() => "blob:subject-preview"),
  revokeURL = vi.fn();
beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal(
    "URL",
    class extends URL {
      static createObjectURL = makeURL;
      static revokeObjectURL = revokeURL;
    },
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
it("已绑定预览通过主体版本授权接口读取，并释放本地ObjectURL", async () => {
  api.bound.mockResolvedValue(new Blob(["bytes"], { type: "image/png" }));
  const media = {
    asset_id: "asset",
    asset_revision: 1,
    kind: "image" as const,
    file_name: "主参考.png",
    mime_type: "image/png",
    bound: { subjectID: "subject", versionID: "version", bindingID: "binding" },
  };
  const view = render(<UnifiedSubjectMedia media={media} label="主参考图" />);
  await screen.findByRole("img", { name: "主参考图" });
  expect(api.bound).toHaveBeenCalledWith(
    { id: "subject", version: "version", binding: "binding" },
    expect.objectContaining({ responseType: "blob" }),
  );
  expect(api.library).not.toHaveBeenCalled();
  view.unmount();
  expect(revokeURL).toHaveBeenCalledWith("blob:subject-preview");
});
it("卸载后到达的响应不创建遗留的ObjectURL", async () => {
  let resolve!: (value: Blob) => void;
  api.bound.mockImplementation(
    () =>
      new Promise<Blob>((r) => {
        resolve = r;
      }),
  );
  const view = render(
    <UnifiedSubjectMedia
      media={{
        asset_id: "asset",
        asset_revision: 1,
        kind: "audio",
        file_name: "音色.wav",
        mime_type: "audio/wav",
        bound: {
          subjectID: "subject",
          versionID: "version",
          bindingID: "voice",
        },
      }}
      label="音色"
    />,
  );
  view.unmount();
  await act(async () => {
    resolve(new Blob(["bytes"]));
  });
  expect(makeURL).not.toHaveBeenCalled();
});

it("切换空间后立即隐藏旧私有预览并重新授权，不沿用上一空间的blob", async () => {
  setCreationScope("space-a");
  api.bound
    .mockReset()
    .mockResolvedValueOnce(new Blob(["bytes"]))
    .mockRejectedValueOnce(new Error("forbidden"));
  const media = {
    asset_id: "asset",
    asset_revision: 1,
    kind: "image" as const,
    bound: { subjectID: "subject", versionID: "version", bindingID: "binding" },
  };
  const view = render(
    <UnifiedSubjectMedia media={media} label="空间私有参考" />,
  );
  await screen.findByRole("img", { name: "空间私有参考" });
  setCreationScope("space-b");
  view.rerender(<UnifiedSubjectMedia media={media} label="空间私有参考" />);
  expect(screen.queryByRole("img", { name: "空间私有参考" })).toBeNull();
  await screen.findByText("预览暂不可用");
  expect(api.bound).toHaveBeenCalledTimes(2);
  expect(revokeURL).toHaveBeenCalledWith("blob:subject-preview");
  setCreationScope(null);
});

it("待保存来源使用主体来源预览接口，不调用受个人空间限制的旧预览", async () => {
  api.source.mockResolvedValue({
    asset: { id: "asset" },
    expires_at: new Date(Date.now() + 60000).toISOString(),
    url: "https://example.test/source-preview.png",
  });
  render(
    <UnifiedSubjectMedia
      media={{
        asset_id: "asset",
        asset_revision: 7,
        kind: "image",
        file_name: "图.png",
        mime_type: "image/png",
        library_item_id: "item",
        library_source: "subject",
      }}
      label="来源主图"
    />,
  );
  const image = await screen.findByRole("img", { name: "来源主图" });
  expect(image.getAttribute("src")).toBe(
    "https://example.test/source-preview.png",
  );
  expect(api.source).toHaveBeenCalledWith(
    { item_id: "item" },
    expect.objectContaining({ signal: expect.any(AbortSignal) }),
  );
  expect(api.library).not.toHaveBeenCalled();
  expect(api.bound).not.toHaveBeenCalled();
});

it("音色以真实播放/暂停按钮呈现，并跟随播放结束恢复", async () => {
  api.bound.mockResolvedValue(new Blob(["bytes"], { type: "audio/wav" }));
  const play = vi
    .spyOn(HTMLMediaElement.prototype, "play")
    .mockImplementation(function (this: HTMLMediaElement) {
      fireEvent.play(this);
      return Promise.resolve();
    });
  const pause = vi
    .spyOn(HTMLMediaElement.prototype, "pause")
    .mockImplementation(function (this: HTMLMediaElement) {
      fireEvent.pause(this);
    });
  const view = render(
    <UnifiedSubjectMedia
      media={{
        asset_id: "asset",
        asset_revision: 1,
        kind: "audio",
        file_name: "音色.wav",
        mime_type: "audio/wav",
        bound: {
          subjectID: "subject",
          versionID: "version",
          bindingID: "voice",
        },
      }}
      label="上传音色1"
      interactive
    />,
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "播放 上传音色1" }),
  );
  expect(play).toHaveBeenCalledTimes(1);
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "暂停 上传音色1" }),
    ).toHaveProperty("disabled", false),
  );
  fireEvent.click(screen.getByRole("button", { name: "暂停 上传音色1" }));
  expect(pause).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "播放 上传音色1" }));
  fireEvent.ended(view.container.querySelector("audio")!);
  expect(screen.getByRole("button", { name: "播放 上传音色1" })).toBeTruthy();
  expect(view.container.querySelector("audio")!.controls).toBe(false);
});
