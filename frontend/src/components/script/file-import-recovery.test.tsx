import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { FileImportConflict } from "./file-import-recovery";
import { getWorkspace } from "./source-queries";
import { listFileImports } from "./file-import-queries";
import type { useFileImport } from "./use-file-import";
vi.mock("./source-queries", async (original) => ({
  ...(await original<typeof import("./source-queries")>()),
  getWorkspace: vi.fn(),
}));
vi.mock("./file-import-queries", () => ({
  listFileImports: vi.fn(),
  getFileImport: vi.fn(),
}));
const id = (n: number) =>
  `${String(n).repeat(8)}-${String(n).repeat(4)}-4${String(n).repeat(3)}-8${String(n).repeat(3)}-${String(n).repeat(12)}`;
const scope = {
  origin: window.location.origin,
  projectId: id(1),
  actorId: id(2),
  orgId: id(3),
};
const original = {
  ...scope,
  key: id(4),
  version: 1 as const,
  action: "create" as const,
  body: {
    expected_revision: 1,
    base_version_id: id(5),
    rights_confirmed: true as const,
    asset_ids: [id(6), id(7)],
  },
};
function writer(): ReturnType<typeof useFileImport> {
  return {
    ready: true,
    busy: false,
    locked: true,
    intent: null,
    rejected: { intent: original, cause: new ApiError(409, "stale_revision") },
    error: "版本已变化",
    storageError: undefined,
    submit: vi.fn(async () => {}),
    replay: vi.fn(async () => {}),
    restoreStorage: vi.fn(async () => {}),
    acknowledgeLatest: vi.fn(),
  };
}
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(listFileImports).mockResolvedValue({
    current_actor_id: scope.actorId,
    current_org_id: scope.orgId,
    items: [],
  });
  vi.mocked(getWorkspace).mockResolvedValue({
    current_actor_id: scope.actorId,
    current_org_id: scope.orgId,
    state: {
      project_id: scope.projectId,
      org_id: scope.orgId,
      revision: 9,
      draft_version_id: id(8),
      updated_at: "2026-10-02T00:00:00Z",
    },
  });
});
afterEach(cleanup);
it("409先读取并审阅当前头，再人工新CAS；原件顺序与版权正文完整保留", async () => {
  const current = writer();
  render(<FileImportConflict scope={scope} writer={current} />);
  expect(
    screen.queryByRole("button", { name: "按此原件顺序另提交新导入意图" }),
  ).toBeNull();
  fireEvent.click(
    screen.getByRole("button", { name: "读取最新导入事实并保留草稿" }),
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "按此原件顺序另提交新导入意图" }),
  );
  expect(current.acknowledgeLatest).toHaveBeenCalledTimes(1);
  expect(current.submit).toHaveBeenCalledWith({
    action: "create",
    body: { ...original.body, expected_revision: 9, base_version_id: id(8) },
  });
});
it("scope改变不能把旧草稿移交新actor，最新读取失败不解锁或发新键", async () => {
  vi.mocked(listFileImports).mockResolvedValue({
    current_actor_id: id(9),
    current_org_id: scope.orgId,
    items: [],
  });
  const current = writer();
  render(<FileImportConflict scope={scope} writer={current} />);
  fireEvent.click(
    screen.getByRole("button", { name: "读取最新导入事实并保留草稿" }),
  );
  expect(
    await screen.findByText("当前主体或组织已变化，请保留原意图。"),
  ).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: "按此原件顺序另提交新导入意图" }),
  ).toBeNull();
  expect(current.submit).not.toHaveBeenCalled();
  expect(current.acknowledgeLatest).not.toHaveBeenCalled();
});
