import { beforeEach, expect, it, vi } from "vitest";
import * as generated from "@/api/script";
import {
  getSource,
  getWorkspace,
  listSources,
  readAllSources,
  requireScriptScope,
  runSourceIntent,
} from "./source-queries";
vi.mock("@/api/script", () => ({
  getScriptWorkspace: vi.fn(),
  listScriptSources: vi.fn(),
  getScriptSource: vi.fn(),
  createScriptSource: vi.fn(),
  updateScriptSource: vi.fn(),
  deleteScriptSource: vi.fn(),
  importScriptSources: vi.fn(),
  reorderScriptSources: vi.fn(),
}));
beforeEach(() => vi.resetAllMocks());
const scope = {
  origin: "http://127.0.0.1:3000",
  actorId: "11111111-1111-4111-8111-111111111111",
  orgId: "22222222-2222-4222-8222-222222222222",
  projectId: "33333333-3333-4333-8333-333333333333",
};
const version = "44444444-4444-4444-8444-444444444444";
const ids = [
  "55555555-5555-4555-8555-555555555555",
  "66666666-6666-4666-8666-666666666666",
];
const summary = (position: number) => ({
  id: ids[position],
  source_lineage_id: ids[position],
  source_revision: 1,
  source_kind: "chapter",
  title: `章${position}`,
  status: "draft",
  origin: "manual",
  position,
  char_count: 1,
  content_hash: "a".repeat(64),
  rich_sha256: "b".repeat(64),
});
it("只调用正式生成GET并校验当前主体和project，正文非法不得缓存为可编辑", async () => {
  const workspace = {
    current_actor_id: scope.actorId,
    current_org_id: scope.orgId,
    state: {
      project_id: scope.projectId,
      org_id: scope.orgId,
      revision: 0,
      updated_at: "0001-01-01T00:00:00Z",
    },
  };
  vi.mocked(generated.getScriptWorkspace).mockResolvedValue(workspace);
  requireScriptScope(await getWorkspace(scope.projectId), scope);
  expect(() =>
    requireScriptScope({ ...workspace, current_actor_id: scope.orgId }, scope),
  ).toThrow();
  vi.mocked(generated.getScriptSource).mockResolvedValue({
    ...summary(0),
    plain_text: "wrong",
  });
  await expect(getSource(scope, ids[0], version)).rejects.toMatchObject({
    code: "invalid_response",
  });
});
it("完整重排先读取同immutable version全部分页，拒重复/跨version；第一页错误可重新读取", async () => {
  vi.mocked(generated.listScriptSources)
    .mockResolvedValueOnce({
      version_id: version,
      items: [summary(0)],
      next_position: 1,
    })
    .mockResolvedValueOnce({ version_id: version, items: [summary(1)] });
  expect(
    (await readAllSources(scope, version)).items.map((item) => item.id),
  ).toEqual(ids);
  expect(generated.listScriptSources).toHaveBeenLastCalledWith(
    { pid: scope.projectId, version_id: version, after: 1, limit: 100 },
    { signal: undefined },
  );
  vi.mocked(generated.listScriptSources)
    .mockResolvedValueOnce({
      version_id: version,
      items: [summary(0)],
      next_position: 1,
    })
    .mockResolvedValueOnce({
      version_id: version,
      items: [{ ...summary(0), position: 1 }],
    });
  await expect(readAllSources(scope, version)).rejects.toMatchObject({
    code: "invalid_response",
  });
  vi.mocked(generated.listScriptSources).mockRejectedValueOnce(
    new Error("unavailable"),
  );
  await expect(listSources(scope, version)).rejects.toThrow("unavailable");
});
it("写入只生成函数/原键原体，旧回执仅返回confirmation，不覆盖较新缓存；非法成功为unknown", async () => {
  const intent = {
    ...scope,
    version: 1 as const,
    key: "77777777-7777-4777-8777-777777777777",
    action: "create" as const,
    body: {
      expected_revision: 0,
      rights_confirmed: true as const,
      source_kind: "chapter" as const,
      title: "章",
      status: "draft" as const,
      document: { type: "doc" as const },
      provenance: {},
    },
  };
  const receipt = {
    script_revision: 1,
    project_revision: 7,
    version_id: version,
    split_set_id: ids[1],
    source_mappings: [{ source_lineage_id: ids[0], new_source_id: ids[0] }],
    changed: true,
    duplicate: false,
  };
  vi.mocked(generated.createScriptSource).mockResolvedValue(receipt);
  expect(await runSourceIntent(intent)).toEqual(receipt);
  expect(generated.createScriptSource).toHaveBeenCalledWith(
    { pid: scope.projectId },
    intent.body,
    expect.objectContaining({
      headers: expect.objectContaining({
        "Idempotency-Key": intent.key,
        Origin: scope.origin,
      }),
    }),
  );
  vi.mocked(generated.createScriptSource).mockResolvedValue({
    ...receipt,
    source_mappings: [],
  });
  await expect(runSourceIntent(intent)).rejects.toMatchObject({
    code: "invalid_response",
  });
});
