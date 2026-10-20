import {
  render,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CanvasImageDialog } from "./dialog";
import { canvasMediaSource } from "./source";
import { processCanvasImage } from "./operations";
import { uploadMediaAsset } from "@/api/media";
import { RequestError } from "@/lib/request";
vi.mock("./source", () => ({ canvasMediaSource: vi.fn() }));
vi.mock("./operations", () => ({ processCanvasImage: vi.fn() }));
vi.mock("@/api/media", () => ({ uploadMediaAsset: vi.fn() }));
vi.mock("next/image", () => ({ default: () => <span>原图预览</span> }));
const project = crypto.randomUUID(),
  asset = crypto.randomUUID();
beforeEach(() => {
  vi.resetAllMocks();
  vi.stubGlobal(
    "URL",
    class extends URL {
      static createObjectURL = vi.fn(() => "blob:preview");
      static revokeObjectURL = vi.fn();
    },
  );
  vi.mocked(canvasMediaSource).mockResolvedValue({
    blob: new Blob(["image"]),
    asset: { id: asset, project_id: project },
  });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("部分上传失败只重试原文件和原幂等键，不重复处理或重复加入画布", async () => {
  const created = vi.fn(),
    close = vi.fn();
  const files = [new File(["a"], "a.png"), new File(["b"], "b.png")];
  const first = { id: crypto.randomUUID(), project_id: project, kind: "image" },
    second = { id: crypto.randomUUID(), project_id: project, kind: "image" };
  vi.mocked(processCanvasImage).mockResolvedValue(files);
  vi.mocked(uploadMediaAsset)
    .mockResolvedValueOnce({ asset: first })
    .mockRejectedValueOnce(new Error("connection lost"))
    .mockResolvedValueOnce({ asset: second });
  render(
    <CanvasImageDialog
      projectID={project}
      assetID={asset}
      kind="split"
      availableSlots={20}
      onClose={close}
      onCreated={created}
    />,
  );
  await screen.findByText("原图预览");
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "保存并添加 4 张图片" }));
  await screen.findByRole("button", { name: "重试原任务" });
  expect(created).not.toHaveBeenCalled();
  expect(screen.getByLabelText("行数").hasAttribute("disabled")).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "重试原任务" }));
  await waitFor(() => expect(created).toHaveBeenCalledWith([first, second]));
  expect(processCanvasImage).toHaveBeenCalledOnce();
  const calls = vi.mocked(uploadMediaAsset).mock.calls;
  expect(calls[1][2]).toBe(files[1]);
  expect(calls[2][2]).toBe(files[1]);
  expect(calls[1][3]?.headers).toEqual(calls[2][3]?.headers);
  expect(calls[0][3]?.headers).not.toEqual(calls[1][3]?.headers);
  expect(close).toHaveBeenCalledOnce();
});
it("结果超过剩余节点容量时不能生成或上传", async () => {
  render(
    <CanvasImageDialog
      projectID={project}
      assetID={asset}
      kind="split"
      availableSlots={3}
      onClose={vi.fn()}
      onCreated={vi.fn()}
    />,
  );
  await screen.findByText("原图预览");
  fireEvent.click(screen.getByRole("checkbox"));
  expect(
    screen
      .getByRole("button", { name: "保存并添加 4 张图片" })
      .hasAttribute("disabled"),
  ).toBe(true);
  expect(processCanvasImage).not.toHaveBeenCalled();
  expect(uploadMediaAsset).not.toHaveBeenCalled();
});

it("加入画布失败保留上传回执，重试添加不重复上传", async () => {
  const created = vi.fn().mockReturnValueOnce(false).mockReturnValue(true),
    close = vi.fn();
  const result = {
    id: crypto.randomUUID(),
    project_id: project,
    kind: "image",
  };
  vi.mocked(processCanvasImage).mockResolvedValue([new File(["a"], "a.png")]);
  vi.mocked(uploadMediaAsset).mockResolvedValue({ asset: result });
  render(
    <CanvasImageDialog
      projectID={project}
      assetID={asset}
      kind="crop"
      availableSlots={20}
      onClose={close}
      onCreated={created}
    />,
  );
  await screen.findByText("原图预览");
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "保存并添加 1 张图片" }));
  await screen.findByRole("button", { name: "重试原任务" });
  expect(close).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "重试原任务" }));
  await waitFor(() => expect(close).toHaveBeenCalledOnce());
  expect(created).toHaveBeenLastCalledWith([result]);
  expect(uploadMediaAsset).toHaveBeenCalledOnce();
  expect(processCanvasImage).toHaveBeenCalledOnce();
});
it("原图加载失败结束加载提示并可重试，不展示内部错误码", async () => {
  vi.mocked(canvasMediaSource).mockRejectedValueOnce(
    new RequestError("invalid_receipt"),
  );
  render(
    <CanvasImageDialog
      projectID={project}
      assetID={asset}
      kind="crop"
      availableSlots={20}
      onClose={vi.fn()}
      onCreated={vi.fn()}
    />,
  );
  await screen.findByText("原图加载失败");
  expect(screen.queryByText("正在读取原图…")).toBeNull();
  expect(screen.queryByText("invalid_receipt")).toBeNull();
  expect(
    screen
      .getByRole("button", { name: "保存并添加 1 张图片" })
      .hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "重新读取原图" }));
  await screen.findByText("原图预览");
  expect(canvasMediaSource).toHaveBeenCalledTimes(2);
  expect(uploadMediaAsset).not.toHaveBeenCalled();
});
