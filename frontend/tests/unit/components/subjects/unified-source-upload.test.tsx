import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useRef, useState } from "react";
import { UnifiedSubjectSourceUpload } from "@/components/subjects/unified-source-upload";
import { RequestError } from "@/lib/request";
const api = vi.hoisted(() => ({ upload: vi.fn(), scope: 0 }));
vi.mock("@/lib/request", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/request")>()),
  getScopeGeneration: () => api.scope,
}));
vi.mock("@/api/subjects", () => ({ uploadSubjectSource: api.upload }));
const file = new File(["bytes"], "参考.png", { type: "image/png" });
const asset = {
  id: "11111111-1111-4111-8111-111111111111",
  revision: 1,
  kind: "image",
  mime_type: "image/png",
  file_name: "参考.png",
};
beforeEach(() => {
  vi.resetAllMocks();
  api.scope = 0;
});
afterEach(cleanup);
it("上传前要求权限确认，未知结果沿专用来源接口同键重试", async () => {
  api.upload
    .mockRejectedValueOnce(new Error("network"))
    .mockResolvedValueOnce({ asset });
  const onUploaded = vi.fn();
  render(
    <UnifiedSubjectSourceUpload
      allowedKinds={["image", "video"]}
      initialFile={file}
      onClose={vi.fn()}
      onUploaded={onUploaded}
    />,
  );
  expect(screen.getByRole("button", { name: "上传" })).toHaveProperty(
    "disabled",
    true,
  );
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "上传" }));
  const retry = await screen.findByRole("button", { name: "重试原上传" });
  expect(screen.getByRole("button", { name: "返回设置" })).toHaveProperty(
    "disabled",
    false,
  );
  fireEvent.click(retry);
  await waitFor(() =>
    expect(onUploaded).toHaveBeenCalledWith(
      expect.objectContaining({
        asset_id: asset.id,
        library_source: "subject",
      }),
    ),
  );
  expect(api.upload.mock.calls[0][0]).toEqual({ local_review_confirmed: true });
  expect(api.upload.mock.calls[0][1]).toBe(file);
  expect(api.upload.mock.calls[0][2].headers["Idempotency-Key"]).toBe(
    api.upload.mock.calls[1][2].headers["Idempotency-Key"],
  );
});

it("未知上传可以返回设置再恢复，同一File与key保留且恢复不会自动提交", async () => {
  api.upload
    .mockRejectedValueOnce(new Error("network"))
    .mockResolvedValueOnce({ asset });
  const uploaded = vi.fn();
  function Owner() {
    const [open, setOpen] = useState(true);
    const resume = useRef<HTMLButtonElement>(null);
    return (
      <>
        {!open ? (
          <button ref={resume} onClick={() => setOpen(true)}>
            继续确认上传
          </button>
        ) : null}
        <UnifiedSubjectSourceUpload
          open={open}
          allowedKinds={["image"]}
          initialFile={file}
          onSuspend={() => setOpen(false)}
          onReturnFocus={() => resume.current?.focus()}
          onClose={() => setOpen(false)}
          onUploaded={uploaded}
        />
      </>
    );
  }
  render(<Owner />);
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "上传" }));
  await screen.findByRole("button", { name: "重试原上传" });
  fireEvent.click(screen.getByRole("button", { name: "返回设置" }));
  expect(screen.queryByRole("dialog")).toBeNull();
  const resume = screen.getByRole("button", { name: "继续确认上传" });
  await waitFor(() => expect(document.activeElement).toBe(resume));
  fireEvent.click(resume);
  expect(api.upload).toHaveBeenCalledOnce();
  expect(screen.getByText("参考.png")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "重试原上传" }));
  await waitFor(() => expect(uploaded).toHaveBeenCalledOnce());
  expect(api.upload.mock.calls[0][1]).toBe(api.upload.mock.calls[1][1]);
  expect(api.upload.mock.calls[0][2].headers["Idempotency-Key"]).toBe(
    api.upload.mock.calls[1][2].headers["Idempotency-Key"],
  );
});
it("补充图片预筛阻止音频文件上传，主参考支持图片/视频", () => {
  render(
    <UnifiedSubjectSourceUpload
      allowedKinds={["image"]}
      initialFile={new File(["sound"], "声音.wav", { type: "audio/wav" })}
      onClose={vi.fn()}
      onUploaded={vi.fn()}
    />,
  );
  expect(screen.getByRole("alert").textContent).toContain("补充参考仅支持图片");
  expect(screen.getByRole("button", { name: "上传" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(api.upload).not.toHaveBeenCalled();
});
it("重复素材的选择错误在当前上传框显示，允许换文件", async () => {
  api.upload.mockResolvedValue({ asset });
  render(
    <UnifiedSubjectSourceUpload
      allowedKinds={["image"]}
      initialFile={file}
      onClose={vi.fn()}
      onUploaded={() => "此图片已经在参考主体中。"}
    />,
  );
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "上传" }));
  await screen.findByText("此图片已经在参考主体中。");
  expect(screen.getByLabelText("选择主体参考文件")).toHaveProperty(
    "disabled",
    false,
  );
});

it("拖入文件显示反馈，移出清除，放下只选择而不自动上传", () => {
  render(
    <UnifiedSubjectSourceUpload
      allowedKinds={["image"]}
      onClose={vi.fn()}
      onUploaded={vi.fn()}
    />,
  );
  const zone = screen.getByRole("group", { name: "文件上传区域" });
  const transfer = { files: [file], types: ["Files"], dropEffect: "none" };
  fireEvent.dragEnter(zone, { dataTransfer: transfer });
  expect(screen.getByText("松开即可添加")).toBeTruthy();
  fireEvent.dragLeave(zone, { relatedTarget: document.body });
  expect(screen.queryByText("松开即可添加")).toBeNull();
  fireEvent.dragEnter(zone, { dataTransfer: transfer });
  fireEvent.drop(zone, { dataTransfer: transfer });
  expect(screen.getByText("参考.png")).toBeTruthy();
  expect(screen.queryByText("松开即可添加")).toBeNull();
  expect(api.upload).not.toHaveBeenCalled();
});

it("多文件拖入报错且不替换已有文件", () => {
  render(
    <UnifiedSubjectSourceUpload
      allowedKinds={["image"]}
      initialFile={file}
      onClose={vi.fn()}
      onUploaded={vi.fn()}
    />,
  );
  fireEvent.drop(screen.getByRole("group", { name: "文件上传区域" }), {
    dataTransfer: { files: [file, file] },
  });
  expect(screen.getByRole("alert").textContent).toContain("请每次上传一个文件");
  expect(screen.getByText("参考.png")).toBeTruthy();
  expect(api.upload).not.toHaveBeenCalled();
});

it("未知结果只显示一条状态，拖入不能改动原文件和重试请求", async () => {
  api.upload.mockRejectedValue(new Error("network"));
  render(
    <UnifiedSubjectSourceUpload
      allowedKinds={["image"]}
      initialFile={file}
      onClose={vi.fn()}
      onUploaded={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "上传" }));
  await screen.findByRole("button", { name: "重试原上传" });
  const zone = screen.getByRole("group", { name: "文件上传区域" });
  fireEvent.dragEnter(zone, { dataTransfer: { types: ["Files"] } });
  fireEvent.drop(zone, { dataTransfer: { files: [file, file] } });
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.getByRole("status").textContent).toBe(
    "结果待确认，重试不会重复上传。",
  );
  expect(screen.queryByText("松开即可添加")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试原上传" }));
  await waitFor(() => expect(api.upload).toHaveBeenCalledTimes(2));
  expect(api.upload.mock.calls[1][1]).toBe(file);
  expect(api.upload.mock.calls[1][2].headers["Idempotency-Key"]).toBe(
    api.upload.mock.calls[0][2].headers["Idempotency-Key"],
  );
});

it("权限拒绝显示上传上下文，保留更换文件且不标记为未知结果", async () => {
  api.upload.mockRejectedValue(new RequestError("forbidden", 403));
  render(
    <UnifiedSubjectSourceUpload
      allowedKinds={["image"]}
      initialFile={file}
      onClose={vi.fn()}
      onUploaded={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "上传" }));
  await screen.findByText("当前空间没有上传权限。");
  expect(screen.queryByRole("button", { name: "重试原上传" })).toBeNull();
  expect(screen.getByRole("button", { name: "更换上传文件" })).toHaveProperty(
    "disabled",
    false,
  );
});

it("多图部分成功后重试只发送原失败项，保留File、顺序和幂等key", async () => {
  const second = new File(["second"], "补充.png", { type: "image/png" });
  const secondAsset = {
    ...asset,
    id: "22222222-2222-4222-8222-222222222222",
    file_name: second.name,
  };
  api.upload
    .mockResolvedValueOnce({ asset })
    .mockRejectedValueOnce(new Error("network"))
    .mockResolvedValueOnce({ asset: secondAsset });
  const onUploaded = vi.fn(),
    onUploadedMany = vi.fn();
  function Owner() {
    const [open, setOpen] = useState(true);
    return (
      <>
        {!open ? (
          <button onClick={() => setOpen(true)}>继续本批上传</button>
        ) : null}
        <UnifiedSubjectSourceUpload
          open={open}
          onSuspend={() => setOpen(false)}
          allowedKinds={["image", "video"]}
          initialFiles={[file, second]}
          maxFiles={121}
          onClose={vi.fn()}
          onUploaded={onUploaded}
          onUploadedMany={onUploadedMany}
        />
      </>
    );
  }
  render(<Owner />);
  expect(api.upload).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "上传" }));
  await screen.findByRole("button", { name: "重试原上传" });
  fireEvent.click(screen.getByRole("button", { name: "返回设置" }));
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "继续本批上传" }));
  expect(api.upload).toHaveBeenCalledTimes(2);
  fireEvent.click(screen.getByRole("button", { name: "重试原上传" }));
  await waitFor(() => expect(onUploadedMany).toHaveBeenCalledOnce());
  expect(onUploaded).not.toHaveBeenCalled();
  expect(
    onUploadedMany.mock.calls[0][0].map(
      (media: { asset_id: string }) => media.asset_id,
    ),
  ).toEqual([asset.id, secondAsset.id]);
  expect(api.upload.mock.calls.map((call) => call[1])).toEqual([
    file,
    second,
    second,
  ]);
  expect(api.upload.mock.calls[1][2].headers["Idempotency-Key"]).toBe(
    api.upload.mock.calls[2][2].headers["Idempotency-Key"],
  );
  expect(api.upload.mock.calls[0][2].headers["Idempotency-Key"]).not.toBe(
    api.upload.mock.calls[1][2].headers["Idempotency-Key"],
  );
});

it.each([
  {
    files: [file, new File(["v"], "视频.mp4", { type: "video/mp4" })],
    maxFiles: 121,
    message: "图片可多选，视频或音频请单独选择。",
  },
  {
    files: [file, new File(["b"], "另一张.png", { type: "image/png" })],
    maxFiles: 1,
    message: "本次最多可添加1个文件。",
  },
])(
  "不提交混合媒体或超出剩余额度的批次 $message",
  ({ files, maxFiles, message }) => {
    render(
      <UnifiedSubjectSourceUpload
        allowedKinds={["image", "video"]}
        initialFiles={files}
        maxFiles={maxFiles}
        onClose={vi.fn()}
        onUploaded={vi.fn()}
        onUploadedMany={vi.fn()}
      />,
    );
    expect(screen.getByRole("alert").textContent).toContain(message);
    fireEvent.click(screen.getByRole("checkbox"));
    expect(screen.getByRole("button", { name: "上传" })).toHaveProperty(
      "disabled",
      true,
    );
    expect(api.upload).not.toHaveBeenCalled();
  },
);

it("上传跨空间后不会采纳回执或继续发送批次余项", async () => {
  let resolve!: (value: unknown) => void;
  api.upload.mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const second = new File(["second"], "补充.png", { type: "image/png" });
  const onUploadedMany = vi.fn(),
    onUploaded = vi.fn();
  render(
    <UnifiedSubjectSourceUpload
      allowedKinds={["image"]}
      initialFiles={[file, second]}
      maxFiles={121}
      onClose={vi.fn()}
      onUploaded={onUploaded}
      onUploadedMany={onUploadedMany}
    />,
  );
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "上传" }));
  await waitFor(() => expect(api.upload).toHaveBeenCalledOnce());
  api.scope += 1;
  resolve({ asset });
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "上传" })).toHaveProperty(
      "disabled",
      false,
    ),
  );
  expect(onUploaded).not.toHaveBeenCalled();
  expect(onUploadedMany).not.toHaveBeenCalled();
  expect(api.upload).toHaveBeenCalledOnce();
});
