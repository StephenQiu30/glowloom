import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { exportJobSchema } from "./media-export-model";
import { MediaExportPreviewDialog } from "./media-export-preview-dialog";

const api = vi.hoisted(() => ({ preview: vi.fn(), review: vi.fn() }));
vi.mock("./media-export-queries", () => ({
  MEDIA_EXPORTS_KEY: ["canvas", "media-exports"],
  previewTimelineExport: api.preview,
  reviewTimelineExport: api.review,
}));
const job = exportJobSchema.parse({
  id: "282e0c06-905c-4c23-b2b8-42e91d2d092b",
  project_id: "bf8a08d7-0e46-4d99-9623-772c09ea5fbe",
  source: {
    canvas_id: "b1bbf67e-7f6a-44d2-9ae3-bd69ea84256d",
    node_id: "d5191fa9-b09c-40cd-9a92-62dc926d895c",
    revision: 8,
  },
  status: "review_required",
  stage: "owner_review",
  progress: 95,
  attempt: 1,
  revision: 4,
  asset_id: "a5f427e1-e35e-4d86-b0ea-de8a96cc4be7",
  sha256: "a".repeat(64),
  failure_code: null,
  created_at: "2026-10-01T14:00:00Z",
  updated_at: "2026-10-01T14:01:00Z",
});
const preview = {
  job_id: job.id,
  revision: 4,
  sha256: job.sha256,
  url: "https://example.test/private/output.mp4",
  expires_at: new Date(Date.now() + 600000).toISOString(),
  asset: {
    id: job.asset_id,
    project_id: job.project_id,
    kind: "video",
    file_name: "output.mp4",
    mime_type: "video/mp4",
    byte_size: 3000,
    width: 1280,
    height: 720,
    duration_ms: 1500,
    revision: 1,
  },
};
const clients: QueryClient[] = [];
function show() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  clients.push(client);
  const reviewed = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <MediaExportPreviewDialog
        job={job}
        readOnly={false}
        onClose={vi.fn()}
        onReviewed={reviewed}
        onBusy={vi.fn()}
      />
    </QueryClientProvider>,
  );
  return reviewed;
}
beforeEach(() => {
  api.preview.mockResolvedValue(preview);
  api.review.mockResolvedValue({ ...job, status: "succeeded" });
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
  vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.resetAllMocks();
  vi.restoreAllMocks();
});
describe("实际导出成片人工审核交互", () => {
  it("真实媒体解码且用户复核后才能审核，不把preview授权当审核成功", async () => {
    const reviewed = show();
    await waitFor(() => expect(document.querySelector("video")).not.toBeNull());
    const button = screen.getByRole("button", {
      name: "确认审核成片",
    }) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    const video = document.querySelector("video")!;
    Object.defineProperties(video, {
      videoWidth: { value: 1280 },
      videoHeight: { value: 720 },
      duration: { value: 1.5 },
    });
    fireEvent.loadedData(video);
    expect(button.disabled).toBe(true);
    fireEvent.click(
      screen.getByRole("checkbox", { name: "我已检查实际成片并确认可用" }),
    );
    expect(button.disabled).toBe(false);
    fireEvent.click(button);
    await waitFor(() => expect(reviewed).toHaveBeenCalledOnce());
    expect(api.review).toHaveBeenCalledWith(
      job,
      preview,
      expect.stringMatching(/^[a-f0-9-]{36}$/),
    );
  });
  it("刷新授权失败释放旧来源，即使缓存仍保留旧输出也不能继续审核", async () => {
    show();
    await waitFor(() => expect(document.querySelector("video")).not.toBeNull());
    const video = document.querySelector("video")!;
    fireEvent.error(video);
    api.preview.mockRejectedValueOnce(new Error("授权拒绝"));
    fireEvent.click(
      await screen.findByRole("button", { name: "重新读取成片" }),
    );
    await waitFor(() => expect(document.querySelector("video")).toBeNull());
    expect(video.hasAttribute("src")).toBe(false);
    expect(
      (
        screen.getByRole("button", {
          name: "确认审核成片",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    expect(api.review).not.toHaveBeenCalled();
  });
});
