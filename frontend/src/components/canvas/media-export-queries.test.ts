import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  createTimelineExport,
  listTimelineExports,
  reviewTimelineExport,
} from "./media-export-queries";
import { exportJobSchema } from "./media-export-model";

const api = vi.hoisted(() => ({
  create: vi.fn(),
  list: vi.fn(),
  review: vi.fn(),
}));
vi.mock("@/gen/api/mediaExports", () => ({
  createMediaExport: api.create,
  listMediaExports: api.list,
  reviewMediaExport: api.review,
}));
const project = "2b6d8f26-b5f6-4327-8b67-c78b3c9a2d71";
const source = {
  canvas_id: "40a5b276-2835-4de7-a8d1-31bb5fbdcfb7",
  node_id: "0b4a7f80-502e-4723-b57b-0b2242658666",
  revision: 9,
};
const value = {
  id: "de44ee7b-ed23-407e-bc14-c4da6d4a5ebd",
  project_id: project,
  source,
  status: "review_required",
  stage: "owner_review",
  progress: 95,
  attempt: 1,
  revision: 4,
  asset_id: "1c115707-5c76-4c96-8b81-15ca9f6cfe63",
  sha256: "a".repeat(64),
  failure_code: null,
  created_at: "2026-10-01T14:00:00Z",
  updated_at: "2026-10-01T14:01:00Z",
};
beforeEach(() => vi.resetAllMocks());
describe("正式导出生成客户端边界", () => {
  it("创建只提交保存后的来源，不接受返回另一画布身份的202", async () => {
    const key = crypto.randomUUID();
    api.create.mockResolvedValue(value);
    await createTimelineExport(project, source, key);
    expect(api.create).toHaveBeenCalledWith({ pid: project }, source, {
      headers: { "Idempotency-Key": key },
    });
    api.create.mockResolvedValue({
      ...value,
      source: { ...source, revision: 8 },
    });
    await expect(createTimelineExport(project, source, key)).rejects.toThrow();
  });
  it("列表恢复拒绝越项目与来源边界，不把错误记录当当前节点任务", async () => {
    api.list.mockResolvedValue({
      items: [{ ...value, project_id: crypto.randomUUID() }],
      next_cursor: null,
    });
    await expect(
      listTimelineExports(project, source.canvas_id, source.node_id),
    ).rejects.toThrow();
    api.list.mockResolvedValue({ items: [value], next_cursor: "next-page" });
    expect(
      (await listTimelineExports(project, source.canvas_id, source.node_id))
        .next_cursor,
    ).toBe("next-page");
  });
  it("审核绑定实际输出job修订和SHA，成功之后才允许ready素材消费", async () => {
    const job = exportJobSchema.parse(value);
    const preview = {
      job_id: job.id,
      revision: 4,
      sha256: value.sha256,
      url: "http://localhost:9000/private/output.mp4?token=synthetic",
      expires_at: "2099-10-01T14:20:00Z",
      asset: {
        id: job.asset_id!,
        project_id: project,
        kind: "video" as const,
        file_name: "output.mp4",
        mime_type: "video/mp4" as const,
        byte_size: 3000,
        width: 1280,
        height: 720,
        duration_ms: 1500,
        revision: 1,
      },
    };
    const key = crypto.randomUUID();
    api.review.mockResolvedValue({
      ...value,
      status: "succeeded",
      progress: 100,
      revision: 5,
    });
    await reviewTimelineExport(job, preview, key);
    expect(api.review).toHaveBeenCalledWith(
      { job_id: job.id },
      {
        project_id: project,
        revision: 4,
        sha256: value.sha256,
        local_review_confirmed: true,
      },
      { headers: { "Idempotency-Key": key } },
    );
    api.review.mockResolvedValue({ ...value, status: "review_required" });
    await expect(reviewTimelineExport(job, preview, key)).rejects.toThrow();
  });
});
