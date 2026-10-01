import axios from "axios";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  controlMediaTranscription,
  createMediaTranscription,
  downloadMediaTranscriptionSubtitles,
  getMediaTranscriptionResult,
  listMediaTranscriptions,
} from "./queries";
import type { TranscriptionJob } from "./transcription-model";

const id = (n: number) =>
  `00000000-0000-4000-8000-${String(n).padStart(12, "0")}`;
const source = { canvas_id: id(2), node_id: id(3), revision: 4 };
const job: TranscriptionJob = {
  id: id(4),
  project_id: id(1),
  source,
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
const result = {
  job_id: job.id,
  revision: 6,
  sha256: job.result_sha256,
  source_asset_id: id(5),
  source_asset_revision: 1,
  source_sha256: "b".repeat(64),
  draft: {
    version: 1,
    language: "chinese",
    duration_ms: 1000,
    segments: [{ start_ms: 0, end_ms: 1000, text: "实际字幕稿" }],
  },
  srt: "1\n00:00:00,000 --> 00:00:01,000\n实际字幕稿\n",
};
afterEach(() => vi.restoreAllMocks());
describe("转写生成客户端消费边界", () => {
  it("创建仅发送冻结来源与语言，携带调用方原幂等键", async () => {
    const send = vi.spyOn(axios, "request").mockResolvedValue({ data: job });
    expect(
      await createMediaTranscription(id(1), source, "auto", id(9)),
    ).toEqual(job);
    const sent = send.mock.calls[0][0];
    expect(sent.url).toBe(`/api/projects/${id(1)}/media-transcriptions`);
    expect(sent.data).toEqual({ ...source, language: "auto" });
    expect(sent.headers).toMatchObject({ "Idempotency-Key": id(9) });
  });
  it("拒绝创建回执指向别的项目或来源修订", async () => {
    const send = vi.spyOn(axios, "request");
    for (const invalid of [
      { ...job, project_id: id(99) },
      { ...job, source: { ...source, revision: 99 } },
    ]) {
      send.mockResolvedValue({ data: invalid });
      await expect(
        createMediaTranscription(id(1), source, "auto", id(9)),
      ).rejects.toMatchObject({ code: "invalid_response" });
    }
  });
  it("分页与取消信号归入当前项目画布；跨范围页面拒绝", async () => {
    const send = vi
      .spyOn(axios, "request")
      .mockResolvedValue({ data: { items: [job], next_cursor: "next" } });
    const signal = new AbortController().signal;
    expect(
      await listMediaTranscriptions(id(1), id(2), id(3), "cursor", signal),
    ).toEqual({ items: [job], next_cursor: "next" });
    expect(send.mock.calls[0][0]).toMatchObject({
      signal,
      params: { canvas_id: id(2), node_id: id(3), limit: 25, cursor: "cursor" },
    });
    send.mockResolvedValue({
      data: {
        items: [{ ...job, source: { ...source, node_id: id(99) } }],
        next_cursor: null,
      },
    });
    await expect(
      listMediaTranscriptions(id(1), id(2), id(3)),
    ).rejects.toMatchObject({ code: "invalid_response" });
  });
  it("结果同时校验成功任务身份、修订、SHA和时间戳", async () => {
    const send = vi.spyOn(axios, "request").mockResolvedValue({ data: result });
    expect(await getMediaTranscriptionResult(job)).toEqual(result);
    for (const invalid of [
      { ...result, revision: 7 },
      { ...result, sha256: "c".repeat(64) },
      {
        ...result,
        draft: {
          ...result.draft,
          segments: [{ end_ms: 1000, text: "缺少起点" }],
        },
      },
    ]) {
      send.mockResolvedValue({ data: invalid });
      await expect(getMediaTranscriptionResult(job)).rejects.toMatchObject({
        code: "invalid_response",
      });
    }
  });
  it("控制命令使用原任务修订与key，拒绝串任务回执", async () => {
    const send = vi.spyOn(axios, "request").mockResolvedValue({
      data: { ...job, revision: 7, status: "cancel_requested" },
    });
    await controlMediaTranscription(job, "cancel", id(9));
    expect(send.mock.calls[0][0]).toMatchObject({
      url: `/api/media-transcriptions/${job.id}/cancel`,
      data: { project_id: id(1), revision: 6 },
    });
    expect(send.mock.calls[0][0].headers).toMatchObject({
      "Idempotency-Key": id(9),
    });
    send.mockResolvedValue({ data: { ...job, id: id(99) } });
    await expect(
      controlMediaTranscription(job, "retry", id(9)),
    ).rejects.toMatchObject({ code: "invalid_response" });
  });
  it("SRT使用正式下载端点，仅接受实际有界字幕Blob", async () => {
    const blob = new Blob([result.srt], {
      type: "application/x-subrip;charset=utf-8",
    });
    const send = vi.spyOn(axios, "request").mockResolvedValue({ data: blob });
    expect(await downloadMediaTranscriptionSubtitles(job)).toBe(blob);
    expect(send.mock.calls[0][0]).toMatchObject({
      responseType: "blob",
      params: { project_id: id(1) },
    });
    send.mockResolvedValue({ data: new Blob(["HTML"], { type: "text/html" }) });
    await expect(
      downloadMediaTranscriptionSubtitles(job),
    ).rejects.toMatchObject({ code: "invalid_response" });
  });
});
