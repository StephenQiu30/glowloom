import { fireEvent, render, screen, cleanup } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { UploadDialog } from "@/components/media/upload-dialog";
import { request } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
it("上传后归档失败只重试归档，不重复上传原件", async () => {
  vi.mocked(request).mockResolvedValue({
    asset: { id: "33333333-3333-4333-8333-333333333333" },
  });
  const place = vi
    .fn()
    .mockRejectedValueOnce(new Error("placement unavailable"))
    .mockResolvedValue(undefined);
  render(<UploadDialog onClose={() => {}} onUploaded={place} />);
  fireEvent.change(screen.getByLabelText("选择素材文件"), {
    target: {
      files: [
        new File([new Uint8Array([1, 2, 3])], "测试.png", {
          type: "image/png",
        }),
      ],
    },
  });
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "上传" }));
  const retry = await screen.findByRole("button", { name: "重试归档" });
  expect(screen.queryByText("素材上传成功")).toBeNull();
  fireEvent.click(retry);
  await screen.findByText("素材上传成功");
  expect(request).toHaveBeenCalledTimes(1);
  expect(place).toHaveBeenCalledTimes(2);
});

it("素材上传未知结果只保留一条状态且同键重试，拖入不能替换请求", async () => {
  vi.mocked(request)
    .mockRejectedValueOnce(new Error("network"))
    .mockResolvedValueOnce({
      asset: { id: "33333333-3333-4333-8333-333333333333" },
    });
  const file = new File(["fixture"], "素材.png", { type: "image/png" });
  render(
    <UploadDialog initialFile={file} onClose={vi.fn()} onUploaded={vi.fn()} />,
  );
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "上传" }));
  const retry = await screen.findByRole("button", { name: "重试原上传" });
  expect(screen.getByRole("status").textContent).toBe(
    "结果待确认，重试不会重复上传。",
  );
  fireEvent.drop(screen.getByRole("group", { name: "文件上传区域" }), {
    dataTransfer: { files: [file, file] },
  });
  expect(screen.queryByText("请每次上传一个文件。")).toBeNull();
  expect(screen.getByRole("button", { name: "更换上传文件" })).toHaveProperty(
    "disabled",
    true,
  );
  const original = vi.mocked(request).mock.calls[0];
  fireEvent.click(retry);
  await screen.findByText("素材上传成功");
  const retried = vi.mocked(request).mock.calls[1];
  expect(retried[1]?.headers).toEqual(original[1]?.headers);
  expect((retried[1]?.data as FormData).get("file")).toBe(file);
});
