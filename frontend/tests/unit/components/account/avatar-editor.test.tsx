import { render, findFeedback } from "../../../feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AvatarEditor } from "@/components/account/avatar-editor";
import { request, RequestError } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
beforeEach(() => {
  URL.createObjectURL = vi.fn(() => "blob:local");
  URL.revokeObjectURL = vi.fn();
  vi.mocked(request).mockResolvedValue({ id: "new-asset" });
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it("上传失败保留已绑定头像，不进行绑定；取消选择释放本地 URL", async () => {
  const bind = vi.fn();
  const load = vi.fn().mockResolvedValue({
    url: "https://example.com/current",
    expires_at: new Date(Date.now() + 60000).toISOString(),
  });
  vi.mocked(request).mockRejectedValue(
    new RequestError("temporarily_unavailable", 503),
  );
  render(<AvatarEditor assetID="old-asset" load={load} bind={bind} />);
  fireEvent.change(screen.getByLabelText(/选择头像/), {
    target: { files: [new File(["pixels"], "a.png", { type: "image/png" })] },
  });
  fireEvent.click(screen.getByText("上传并保存头像"));
  await findFeedback();
  expect(bind).not.toHaveBeenCalled();
  expect(screen.getByText("恢复默认头像")).toBeTruthy();
  fireEvent.click(screen.getByText("取消选择"));
  await waitFor(() =>
    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:local"),
  );
});
it("移除绑定发送明确 null，绑定失败仍保留旧头像和重试按钮", async () => {
  const bind = vi
    .fn()
    .mockRejectedValue(new RequestError("revision_conflict", 409));
  render(
    <AvatarEditor
      assetID="old-asset"
      load={async () => ({ url: "https://example.com/current" })}
      bind={bind}
    />,
  );
  fireEvent.click(screen.getByText("恢复默认头像"));
  await findFeedback();
  expect(bind).toHaveBeenCalledWith(null, expect.any(AbortSignal));
  expect(screen.queryByText("已恢复默认头像。")).toBeNull();
  expect(screen.getByText("恢复默认头像")).toBeTruthy();
});
