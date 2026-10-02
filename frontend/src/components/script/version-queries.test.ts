import { expect, it, vi } from "vitest";
import * as script from "@/gen/api/script";
import { versionSourceText, listConfirmations } from "./version-queries";
vi.mock("@/gen/api/script", () => ({
  getScriptVersionSourceText: vi.fn(),
  listScriptSplitConfirmations: vi.fn(),
}));
const id = "11111111-1111-4111-8111-111111111111";
const scope = {
  projectId: id,
  orgId: id,
  actorId: id,
  origin: "http://127.0.0.1:3000",
};
const version = {
  id,
  version_no: 1,
  content_hash: "a".repeat(64),
  document_sha256: "b".repeat(64),
  source_manifest_sha256: "c".repeat(64),
  char_count: 4,
  source_count: 1,
  created_at: "2026-10-02T00:00:00Z",
};
it("候选规范片段不需formal集ID，仍严格核整版SHA、Unicode字数与原范围", async () => {
  vi.mocked(script.getScriptVersionSourceText).mockResolvedValue({
    script_version_id: id,
    span_start: 1,
    span_end: 3,
    content_hash: version.content_hash,
    text: "😀乙",
  });
  expect((await versionSourceText(scope, version, 1, 3)).text).toBe("😀乙");
  expect(script.getScriptVersionSourceText).toHaveBeenCalledWith(
    { pid: id, vid: id, from: 1, to: 3 },
    expect.anything(),
  );
  vi.mocked(script.getScriptVersionSourceText).mockResolvedValue({
    script_version_id: id,
    span_start: 1,
    span_end: 3,
    content_hash: "d".repeat(64),
    text: "😀乙",
  });
  await expect(versionSourceText(scope, version, 1, 3)).rejects.toThrow();
  await expect(versionSourceText(scope, version, 0, 0)).rejects.toThrow();
});
it("确认历史页必须严格早于原cursor且唯一，重复身份不显示为两次确认", async () => {
  const summary = {
    id,
    revision: 2,
    candidate_set_id: id,
    formal_set_id: id,
    episode_count: 1,
    created_at: version.created_at,
  };
  vi.mocked(script.listScriptSplitConfirmations).mockResolvedValue({
    items: [summary],
    next_revision: 2,
  });
  expect((await listConfirmations(scope, id, 3)).items[0].revision).toBe(2);
  vi.mocked(script.listScriptSplitConfirmations).mockResolvedValue({
    items: [summary, summary],
  });
  await expect(listConfirmations(scope, id, 3)).rejects.toThrow();
});
