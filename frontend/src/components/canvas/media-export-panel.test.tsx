import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { MediaExportPanel } from "./media-export-panel";
import { createTimeline } from "./timeline";
import type { ExportSource } from "./media-export-model";

const api = vi.hoisted(() => ({ create: vi.fn(), list: vi.fn() }));
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
    ]);
    revision = 12;
    fireEvent.click(replay);
    await waitFor(() => expect(api.create).toHaveBeenCalledTimes(2));
    await screen.findByText("已创建导出任务，使用画布修订 9。");
    expect(api.create.mock.calls[1]).toEqual(original);
    expect(save).toHaveBeenCalledOnce();
  });
});
