import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useState } from "react";
import {
  MediaUploadDialog,
  type CanvasMediaUpload,
} from "./media-upload-dialog";
import type { MediaAsset } from "./queries";

const image = new File(["image"], "参考.png", { type: "image/png" });
const second = new File(["image2"], "镜头.webp", { type: "image/webp" });
const asset: MediaAsset = {
  id: "ea1459ea-a0e0-427f-acb0-691ddf39639c",
  project_id: "568e584f-468d-4ed6-bc92-cdf57b9f7203",
  kind: "image",
  file_name: image.name,
  mime_type: image.type,
  byte_size: image.size,
  revision: 1,
};
const secondAsset = {
  ...asset,
  id: "1c037e9f-0bc5-43b6-ada8-94990f350b6b",
  file_name: second.name,
};
afterEach(cleanup);
function setup({
  files = [image],
  upload = vi.fn<CanvasMediaUpload>().mockResolvedValue(asset),
  imported = vi.fn().mockResolvedValue(undefined),
  remainingSlots = 20,
  disabled = false,
  target = "canvas" as "canvas" | "folder-cover" | "project-cover",
} = {}) {
  const changed = vi.fn();
  const busy = vi.fn();
  function Harness() {
    const [open, setOpen] = useState(true);
    return (
      <MediaUploadDialog
        open={open}
        onOpenChange={(value) => {
          changed(value);
          setOpen(value);
        }}
        initialFiles={files}
        remainingSlots={remainingSlots}
        upload={upload}
        onImported={imported}
        onBusyChange={busy}
        disabled={disabled}
        target={target}
        maximumFiles={target !== "canvas" ? 1 : undefined}
      />
    );
  }
  return { ...render(<Harness />), upload, imported, changed, busy };
}
function confirm() {
  fireEvent.click(screen.getByRole("checkbox", { name: /已检查内容/ }));
}
function start() {
  fireEvent.click(screen.getByRole("button", { name: "开始上传" }));
}

it("必须显式本地人工确认，没有自动审核和预览URL", async () => {
  const view = setup();
  expect(screen.getByRole("button", { name: "开始上传" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(screen.getByText(/本地人工确认/)).toBeTruthy();
  expect(view.container.querySelector("img,video,audio")).toBeNull();
  confirm();
  start();
  await waitFor(() => expect(view.imported).toHaveBeenCalledWith([asset]));
  expect(view.upload).toHaveBeenCalledTimes(1);
  expect(view.upload.mock.calls[0][1]).toMatch(/^[0-9a-f-]{36}$/);
  expect(view.changed).toHaveBeenCalledWith(false);
});
it("目录封面只受理图片并确认使用权，上传后选素材而不显示画布保存", async () => {
  const view = setup({ target: "folder-cover", remainingSlots: 1 });
  expect(screen.getByRole("dialog", { name: "上传目录封面" })).toBeTruthy();
  expect(screen.getByLabelText("选择本地媒体文件").getAttribute("accept")).toBe(
    ".jpg,.jpeg,.png,.webp",
  );
  expect(screen.queryByText(/加入画布|当前可添加/)).toBeNull();
  confirm();
  start();
  await waitFor(() =>
    expect(view.imported).toHaveBeenCalledExactlyOnceWith([asset]),
  );
  expect(view.upload).toHaveBeenCalledTimes(1);
});
it("项目主图沿图片人工确认管线，只选择正式素材，不创建画布节点", async () => {
  const view = setup({ target: "project-cover", remainingSlots: 1 });
  expect(screen.getByRole("dialog", { name: "上传项目主图" })).toBeTruthy();
  expect(screen.getByLabelText("选择本地媒体文件").getAttribute("accept")).toBe(
    ".jpg,.jpeg,.png,.webp",
  );
  expect(screen.queryByText(/目录封面|加入画布|当前可添加/)).toBeNull();
  confirm();
  start();
  await waitFor(() =>
    expect(view.imported).toHaveBeenCalledExactlyOnceWith([asset]),
  );
});
it("项目主图明确拒绝视频和多文件，不发送上传", async () => {
  const view = setup({
    target: "project-cover",
    files: [new File(["movie"], "片段.mp4", { type: "video/mp4" }), image],
    remainingSlots: 1,
  });
  expect(screen.getByText(/项目主图只支持图片/)).toBeTruthy();
  confirm();
  start();
  expect(view.upload).not.toHaveBeenCalled();
});
it("逐文件上传，可见真实发送进度，全部成功只保存一次", async () => {
  let finish: (asset: MediaAsset) => void = () => {};
  const upload = vi
    .fn<CanvasMediaUpload>()
    .mockImplementationOnce((_file, _key, { onProgress }) => {
      onProgress({ loaded: 2, total: 4 });
      return new Promise((resolve) => {
        finish = resolve;
      });
    })
    .mockResolvedValue(secondAsset);
  const view = setup({ files: [image, second], upload });
  confirm();
  start();
  expect(upload).toHaveBeenCalledTimes(1);
  expect(
    screen
      .getByRole("progressbar", { name: "参考.png 上传进度" })
      .getAttribute("aria-valuenow"),
  ).toBe("50");
  expect(screen.getByText(/正在上传/)).toBeTruthy();
  await act(async () => finish(asset));
  await waitFor(() => expect(view.imported).toHaveBeenCalledTimes(1));
  expect(upload).toHaveBeenCalledTimes(2);
  expect(view.imported).toHaveBeenCalledWith([asset, secondAsset]);
});
it("部分失败不自动保存，重试只重发失败文件并复用原key", async () => {
  const upload = vi
    .fn<CanvasMediaUpload>()
    .mockResolvedValueOnce(asset)
    .mockRejectedValueOnce(new Error("private server path"))
    .mockResolvedValueOnce(secondAsset);
  const view = setup({ files: [image, second], upload });
  confirm();
  start();
  await screen.findByRole("button", { name: "重试未成功文件" });
  expect(view.imported).not.toHaveBeenCalled();
  expect(screen.queryByText("private server path")).toBeNull();
  expect(screen.getByText(/已成功上传的素材已保留/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "重试未成功文件" }));
  await waitFor(() => expect(view.imported).toHaveBeenCalledTimes(1));
  expect(upload.mock.calls.map(([file]) => file.name)).toEqual([
    image.name,
    second.name,
    second.name,
  ]);
  expect(upload.mock.calls[2][1]).toBe(upload.mock.calls[1][1]);
});
it("用户可明确将部分成功资产加入画布", async () => {
  const upload = vi
    .fn<CanvasMediaUpload>()
    .mockResolvedValueOnce(asset)
    .mockRejectedValueOnce(new Error("offline"));
  const view = setup({ files: [image, second], upload });
  confirm();
  start();
  fireEvent.click(
    await screen.findByRole("button", { name: "将成功的 1 个加入画布" }),
  );
  await waitFor(() => expect(view.imported).toHaveBeenCalledWith([asset]));
  expect(upload).toHaveBeenCalledTimes(2);
});
it("画布保存失败保留已上传资产，重试保存不重复上传", async () => {
  const imported = vi
    .fn()
    .mockRejectedValueOnce(new Error("conflict with private detail"))
    .mockResolvedValueOnce(undefined);
  const view = setup({ imported });
  confirm();
  start();
  fireEvent.click(await screen.findByRole("button", { name: "重试加入画布" }));
  await waitFor(() => expect(imported).toHaveBeenCalledTimes(2));
  expect(view.upload).toHaveBeenCalledTimes(1);
  expect(imported.mock.calls[0][0]).toEqual(imported.mock.calls[1][0]);
});
it("取消请求后显式展示取消，重试仍复用原key", async () => {
  let signal: AbortSignal | undefined;
  const upload = vi
    .fn<CanvasMediaUpload>()
    .mockImplementationOnce((_file, _key, options) => {
      signal = options.signal;
      return new Promise((_resolve, reject) =>
        signal!.addEventListener("abort", () =>
          reject(new DOMException("cancelled", "AbortError")),
        ),
      );
    })
    .mockResolvedValue(asset);
  const view = setup({ upload });
  confirm();
  start();
  fireEvent.click(screen.getByRole("button", { name: "取消上传" }));
  expect(signal?.aborted).toBe(true);
  await screen.findByText("已取消，可用原请求重试");
  expect(view.imported).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "重试未成功文件" }));
  await waitFor(() => expect(view.imported).toHaveBeenCalledTimes(1));
  expect(upload.mock.calls[0][1]).toBe(upload.mock.calls[1][1]);
});
it("卸载取消上传，不执行后续保存", async () => {
  let signal: AbortSignal | undefined;
  let finish: (asset: MediaAsset) => void = () => {};
  const upload = vi
    .fn<CanvasMediaUpload>()
    .mockImplementation((_file, _key, options) => {
      signal = options.signal;
      return new Promise((resolve) => {
        finish = resolve;
      });
    });
  const view = setup({ upload });
  confirm();
  start();
  view.unmount();
  expect(signal?.aborted).toBe(true);
  await act(async () => finish(asset));
  expect(view.imported).not.toHaveBeenCalled();
  expect(view.busy).toHaveBeenLastCalledWith(false);
});
it("保存进行中阻止Esc和关闭，重复点击不产生第二次保存", async () => {
  let finish: () => void = () => {};
  const imported = vi.fn().mockImplementation(
    () =>
      new Promise<void>((resolve) => {
        finish = resolve;
      }),
  );
  const view = setup({ imported });
  confirm();
  start();
  await waitFor(() => expect(imported).toHaveBeenCalledTimes(1));
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  expect(screen.getByRole("button", { name: "关闭上传窗口" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(view.changed).not.toHaveBeenCalled();
  await act(async () => finish());
  expect(view.changed).toHaveBeenCalledWith(false);
});
it("超剩余节点与只读均不能上传，可由文件选择器选择新的有效批次", async () => {
  const view = setup({ remainingSlots: 0 });
  confirm();
  expect(screen.getByRole("button", { name: "开始上传" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(view.upload).not.toHaveBeenCalled();
  cleanup();
  setup({ files: [], disabled: true });
  expect(screen.getByLabelText("选择本地媒体文件")).toHaveProperty(
    "disabled",
    true,
  );
});
it("从原生文件选择器读取新批次，不接受不支持格式", async () => {
  const view = setup({ files: [] });
  fireEvent.change(screen.getByLabelText("选择本地媒体文件"), {
    target: { files: [new File(["x"], "x.zip", { type: "application/zip" })] },
  });
  expect(screen.getByText(/不支持/)).toBeTruthy();
  fireEvent.change(screen.getByLabelText("选择本地媒体文件"), {
    target: { files: [image] },
  });
  confirm();
  start();
  await waitFor(() => expect(view.imported).toHaveBeenCalledTimes(1));
});

it("部分成功保存失败后冻结原保存批次，不再上传其他文件改变请求", async () => {
  const upload = vi
    .fn<CanvasMediaUpload>()
    .mockResolvedValueOnce(asset)
    .mockRejectedValueOnce(new Error("offline"));
  const imported = vi
    .fn()
    .mockRejectedValueOnce(new Error("conflict"))
    .mockResolvedValueOnce(undefined);
  setup({ files: [image, second], upload, imported });
  confirm();
  start();
  fireEvent.click(
    await screen.findByRole("button", { name: "将成功的 1 个加入画布" }),
  );
  await screen.findByRole("button", { name: "重试加入画布" });
  expect(screen.queryByRole("button", { name: "重试未成功文件" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试加入画布" }));
  await waitFor(() => expect(imported).toHaveBeenCalledTimes(2));
  expect(imported.mock.calls[0][0]).toEqual([asset]);
  expect(imported.mock.calls[1][0]).toEqual([asset]);
  expect(upload).toHaveBeenCalledTimes(2);
});

it("上传期间更换busy回调不发布错误的空闲状态", async () => {
  const notify = vi.fn();
  let finish: (asset: MediaAsset) => void = () => {};
  const upload = vi.fn<CanvasMediaUpload>().mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const props = {
    open: true,
    onOpenChange: vi.fn(),
    initialFiles: [image],
    remainingSlots: 20,
    upload,
    onImported: vi.fn().mockResolvedValue(undefined),
  };
  const view = render(
    <MediaUploadDialog {...props} onBusyChange={(busy) => notify(busy)} />,
  );
  confirm();
  start();
  notify.mockClear();
  view.rerender(
    <MediaUploadDialog {...props} onBusyChange={(busy) => notify(busy)} />,
  );
  expect(notify.mock.calls.some(([busy]) => !busy)).toBe(false);
  await act(async () => finish(asset));
});

it("取消后的晚到回执不保存，下一次重试复用同一UUID", async () => {
  let finish: (asset: MediaAsset) => void = () => {};
  const upload = vi
    .fn<CanvasMediaUpload>()
    .mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    )
    .mockResolvedValue(asset);
  const view = setup({ upload });
  confirm();
  start();
  fireEvent.click(screen.getByRole("button", { name: "取消上传" }));
  await act(async () => finish(asset));
  expect(view.imported).not.toHaveBeenCalled();
  fireEvent.click(
    await screen.findByRole("button", { name: "重试未成功文件" }),
  );
  await waitFor(() => expect(view.imported).toHaveBeenCalledTimes(1));
  expect(upload.mock.calls[0][1]).toBe(upload.mock.calls[1][1]);
});
