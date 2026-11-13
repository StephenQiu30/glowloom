import { afterEach, expect, it, vi } from "vitest";
import { toast as sonner } from "sonner";
import { toast } from "@/components/feedback/notification";

afterEach(() => {
  sonner.dismiss();
  vi.restoreAllMocks();
});

it("普通结果4秒，错误6秒，带恢复操作的通知持续保留", () => {
  const success = vi.spyOn(sonner, "success");
  const error = vi.spyOn(sonner, "error");
  toast.success("已保存");
  toast.error("上传失败");
  toast.error("上传失败", {
    id: "upload-1",
    action: { label: "重试", onClick: vi.fn() },
  });
  expect(success.mock.calls[0][1]?.duration).toBe(4000);
  expect(error.mock.calls[0][1]?.duration).toBe(6000);
  expect(error.mock.calls[1][1]).toMatchObject({
    duration: Infinity,
    id: "upload-1",
  });
});

it("同一任务更新后只保留一个通知，进行中不自动消失", () => {
  const loading = vi.spyOn(sonner, "loading");
  toast.loading("正在上传…", { id: "upload-2" });
  expect(loading.mock.calls[0][1]?.duration).toBe(Infinity);
  toast.success("已上传", { id: "upload-2" });
  expect(sonner.getToasts().filter((t) => t.id === "upload-2")).toHaveLength(1);
});
