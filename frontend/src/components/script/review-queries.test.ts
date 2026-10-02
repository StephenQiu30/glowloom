import { beforeEach, expect, it, vi } from "vitest";
import * as script from "@/gen/api/script";
import {
  episodeSourceText,
  listSourceWrites,
  runSourceControl,
  listVersions,
} from "./review-queries";
vi.mock("@/gen/api/script", () => ({
  getScriptEpisodeSourceText: vi.fn(),
  listScriptSourceWrites: vi.fn(),
  cancelScriptSourceWrite: vi.fn(),
  reconcileScriptSourceWrite: vi.fn(),
  listScriptVersions: vi.fn(),
}));
const scope = {
  origin: "http://127.0.0.1:3000",
  projectId: "11111111-1111-4111-8111-111111111111",
  actorId: "22222222-2222-4222-8222-222222222222",
  orgId: "33333333-3333-4333-8333-333333333333",
};
const eid = "44444444-4444-4444-8444-444444444444";
const vid = "55555555-5555-4555-8555-555555555555";
const episode = {
  id: eid,
  project_id: scope.projectId,
  org_id: scope.orgId,
  script_version_id: vid,
  split_set_id: vid,
  seq_no: 1,
  title: "集",
  span_start: 4,
  span_end: 100000,
  revision: 1,
  inherit_status: "not_inherited",
  is_delete: false,
};
beforeEach(() => vi.clearAllMocks());
it("规范片段拒绝零宽/越界/过量区间，真实Emoji按scalar并拒失效字符串DTO", async () => {
  await expect(episodeSourceText(scope, episode, 4, 4)).rejects.toThrow();
  await expect(episodeSourceText(scope, episode, 4, 70000)).rejects.toThrow();
  expect(script.getScriptEpisodeSourceText).not.toHaveBeenCalled();
  vi.mocked(script.getScriptEpisodeSourceText).mockResolvedValue({
    episode_id: eid,
    script_version_id: vid,
    span_start: 4,
    span_end: 6,
    content_hash: "a".repeat(64),
    text: "😀乙",
  });
  expect((await episodeSourceText(scope, episode, 4, 6)).text).toBe("😀乙");
  vi.mocked(script.getScriptEpisodeSourceText).mockResolvedValue({
    episode_id: eid,
    script_version_id: vid,
    span_start: 4,
    span_end: 6,
    content_hash: "a".repeat(64),
    text: "\ud800乙",
  });
  await expect(episodeSourceText(scope, episode, 4, 6)).rejects.toThrow();
});
it("生成GETjournal匹配当前principal；202控制绑定原意图CAS/key且不当作停止", async () => {
  vi.mocked(script.listScriptSourceWrites).mockResolvedValue({
    current_actor_id: scope.actorId,
    current_org_id: scope.orgId,
    items: [],
  });
  await listSourceWrites(scope);
  expect(script.listScriptSourceWrites).toHaveBeenCalledWith(
    { pid: scope.projectId, after: 0, limit: 25 },
    { signal: undefined },
  );
  const command = {
    intentId: eid,
    action: "cancel" as const,
    body: { expected_revision: 3 },
  };
  vi.mocked(script.cancelScriptSourceWrite).mockResolvedValue({
    intent_id: eid,
    revision: 4,
    action: "cancel",
    accepted: true,
  });
  expect((await runSourceControl(scope, command, vid)).accepted).toBe(true);
  expect(script.cancelScriptSourceWrite).toHaveBeenCalledWith(
    { pid: scope.projectId, wid: eid },
    command.body,
    expect.objectContaining({
      headers: expect.objectContaining({
        "Idempotency-Key": vid,
        Origin: scope.origin,
      }),
    }),
  );
  vi.mocked(script.listScriptVersions).mockResolvedValue({ items: [] });
  await listVersions(scope, 7);
  expect(script.listScriptVersions).toHaveBeenCalledWith(
    { pid: scope.projectId, before_version_no: 7, limit: 25 },
    { signal: undefined },
  );
});
