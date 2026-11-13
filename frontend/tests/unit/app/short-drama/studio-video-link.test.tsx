import { render, findFeedback } from "../../../feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
const api = vi.hoisted(() => ({ import: vi.fn() }));
vi.mock("@/api/media", async (original) => ({
  ...(await original<typeof import("@/api/media")>()),
  importPersonalVideoLink: api.import,
}));
import { VideoLinkDialog } from "@/app/short-drama/studio-references";
import { RequestError } from "@/lib/request";
afterEach(cleanup);
beforeEach(() => vi.clearAllMocks());
it("空链接、私网或平台伪装地址不会提交，实际解析失败不采用素材", async () => {
  const adopted = vi.fn();
  render(<VideoLinkDialog onClose={vi.fn()} onUploaded={adopted} />);
  const confirm = screen.getByRole("button", {
    name: "确认",
  }) as HTMLButtonElement;
  expect(confirm.disabled).toBe(true);
  fireEvent.change(screen.getByLabelText("参考内容链接"), {
    target: { value: "https://douyin.com.evil.invalid/video" },
  });
  fireEvent.click(screen.getByRole("checkbox"));
  expect(confirm.disabled).toBe(true);
  expect(api.import).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText("参考内容链接"), {
    target: { value: "https://v.douyin.com/reviewed" },
  });
  api.import.mockRejectedValue(
    new RequestError("video_link_unresolvable", 422),
  );
  fireEvent.click(confirm);
  expect((await findFeedback()).textContent).toContain("无法公开读取");
  expect(adopted).not.toHaveBeenCalled();
});
it("回执丢失后复用URL与永久key；后续采用失败仅重试已入库素材", async () => {
  const asset = {
    id: "11111111-1111-4111-8111-111111111111",
    kind: "video",
    revision: 1,
  };
  api.import
    .mockRejectedValueOnce(new Error("lost receipt"))
    .mockResolvedValueOnce({ asset });
  const adopted = vi
    .fn()
    .mockRejectedValueOnce(new Error("library temporarily unavailable"))
    .mockResolvedValueOnce(undefined);
  render(<VideoLinkDialog onClose={vi.fn()} onUploaded={adopted} />);
  fireEvent.change(screen.getByLabelText("参考内容链接"), {
    target: { value: "https://v.douyin.com/reviewed" },
  });
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await findFeedback();
  fireEvent.click(screen.getByRole("button", { name: "重试原导入" }));
  await waitFor(() => expect(adopted).toHaveBeenCalledTimes(1));
  await findFeedback();
  fireEvent.click(screen.getByRole("button", { name: "重试原导入" }));
  await waitFor(() => expect(adopted).toHaveBeenCalledTimes(2));
  expect(api.import).toHaveBeenCalledTimes(2);
  expect(api.import.mock.calls[0][0]).toEqual(api.import.mock.calls[1][0]);
  expect(api.import.mock.calls[0][1].headers).toEqual(
    api.import.mock.calls[1][1].headers,
  );
  expect(adopted.mock.calls[0][0]).toEqual(adopted.mock.calls[1][0]);
});
