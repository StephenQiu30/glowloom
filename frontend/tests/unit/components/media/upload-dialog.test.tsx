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
