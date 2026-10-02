import { beforeEach, expect, it, vi } from "vitest";
import {
  checkPurgePlan,
  createPurgePlan,
  loadPurgePlan,
  recordPurgePlanJob,
  savePurgePlan,
} from "./library-purge-plan";
import * as queries from "./library-purge-query";
import {
  transferIdentity as identity,
  transferIDs as ids,
} from "@/components/library/transfer-test-fixtures";
import type { PurgeJob, PurgeReview } from "./library-purge-model";
vi.mock("./library-purge-query", () => ({
  getLibraryPurge: vi.fn(),
  reviewLibraryPurge: vi.fn(),
}));
const items = Array.from({ length: 201 }, (_, i) => ({
  id: `10000000-0000-4000-8000-${String(i + 1).padStart(12, "0")}`,
  revision: 2,
  title: `条目${i}`,
}));
const review: PurgeReview = {
  mode: "all",
  revision: 4,
  projectRevision: 0,
  items,
};
function job(status: PurgeJob["status"] = "succeeded"): PurgeJob {
  return {
    id: ids.job,
    scope: identity.scope,
    current_actor_id: ids.actor,
    current_org_id: ids.org,
    status,
    stage: status === "succeeded" ? "completed" : "removing",
    revision: 4,
    attempt: 1,
    cancellation_requested: false,
    needs_reconciliation: status === "needs_reconciliation",
    execution_unconfirmed: false,
    created_at: "2026-10-02T01:00:00Z",
    updated_at: "2026-10-02T01:01:00Z",
    items: items.slice(0, 200).map((item, index) => ({
      index,
      item_id: item.id,
      asset_id: null,
      status: status === "succeeded" ? "succeeded" : "needs_reconciliation",
      failure_code: null,
    })),
  };
}
beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
});
it("全部201项冻结2稳定子键，首批实际终态及精确+201revision才允许余1项", async () => {
  let plan = createPurgePlan(identity, review);
  plan.batches[0].input = {
    scope: identity.scope,
    items: items.slice(0, 200).map(({ id, revision }) => ({ id, revision })),
    expected_revision: 4,
    expected_project_revision: 0,
    permanent_delete_confirmed: true,
  };
  plan = recordPurgePlanJob(plan, job());
  vi.mocked(queries.getLibraryPurge).mockResolvedValue(job());
  vi.mocked(queries.reviewLibraryPurge).mockResolvedValue({
    ...review,
    revision: 205,
    items: items.slice(200),
  });
  const checked = await checkPurgePlan(plan);
  expect(checked.index).toBe(1);
  expect(checked.plan.batches[1].key).toBe(plan.batches[1].key);
  expect(checked.plan.batches[1].input?.items).toEqual([
    { id: items[200].id, revision: 2 },
  ]);
  expect(checked.plan.batches[1].input?.expected_revision).toBe(205);
});
it("未知、未终态、新增trash、残留CAS/外部库revision变化都停止，不自动续批", async () => {
  const plan = createPurgePlan(identity, review);
  plan.batches[0].input = {
    scope: identity.scope,
    items: items.slice(0, 200).map(({ id, revision }) => ({ id, revision })),
    expected_revision: 4,
    expected_project_revision: 0,
    permanent_delete_confirmed: true,
  };
  plan.batches[0].job = job();
  for (const changed of [
    { ...job(), status: "running" as const },
    job("needs_reconciliation"),
  ]) {
    vi.mocked(queries.getLibraryPurge).mockResolvedValue(changed);
    await expect(checkPurgePlan(plan)).rejects.toThrow();
  }
  vi.mocked(queries.getLibraryPurge).mockResolvedValue(job());
  for (const changed of [
    { ...review, revision: 206, items: items.slice(200) },
    {
      ...review,
      revision: 205,
      items: [items[200], { ...items[200], id: ids.target }],
    },
    { ...review, revision: 205, items: [{ ...items[200], revision: 3 }] },
  ]) {
    vi.mocked(queries.reviewLibraryPurge).mockResolvedValue(changed);
    await expect(checkPurgePlan(plan)).rejects.toThrow();
  }
});
it("scope隔离、plan存储CAS、原targets与子键完整，刷新不发送任何写", () => {
  const plan = createPurgePlan(identity, review);
  savePurgePlan(sessionStorage, plan, null);
  expect(loadPurgePlan(sessionStorage, identity)).toEqual(plan);
  expect(
    loadPurgePlan(sessionStorage, { ...identity, orgId: ids.target }),
  ).toBeNull();
  expect(() =>
    savePurgePlan(sessionStorage, createPurgePlan(identity, review), null),
  ).toThrow();
  expect(plan.batches).toHaveLength(2);
});
it("blocked与cancelled留在回收站，精确自己的释放revision后继续，绝不自动重试已处理目标", async () => {
  let plan = createPurgePlan(identity, review);
  plan.batches[0].input = {
    scope: identity.scope,
    items: items.slice(0, 200).map(({ id, revision }) => ({ id, revision })),
    expected_revision: 4,
    expected_project_revision: 0,
    permanent_delete_confirmed: true,
  };
  const terminal = {
    ...job(),
    status: "partial_failed" as const,
    cancellation_requested: true,
    items: job().items.map((item, index) =>
      index === 0
        ? {
            ...item,
            status: "blocked" as const,
            failure_code: "in_use" as const,
          }
        : index === 1
          ? {
              ...item,
              status: "cancelled" as const,
              failure_code: "cancelled" as const,
            }
          : item,
    ),
  };
  plan = recordPurgePlanJob(plan, terminal);
  vi.mocked(queries.getLibraryPurge).mockResolvedValue(terminal);
  vi.mocked(queries.reviewLibraryPurge).mockResolvedValue({
    ...review,
    revision: 204,
    items: [items[0], items[1], items[200]],
  });
  const checked = await checkPurgePlan(plan);
  expect(checked.index).toBe(1);
  expect(checked.plan.batches[1].input?.items).toEqual([
    { id: items[200].id, revision: 2 },
  ]);
});
