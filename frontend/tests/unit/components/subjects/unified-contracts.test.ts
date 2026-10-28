import { describe, expect, it } from "vitest";
import {
  requireUnifiedSubject,
  requireUnifiedSubjectPage,
  requireSubjectBatchResult,
  subjectDateRange,
} from "@/components/subjects/unified-contracts";
const scope = "11111111-1111-4111-8111-111111111111";
const actor = "22222222-2222-4222-8222-222222222222";
const id = "33333333-3333-4333-8333-333333333333";
const version = "44444444-4444-4444-8444-444444444444";
const media = {
  binding_id: "55555555-5555-4555-8555-555555555555",
  asset_id: "66666666-6666-4666-8666-666666666666",
  asset_revision: 1,
  kind: "image",
  mime_type: "image/png",
  file_name: "参考.png",
};
const subject = {
  id,
  owner_scope_id: scope,
  revision: 1,
  version_id: version,
  name: "海边香氛",
  description: "描述",
  primary: media,
  references: [],
  favorite: false,
  created_at: "2026-10-28T00:00:00Z",
  updated_at: "2026-10-28T00:00:00Z",
};
describe("统一主体边界", () => {
  it("拒绝其他空间回执与不完整媒体事实", () => {
    expect(requireUnifiedSubject(subject, scope)).toEqual(subject);
    expect(() => requireUnifiedSubject(subject, actor)).toThrow();
    expect(() =>
      requireUnifiedSubject(
        { ...subject, primary: { ...media, asset_revision: 0 } },
        scope,
      ),
    ).toThrow();
    expect(() =>
      requireUnifiedSubject(
        { ...subject, voice: { name: "声音", media } },
        scope,
      ),
    ).toThrow();
  });
  it("列表核对账号/空间并拒绝重复主体身份", () => {
    const page = {
      current_actor_id: actor,
      current_scope_id: scope,
      items: [subject],
      page: 1,
      page_size: 12,
      total: 1,
      capabilities: {
        intelligent_voice: {
          available: false,
          reason: "intelligent_voice_unavailable",
        },
      },
    };
    expect(requireUnifiedSubjectPage(page, actor, scope).items).toHaveLength(1);
    expect(() => requireUnifiedSubjectPage(page, scope, scope)).toThrow();
    expect(() =>
      requireUnifiedSubjectPage(
        { ...page, items: [subject, subject] },
        actor,
        scope,
      ),
    ).toThrow();
  });
  it("批量回执必须完整覆盖明确选择，不能将漏项当成功", () => {
    const targets = [{ id, expected_revision: 1 }];
    expect(
      requireSubjectBatchResult(
        { items: [{ id, status: "failed", code: "revision_conflict" }] },
        targets,
      ).items[0].status,
    ).toBe("failed");
    expect(() => requireSubjectBatchResult({ items: [] }, targets)).toThrow();
    expect(() =>
      requireSubjectBatchResult(
        { items: [{ id: actor, status: "succeeded" }] },
        targets,
      ),
    ).toThrow();
  });
  it("日期结束采用下一日exclusive边界，非法或倒置范围不发请求", () => {
    const range = subjectDateRange("2026-10-01", "2026-10-28");
    expect(
      new Date(range.to!).getTime() - new Date(range.from!).getTime(),
    ).toBe(28 * 86400000);
    expect(() => subjectDateRange("2026-10-29", "2026-10-28")).toThrow();
    expect(() => subjectDateRange("2026-02-31", "")).toThrow();
    expect(
      new Date(subjectDateRange("0044-03-15", "").from!).getFullYear(),
    ).toBe(44);
    expect(() => subjectDateRange("0000-01-01", "")).toThrow();
  });
});
