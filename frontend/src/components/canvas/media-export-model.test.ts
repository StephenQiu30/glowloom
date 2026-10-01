import { describe, expect, it } from "vitest";
import { exportJobSchema, parseExportPreview } from "./media-export-model";

const project = "588bdf3e-ef94-4eaf-a2b4-8ebbef79d91c";
const job = {
  id: "41493ffb-b3cd-41ac-b3d5-2999e31b9c7c",
  project_id: project,
  output_kind: "video",
  source: {
    canvas_id: "c538a2ea-5708-42dc-a8df-9446296b34da",
    node_id: "d6991e21-188d-4b1d-beb6-ad578d4168e4",
    revision: 8,
  },
  status: "review_required",
  stage: "owner_review",
  progress: 95,
  attempt: 1,
  revision: 4,
  asset_id: "954630cf-5a1a-4709-bd68-78e2d471e5d4",
  sha256: "a".repeat(64),
  failure_code: null,
  created_at: "2026-10-01T13:00:00Z",
  updated_at: "2026-10-01T13:01:00Z",
};
function preview() {
  return {
    job_id: job.id,
    revision: 4,
    sha256: job.sha256,
    url: "http://localhost:9000/private/real-output.mp4?signed=own-fixture",
    expires_at: "2099-10-01T13:20:00Z",
    asset: {
      id: job.asset_id,
      project_id: project,
      kind: "video",
      file_name: "real-output.mp4",
      mime_type: "video/mp4",
      byte_size: 3000,
      width: 1280,
      height: 720,
      duration_ms: 1500,
      revision: 1,
    },
  };
}
describe("本地导出输出审核边界", () => {
  it("音频输出只能消费实际 M4A，不能将视频或另一种任务当音频", () => {
    const audioJob = exportJobSchema.parse({ ...job, output_kind: "audio" });
    const value = preview();
    const audio = {
      ...value,
      waveform: {
        url: "https://example.test/actual-waveform.png",
        expires_at: "2099-10-01T13:20:00Z",
        width: 1280,
        height: 256,
      },
      asset: {
        ...value.asset,
        kind: "audio",
        mime_type: "audio/mp4",
        width: undefined,
        height: undefined,
      },
    };
    expect(parseExportPreview(audio, audioJob).asset.kind).toBe("audio");
    expect(() =>
      parseExportPreview({ ...audio, waveform: undefined }, audioJob),
    ).toThrow();
    expect(() =>
      parseExportPreview(
        {
          ...audio,
          waveform: { ...audio.waveform, url: "data:image/png;base64,invalid" },
        },
        audioJob,
      ),
    ).toThrow();
    expect(() => parseExportPreview(value, audioJob)).toThrow();
    expect(() =>
      parseExportPreview(audio, exportJobSchema.parse(job)),
    ).toThrow();
  });
  it("输出审核冻结job修订与SHA，预审核素材修订可以不同", () => {
    const result = parseExportPreview(preview(), exportJobSchema.parse(job));
    expect(result.revision).toBe(4);
    expect(result.asset.revision).toBe(1);
  });
  it("拒绝跨项目素材、不同输出hash和过期授权", () => {
    const value = preview();
    expect(() =>
      parseExportPreview(
        {
          ...value,
          asset: { ...value.asset, project_id: crypto.randomUUID() },
        },
        exportJobSchema.parse(job),
      ),
    ).toThrow();
    expect(() =>
      parseExportPreview(
        { ...value, sha256: "b".repeat(64) },
        exportJobSchema.parse(job),
      ),
    ).toThrow();
    expect(() =>
      parseExportPreview(
        { ...value, expires_at: "2000-01-01T00:00:00Z" },
        exportJobSchema.parse(job),
      ),
    ).toThrow();
    expect(() =>
      parseExportPreview(
        { ...value, url: "data:video/mp4;base64,bad" },
        exportJobSchema.parse(job),
      ),
    ).toThrow();
  });
  it("必须有真实asset/hash才能进入待审核或成功状态，进度单位0..100", () => {
    expect(() => exportJobSchema.parse({ ...job, asset_id: null })).toThrow();
    expect(() => exportJobSchema.parse({ ...job, sha256: null })).toThrow();
    expect(() => exportJobSchema.parse({ ...job, progress: 101 })).toThrow();
    expect(
      exportJobSchema.parse({
        ...job,
        status: "queued",
        progress: 0,
        asset_id: null,
        sha256: null,
      }).status,
    ).toBe("queued");
  });
});
