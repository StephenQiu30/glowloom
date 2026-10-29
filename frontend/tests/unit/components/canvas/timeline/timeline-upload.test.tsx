import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TimelineUploadDialog } from "@/components/canvas/timeline/timeline-upload-dialog";
const upload = vi.hoisted(() => vi.fn());
vi.mock("@/api/media", () => ({ uploadMediaAsset: upload }));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
const projectID = "10000000-0000-4000-8000-000000000001";
const file = (name = "image.png") =>
  new File(["data"], name, { type: "image/png" });
const receipt = (kind = "image") => ({
  asset: { id: crypto.randomUUID(), project_id: projectID, kind },
});
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
function mount(
  files: File[],
  onUploaded = vi.fn().mockResolvedValue(undefined),
  extra = {},
) {
  const props = {
    projectID,
    initialFiles: files,
    onUploaded,
    onClose: vi.fn(),
    ...extra,
  };
  return { ...render(<TimelineUploadDialog {...props} />), props };
}
function confirm() {
  fireEvent.click(
    screen.getByRole("checkbox", {
      name: "我有使用权限，且无待授权真人内容。",
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "开始上传" }));
}
it("requires one explicit batch confirmation and never silently drops unsupported files", async () => {
  upload.mockResolvedValue(receipt());
  const onUploaded = vi.fn().mockResolvedValue(undefined);
  mount([file(), file("unsupported.txt")], onUploaded);
  expect(
    (screen.getByRole("button", { name: "开始上传" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(screen.getByText("unsupported.txt")).toBeTruthy();
  expect(screen.getByText("时间线仅支持图片、视频和音频文件。")).toBeTruthy();
  expect(upload).not.toHaveBeenCalled();
  confirm();
  await waitFor(() => expect(onUploaded).toHaveBeenCalledTimes(1));
  expect(upload).toHaveBeenCalledTimes(1);
  expect(upload.mock.calls[0][1]).toEqual({ local_review_confirmed: true });
  expect(screen.getByText("已就绪")).toBeTruthy();
});
it("rejects 51 files as a whole and accepts exactly 50", () => {
  const view = mount(Array.from({ length: 51 }, (_, i) => file(`${i}.png`)));
  expect(screen.getByRole("alert").textContent).toContain("最多选择50个文件");
  expect(screen.queryAllByRole("listitem")).toHaveLength(0);
  view.unmount();
  mount(Array.from({ length: 50 }, (_, i) => file(`${i}.png`)));
  expect(screen.getAllByRole("listitem")).toHaveLength(50);
  expect(upload).not.toHaveBeenCalled();
});
it("limits concurrency to two, reports each file progress, and retains successful items", async () => {
  const pending = [
    deferred<ReturnType<typeof receipt>>(),
    deferred<ReturnType<typeof receipt>>(),
    deferred<ReturnType<typeof receipt>>(),
  ];
  pending.forEach((item) => upload.mockImplementationOnce(() => item.promise));
  mount([file("one.png"), file("two.png"), file("three.png")]);
  confirm();
  expect(upload).toHaveBeenCalledTimes(2);
  act(() => upload.mock.calls[0][3].onUploadProgress({ loaded: 1, total: 2 }));
  expect(
    screen
      .getByRole("progressbar", { name: "one.png 上传进度" })
      .getAttribute("aria-valuenow"),
  ).toBe("50");
  await act(async () => pending[0].resolve(receipt()));
  await waitFor(() => expect(upload).toHaveBeenCalledTimes(3));
  expect(screen.getAllByText("已就绪")).toHaveLength(1);
  await act(async () => {
    pending[1].reject(new Error("network"));
    pending[2].resolve(receipt());
  });
  expect(screen.getAllByText("已就绪")).toHaveLength(2);
  expect(screen.getByRole("button", { name: "重试 two.png" })).toBeTruthy();
});
it("reuses the original file key after an uncertain upload and retries only failed files", async () => {
  upload
    .mockRejectedValueOnce(new Error("network"))
    .mockResolvedValueOnce(receipt());
  mount([file()]);
  confirm();
  fireEvent.click(
    await screen.findByRole("button", { name: "重试 image.png" }),
  );
  await screen.findByText("已就绪");
  expect(upload).toHaveBeenCalledTimes(2);
  expect(upload.mock.calls[0][3].headers["Idempotency-Key"]).toBe(
    upload.mock.calls[1][3].headers["Idempotency-Key"],
  );
});
it("retries only archival when upload succeeded but the timeline write failed", async () => {
  const asset = receipt();
  upload.mockResolvedValue(asset);
  const save = vi
    .fn()
    .mockRejectedValueOnce(new Error("conflict"))
    .mockResolvedValueOnce(undefined);
  mount([file()], save);
  confirm();
  fireEvent.click(
    await screen.findByRole("button", { name: "重试归档 image.png" }),
  );
  await screen.findByText("已就绪");
  expect(upload).toHaveBeenCalledTimes(1);
  expect(save).toHaveBeenCalledTimes(2);
  expect(save.mock.calls[1][0]).toEqual(asset.asset);
});
it("aborts uploads on unmount and does not archive a late receipt", async () => {
  const pending = deferred<ReturnType<typeof receipt>>();
  upload.mockReturnValue(pending.promise);
  const save = vi.fn();
  const view = mount([file()], save);
  confirm();
  const signal = upload.mock.calls[0][3].signal as AbortSignal;
  view.unmount();
  expect(signal.aborted).toBe(true);
  await act(async () => pending.resolve(receipt()));
  expect(save).not.toHaveBeenCalled();
});
it("stops submissions when the captured timeline becomes stale or read only", async () => {
  const pending = deferred<ReturnType<typeof receipt>>();
  upload.mockReturnValue(pending.promise);
  let current = true;
  const save = vi.fn();
  const view = mount([file()], save, { isCurrent: () => current });
  confirm();
  current = false;
  await act(async () => pending.resolve(receipt()));
  expect(save).not.toHaveBeenCalled();
  view.rerender(<TimelineUploadDialog {...view.props} disabled />);
  expect(
    (
      screen.getByRole("button", {
        name: "重试归档 image.png",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
});
it("lists every dropped file and rejects a wrong-project receipt without archiving", async () => {
  upload.mockResolvedValue({
    asset: { ...receipt().asset, project_id: crypto.randomUUID() },
  });
  const save = vi.fn();
  mount([], save);
  fireEvent.drop(screen.getByRole("button", { name: "选择或拖入素材" }), {
    dataTransfer: { files: [file(), file("bad.exe")] },
  });
  expect(screen.getAllByRole("listitem")).toHaveLength(2);
  confirm();
  await screen.findByRole("button", { name: "重试 image.png" });
  expect(save).not.toHaveBeenCalled();
  expect(
    within(screen.getAllByRole("listitem")[1]).getByText(
      "时间线仅支持图片、视频和音频文件。",
    ),
  ).toBeTruthy();
});

it("does not start queued files after closing while aborted requests settle", async () => {
  const first = deferred<ReturnType<typeof receipt>>(),
    second = deferred<ReturnType<typeof receipt>>();
  upload.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
  const { props } = mount([
    file("one.png"),
    file("two.png"),
    file("three.png"),
  ]);
  confirm();
  fireEvent.click(screen.getByRole("button", { name: "中止并关闭" }));
  expect(props.onClose).toHaveBeenCalledTimes(1);
  expect(upload.mock.calls.every((call) => call[3].signal.aborted)).toBe(true);
  await act(async () => {
    first.reject(new Error("aborted"));
    second.reject(new Error("aborted"));
  });
  expect(upload).toHaveBeenCalledTimes(2);
});
it("aborts read-only transitions and leaves queued work retryable without submitting it", async () => {
  const first = deferred<ReturnType<typeof receipt>>(),
    second = deferred<ReturnType<typeof receipt>>();
  upload.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
  const view = mount([file("one.png"), file("two.png"), file("three.png")]);
  confirm();
  view.rerender(<TimelineUploadDialog {...view.props} disabled />);
  expect(upload.mock.calls.every((call) => call[3].signal.aborted)).toBe(true);
  await act(async () => {
    first.reject(new Error("aborted"));
    second.reject(new Error("aborted"));
  });
  expect(upload).toHaveBeenCalledTimes(2);
  expect(
    (
      screen.getByRole("button", {
        name: "重试 three.png",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
});

it("accepts image, video, and audio multi-select while checking each receipt kind", async () => {
  upload.mockImplementation((_params, _body, chosen: File) =>
    Promise.resolve(
      receipt(
        chosen.name.endsWith("mp4")
          ? "video"
          : chosen.name.endsWith("wav")
            ? "audio"
            : "image",
      ),
    ),
  );
  const save = vi.fn().mockResolvedValue(undefined);
  mount([], save);
  fireEvent.change(screen.getByLabelText("选择素材文件"), {
    target: {
      files: [file("picture.png"), file("movie.mp4"), file("sound.wav")],
    },
  });
  confirm();
  await waitFor(() => expect(save).toHaveBeenCalledTimes(3));
  expect(save.mock.calls.map((call) => call[0].kind).sort()).toEqual([
    "audio",
    "image",
    "video",
  ]);
});
it("requires fresh confirmation when more files are selected after a completed batch", async () => {
  upload.mockResolvedValue(receipt());
  mount([file("first.png")]);
  confirm();
  await screen.findByText("已就绪");
  fireEvent.change(screen.getByLabelText("选择素材文件"), {
    target: { files: [file("second.png")] },
  });
  expect(
    (screen.getByRole("button", { name: "开始上传" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(screen.getByRole("checkbox").getAttribute("aria-checked")).toBe(
    "false",
  );
  expect(upload).toHaveBeenCalledTimes(1);
  confirm();
  await waitFor(() => expect(upload).toHaveBeenCalledTimes(2));
  expect(upload.mock.calls[0][3].headers["Idempotency-Key"]).not.toBe(
    upload.mock.calls[1][3].headers["Idempotency-Key"],
  );
});
