import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { MediaExportPanel } from "./media-export-panel";
import { createTimeline } from "./timeline";
import type { ExportJob, ExportSource } from "./media-export-model";

const api = vi.hoisted(() => ({
  create: vi.fn(),
  list: vi.fn(),
  download: vi.fn(),
}));
vi.mock("@/api/mediaExports", () => ({ downloadMediaExport: api.download }));
vi.mock("./media-export-queries", () => ({
  MEDIA_EXPORTS_KEY: ["canvas", "media-exports"],
  createTimelineExport: api.create,
  listTimelineExports: api.list,
  controlTimelineExport: vi.fn(),
  downloadTimelineSubtitles: vi.fn(),
}));
vi.mock("./media-export-preview-dialog", () => ({
  MediaExportPreviewDialog: () => null,
}));
const projectId = "bf8a08d7-0e46-4d99-9623-772c09ea5fbe";
const source: ExportSource = {
  canvas_id: "b1bbf67e-7f6a-44d2-9ae3-bd69ea84256d",
  node_id: "d5191fa9-b09c-40cd-9a92-62dc926d895c",
  revision: 7,
};
const clients: QueryClient[] = [];
function show(save: () => Promise<boolean>, current: () => ExportSource) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  clients.push(client);
  const draft = createTimeline();
  draft.clips = [
    {
      id: "00000000-0000-4000-8000-000000000004",
      trackId: draft.tracks.find((track) => track.kind === "video")!.id,
      kind: "video",
      nodeId: "00000000-0000-4000-8000-000000000005",
      assetId: null,
      title: "实际视频",
      startMs: 0,
      durationMs: 1500,
      sourceStartMs: 500,
      sourceDurationMs: 3000,
      volume: 1,
      fadeInMs: 0,
      fadeOutMs: 0,
      text: "",
    },
  ];
  render(
    <QueryClientProvider client={client}>
      <MediaExportPanel
        projectId={projectId}
        canvasId={source.canvas_id}
        nodeId={source.node_id}
        draft={draft}
        source={current}
        disabled={false}
        onPersist={save}
        onImport={vi.fn()}
        onBusy={vi.fn()}
      />
    </QueryClientProvider>,
  );
}
beforeEach(() => api.list.mockResolvedValue({ items: [], next_cursor: null }));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.resetAllMocks();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

const reviewedJob: ExportJob = {
  id: "00000000-0000-4000-8000-000000000010",
  project_id: projectId,
  source,
  output_kind: "video",
  status: "succeeded",
  stage: "completed",
  progress: 100,
  attempt: 1,
  revision: 2,
  asset_id: "00000000-0000-4000-8000-000000000011",
  sha256: "a".repeat(64),
  failure_code: null,
  created_at: "2026-10-02T00:00:00Z",
  updated_at: "2026-10-02T00:00:00Z",
};
describe("审核后输出下载", () => {
  it.each([
    ["video", "video/mp4", "MP4", "mp4"],
    ["audio", "audio/mp4", "M4A", "m4a"],
  ] as const)(
    "%s使用生成API取得Blob，下载后释放ObjectURL",
    async (kind, mime, label, extension) => {
      api.list.mockResolvedValue({
        items: [{ ...reviewedJob, output_kind: kind }],
        next_cursor: null,
      });
      const blob = new Blob(["实际成片字节"], { type: mime });
      api.download.mockResolvedValue(blob);
      const createObjectURL = vi
        .spyOn(URL, "createObjectURL")
        .mockReturnValue("blob:reviewed-output");
      const revokeObjectURL = vi
        .spyOn(URL, "revokeObjectURL")
        .mockImplementation(() => {});
      const click = vi
        .spyOn(HTMLAnchorElement.prototype, "click")
        .mockImplementation(() => {});
      show(vi.fn(), () => source);
      const button = await screen.findByRole("button", {
        name: `下载 ${label}`,
      });
      vi.useFakeTimers();
      await act(async () => fireEvent.click(button));
      expect(api.download).toHaveBeenCalledExactlyOnceWith(
        { job_id: reviewedJob.id, project_id: projectId },
        { responseType: "blob", timeout: 0 },
      );
      expect(createObjectURL).toHaveBeenCalledExactlyOnceWith(blob);
      expect(click).toHaveBeenCalledOnce();
      const anchor = click.mock.instances[0] as HTMLAnchorElement;
      expect(anchor.href).toBe("blob:reviewed-output");
      expect(anchor.download).toBe(`timeline-${reviewedJob.id}.${extension}`);
      expect(revokeObjectURL).not.toHaveBeenCalled();
      act(() => vi.advanceTimersByTime(1000));
      expect(revokeObjectURL).toHaveBeenCalledExactlyOnceWith(
        "blob:reviewed-output",
      );
    },
  );
  it.each([
    new Blob([], { type: "video/mp4" }),
    new Blob(["错误文档"], { type: "application/json" }),
    "未解析的响应",
  ])("无效输出不制造下载链接 %j", async (response) => {
    api.list.mockResolvedValue({ items: [reviewedJob], next_cursor: null });
    api.download.mockResolvedValue(response);
    const objectURL = vi.spyOn(URL, "createObjectURL");
    show(vi.fn(), () => source);
    fireEvent.click(await screen.findByRole("button", { name: "下载 MP4" }));
    await screen.findByRole("alert");
    expect(objectURL).not.toHaveBeenCalled();
  });
  it("尚未审核的输出不提供下载操作", async () => {
    api.list.mockResolvedValue({
      items: [{ ...reviewedJob, status: "review_required" }],
      next_cursor: null,
    });
    show(vi.fn(), () => source);
    await screen.findByRole("button", { name: "检查并审核成片" });
    expect(screen.queryByRole("button", { name: "下载 MP4" })).toBeNull();
    expect(api.download).not.toHaveBeenCalled();
  });
});
describe("正式导出冻结与幂等请求", () => {
  it("时间线保存未确认时不创建导出任务", async () => {
    const save = vi.fn().mockResolvedValue(false);
    show(save, () => source);
    fireEvent.click(screen.getByRole("button", { name: "保存并导出 MP4" }));
    await screen.findByText("时间线尚未确认保存，导出未启动。");
    expect(save).toHaveBeenCalledOnce();
    expect(api.create).not.toHaveBeenCalled();
  });
  it("未知结果重放原修订和幂等键，草稿后续变更不形成第二次导出", async () => {
    let revision = source.revision;
    const save = vi.fn(async () => {
      revision = 9;
      return true;
    });
    api.create.mockRejectedValueOnce(new ApiError(0, "network_error"));
    api.create.mockResolvedValueOnce({ source: { ...source, revision: 9 } });
    show(save, () => ({ ...source, revision }));
    fireEvent.click(screen.getByRole("button", { name: "保存并导出 MP4" }));
    const replay = await screen.findByRole("button", {
      name: "核验原导出创建请求",
    });
    expect(api.create).toHaveBeenCalledOnce();
    const original = api.create.mock.calls[0];
    expect(original).toEqual([
      projectId,
      { ...source, revision: 9 },
      expect.stringMatching(/^[a-f0-9-]{36}$/),
      "video",
    ]);
    revision = 12;
    fireEvent.click(replay);
    await waitFor(() => expect(api.create).toHaveBeenCalledTimes(2));
    await screen.findByText("已创建导出任务，使用画布修订 9。");
    expect(api.create.mock.calls[1]).toEqual(original);
    expect(save).toHaveBeenCalledOnce();
  });
});
