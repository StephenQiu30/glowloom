import { expect, it, vi } from "vitest";
import * as api from "@/gen/api/script";
import { listFileImports, runFileImportIntent } from "./file-import-queries";
vi.mock("@/gen/api/script", () => ({
  listScriptFileImports: vi.fn(),
  getScriptFileImport: vi.fn(),
  createScriptFileImport: vi.fn(),
  cancelScriptFileImport: vi.fn(),
  retryScriptFileImport: vi.fn(),
  reconcileScriptFileImport: vi.fn(),
}));
const id = "11111111-1111-4111-8111-111111111111";
const scope = {
  projectId: id,
  actorId: id,
  orgId: id,
  origin: window.location.origin,
};
it("生产仅调用生成六操作，scope与offset严格读边界；非法写入回执仍为unknown", async () => {
  vi.mocked(api.listScriptFileImports).mockResolvedValue({
    current_actor_id: id,
    current_org_id: id,
    items: [],
  });
  await listFileImports(scope, 25);
  expect(api.listScriptFileImports).toHaveBeenCalledWith(
    { pid: id, after: 25, limit: 25 },
    expect.any(Object),
  );
  const intent = {
    ...scope,
    key: id,
    version: 1 as const,
    action: "create" as const,
    body: {
      expected_revision: 0,
      asset_ids: [id],
      rights_confirmed: true as const,
    },
  };
  vi.mocked(api.createScriptFileImport).mockResolvedValue({ accepted: true });
  await expect(runFileImportIntent(intent)).rejects.toMatchObject({
    status: 502,
    code: "invalid_response",
  });
  expect(api.createScriptFileImport).toHaveBeenCalledWith(
    { pid: id },
    intent.body,
    expect.objectContaining({
      headers: expect.objectContaining({
        "Idempotency-Key": id,
        Origin: scope.origin,
      }),
    }),
  );
});
