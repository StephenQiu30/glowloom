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
import { TranscriptionPanel } from "./transcription-panel";
import { createTimeline } from "./timeline";
import { CanvasNodeType } from "./model";
import type {
  TranscriptionJob,
  TranscriptionResult,
} from "./transcription-model";

const api = vi.hoisted(() => ({
  create: vi.fn(),
  list: vi.fn(),
  result: vi.fn(),
  control: vi.fn(),
  media: vi.fn(),
  canvas: vi.fn(),
  download: vi.fn(),
}));
vi.mock("./queries", () => ({
  MEDIA_TRANSCRIPTIONS_KEY: ["canvas", "media-transcriptions"],
  createMediaTranscription: api.create,
  listMediaTranscriptions: api.list,
  getMediaTranscriptionResult: api.result,
  controlMediaTranscription: api.control,
  downloadMediaTranscriptionSubtitles: api.download,
  getMediaPreview: api.media,
  getCanvas: api.canvas,
}));
const project = "bf8a08d7-0e46-4d99-9623-772c09ea5fbe",
  canvas = "b1bbf67e-7f6a-44d2-9ae3-bd69ea84256d",
  node = "00000000-0000-4000-8000-000000000005",
  asset = "1c115707-5c76-4c96-8b81-15ca9f6cfe63",
  timelineNode = "d5191fa9-b09c-40cd-9a92-62dc926d895c";
const mediaNode = {
  id: node,
  type: CanvasNodeType.Audio,
  title: "正式音频",
  position: { x: 0, y: 0 },
  width: 320,
  height: 220,
  zIndex: 0,
  assetId: asset,
};
const job: TranscriptionJob = {
  id: "de44ee7b-ed23-407e-bc14-c4da6d4a5ebd",
  project_id: project,
  source: { canvas_id: canvas, node_id: node, revision: 9 },
  language: "auto",
  status: "succeeded",
  stage: "complete",
  progress: 100,
  attempt: 1,
  revision: 6,
  result_sha256: "a".repeat(64),
  failure_code: null,
  created_at: "2026-10-02T00:00:00Z",
  updated_at: "2026-10-02T00:01:00Z",
};
const result: TranscriptionResult = {
  job_id: job.id,
  revision: job.revision,
  sha256: job.result_sha256!,
  source_asset_id: asset,
  source_asset_revision: 1,
  source_sha256: "b".repeat(64),
  draft: {
    version: 1,
    language: "chinese",
    duration_ms: 3000,
    segments: [
      { start_ms: 0, end_ms: 1500, text: "第一句" },
      { start_ms: 1500, end_ms: 3000, text: "第二句" },
    ],
  },
  srt: "1\n00:00:00,000 --> 00:00:01,500\n第一句\n",
};
const clients: QueryClient[] = [];
function show(save = vi.fn().mockResolvedValue(true), revision = () => 9) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  clients.push(client);
  const draft = createTimeline();
  draft.clips.push({
    id: "00000000-0000-4000-8000-000000000004",
    trackId: draft.tracks[1].id,
    kind: "audio",
    nodeId: node,
    assetId: asset,
    title: "声音剪裁",
    startMs: 5000,
    durationMs: 2000,
    sourceStartMs: 500,
    sourceDurationMs: 3000,
    volume: 1,
    fadeInMs: 0,
    fadeOutMs: 0,
    text: "",
  });
  const busy = vi.fn(),
    applied = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <TranscriptionPanel
        projectId={project}
        canvasId={canvas}
        nodes={[mediaNode]}
        draft={draft}
        source={() => ({
          canvas_id: canvas,
          node_id: timelineNode,
          revision: revision(),
        })}
        disabled={false}
        onPersist={save}
        onApplied={applied}
        onBusy={busy}
      />
    </QueryClientProvider>,
  );
  return { save, busy, applied, draft };
}
async function select(label: string, name: string) {
  fireEvent.keyDown(screen.getByRole("combobox", { name: label }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("option", { name }));
}
beforeEach(() => {
  api.list.mockResolvedValue({ items: [], next_cursor: null });
  api.result.mockResolvedValue(result);
  api.media.mockResolvedValue({
    asset: {
      id: asset,
      project_id: project,
      kind: "audio",
      file_name: "speech.wav",
      mime_type: "audio/wave",
      byte_size: 5000,
      revision: 1,
      duration_ms: 3000,
    },
  });
  api.canvas.mockResolvedValue({
    id: canvas,
    projectId: project,
    revision: 9,
    nodes: [mediaNode],
  });
  Element.prototype.scrollIntoView = vi.fn();
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.resetAllMocks();
});
describe("字幕提取与真实采用边界", () => {
  it("保存没有确认时不冻结来源或创建转写", async () => {
    const { save } = show(vi.fn().mockResolvedValue(false));
    fireEvent.click(screen.getByRole("button", { name: "保存并提取字幕" }));
    await screen.findByText("时间线尚未确认保存，字幕提取未启动。");
    expect(save).toHaveBeenCalledOnce();
    expect(api.create).not.toHaveBeenCalled();
  });
  it("未知创建持续锁定并重放同一body和key，不按新修订重提", async () => {
    let revision = 7;
    const save = vi.fn(async () => {
      revision = 9;
      return true;
    });
    api.create
      .mockRejectedValueOnce(new ApiError(0, "dependency_unavailable"))
      .mockResolvedValueOnce({
        ...job,
        status: "queued",
        stage: "queued",
        progress: 0,
        revision: 1,
        result_sha256: null,
      });
    const { busy } = show(save, () => revision);
    fireEvent.click(screen.getByRole("button", { name: "保存并提取字幕" }));
    const replay = await screen.findByRole("button", {
      name: "核验原字幕创建请求",
    });
    const original = api.create.mock.calls[0];
    expect(original).toEqual([
      project,
      { canvas_id: canvas, node_id: node, revision: 9 },
      "auto",
      expect.stringMatching(/^[a-f0-9-]{36}$/),
    ]);
    expect(busy).toHaveBeenLastCalledWith(true);
    const event = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
    revision = 12;
    fireEvent.click(replay);
    await waitFor(() => expect(api.create).toHaveBeenCalledTimes(2));
    expect(api.create.mock.calls[1]).toEqual(original);
    expect(save).toHaveBeenCalledOnce();
    await waitFor(() => expect(busy).toHaveBeenLastCalledWith(false));
  });
  it("刷新恢复字幕稿，须明确复核和选轨；CAS未确认不采用", async () => {
    api.list.mockResolvedValue({ items: [job], next_cursor: null });
    const { save, applied } = show(vi.fn().mockResolvedValue(false));
    fireEvent.click(await screen.findByRole("button", { name: "查看字幕稿" }));
    await screen.findByDisplayValue("第一句");
    expect(applied).not.toHaveBeenCalled();
    await select("采用的音视频片段", "声音剪裁");
    await select("采用的字幕轨", "字幕");
    const adopt = screen.getByRole("button", { name: "复核并保存字幕" });
    expect((adopt as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(
      screen.getByRole("checkbox", {
        name: "已复核字幕文字与时间，并同意替换所选字幕轨",
      }),
    );
    fireEvent.click(adopt);
    await screen.findByText("字幕尚未确认保存，请先核验画布保存结果。");
    expect(save).toHaveBeenCalledOnce();
    expect(applied).not.toHaveBeenCalled();
  });
  it("确认后剪裁换算时间、保存成功再采用，编辑文字不改任务事实", async () => {
    api.list.mockResolvedValue({ items: [job], next_cursor: null });
    const { save, applied } = show();
    fireEvent.click(await screen.findByRole("button", { name: "查看字幕稿" }));
    await screen.findByDisplayValue("第一句");
    fireEvent.change(screen.getByRole("textbox", { name: "字幕 1 文字" }), {
      target: { value: "人工修正" },
    });
    await select("采用的音视频片段", "声音剪裁");
    await select("采用的字幕轨", "字幕");
    fireEvent.click(
      screen.getByRole("checkbox", {
        name: "已复核字幕文字与时间，并同意替换所选字幕轨",
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "复核并保存字幕" }));
    await waitFor(() => expect(applied).toHaveBeenCalledOnce());
    const next = save.mock.calls[0][0];
    expect(
      next.clips
        .filter((clip: { kind: string }) => clip.kind === "subtitle")
        .map((clip: { startMs: number; durationMs: number; text: string }) => [
          clip.startMs,
          clip.durationMs,
          clip.text,
        ]),
    ).toEqual([
      [5000, 1000, "人工修正"],
      [6000, 1000, "第二句"],
    ]);
    expect(api.control).not.toHaveBeenCalled();
  });
  it("结果待核验不给重新识别按钮，取消等待仍不显示已停止", async () => {
    api.list.mockResolvedValue({
      items: [
        {
          ...job,
          status: "cancel_requested",
          stage: "awaiting_reconciliation",
          failure_code: "inference_unknown",
          result_sha256: null,
        },
      ],
      next_cursor: null,
    });
    show();
    await screen.findByText(/结果待核验/);
    expect(screen.queryByRole("button", { name: "重新识别" })).toBeNull();
    expect(screen.queryByText("已取消")).toBeNull();
  });
  it("未知取消保留原任务修订和key，直到回执后才解除锁定", async () => {
    const running = {
      ...job,
      status: "running" as const,
      stage: "recognizing",
      progress: 55,
      result_sha256: null,
    };
    api.list.mockResolvedValue({ items: [running], next_cursor: null });
    api.control
      .mockRejectedValueOnce(new ApiError(0, "dependency_unavailable"))
      .mockResolvedValueOnce({
        ...running,
        status: "cancel_requested",
        revision: 7,
      });
    const { busy, save } = show();
    fireEvent.click(
      await screen.findByRole("button", { name: "请求取消识别" }),
    );
    const replay = await screen.findByRole("button", {
      name: "核验原字幕控制请求",
    });
    const original = api.control.mock.calls[0];
    expect(original).toEqual([running, "cancel", expect.any(String)]);
    expect(busy).toHaveBeenLastCalledWith(true);
    fireEvent.click(replay);
    await screen.findByText(
      "已请求取消。正在进行的识别须结束后才能确认已取消。",
    );
    expect(api.control.mock.calls[1]).toEqual(original);
    expect(busy).toHaveBeenLastCalledWith(false);
    expect(save).not.toHaveBeenCalled();
  });
  it("采用前发现正式素材修订变化时保留原时间线", async () => {
    api.list.mockResolvedValue({ items: [job], next_cursor: null });
    const { save, applied } = show();
    fireEvent.click(await screen.findByRole("button", { name: "查看字幕稿" }));
    await screen.findByDisplayValue("第一句");
    await select("采用的音视频片段", "声音剪裁");
    await select("采用的字幕轨", "字幕");
    fireEvent.click(
      screen.getByRole("checkbox", {
        name: "已复核字幕文字与时间，并同意替换所选字幕轨",
      }),
    );
    api.media.mockResolvedValue({
      asset: { id: asset, project_id: project, revision: 2, kind: "audio" },
    });
    fireEvent.click(screen.getByRole("button", { name: "复核并保存字幕" }));
    await screen.findByText("转写原件或画布来源已变化，字幕未采用。");
    expect(save).not.toHaveBeenCalled();
    expect(applied).not.toHaveBeenCalled();
  });
  it("错误的字幕时间显示可理解的提示，不尝试保存或更改原稿", async () => {
    api.list.mockResolvedValue({ items: [job], next_cursor: null });
    const { save, applied } = show();
    fireEvent.click(await screen.findByRole("button", { name: "查看字幕稿" }));
    await screen.findByDisplayValue("第一句");
    await select("采用的音视频片段", "声音剪裁");
    await select("采用的字幕轨", "字幕");
    fireEvent.change(
      screen.getByRole("spinbutton", { name: "字幕 1 终点 ms" }),
      { target: { value: "0" } },
    );
    fireEvent.click(
      screen.getByRole("checkbox", {
        name: "已复核字幕文字与时间，并同意替换所选字幕轨",
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "复核并保存字幕" }));
    await screen.findByText(
      "请检查字幕文字与时间，起终点须有效、不得重叠或超过素材时长。",
    );
    expect(save).not.toHaveBeenCalled();
    expect(applied).not.toHaveBeenCalled();
    expect(result.draft.segments[0].end_ms).toBe(1500);
  });
});
