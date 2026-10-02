import { beforeEach, expect, it, vi } from "vitest";
import * as media from "@/gen/api/media";
import {
  runLibraryPurge,
  getLibraryPurge,
  listLibraryPurges,
} from "./library-purge-query";
import {
  purgeTestIdentity as identity,
  purgeTestInput as body,
  purgeTestJob as job,
} from "./library-purge-fixtures";
import {
  transferLibrary,
  transferIDs as ids,
} from "@/components/library/transfer-test-fixtures";
import * as library from "./library-queries";
vi.mock("@/gen/api/media", () => ({
  createMediaPurge: vi.fn(),
  getMediaPurge: vi.fn(),
  listMediaPurges: vi.fn(),
  cancelMediaPurge: vi.fn(),
  reconcileMediaPurge: vi.fn(),
}));
vi.mock("./library-queries", () => ({ freshLibrary: vi.fn() }));
const original = {
  ...identity,
  version: 1 as const,
  key: ids.target,
  action: "create" as const,
  body,
};
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(library.freshLibrary).mockResolvedValue(transferLibrary);
});
it("永久命令只调用正式生成SDK，精确scope/body/Origin/同key；取消对账保留实际job与状态revision", async () => {
  vi.mocked(media.createMediaPurge).mockResolvedValue(job);
  const controller = new AbortController();
  expect(await runLibraryPurge(original, controller.signal)).toEqual(job);
  expect(media.createMediaPurge).toHaveBeenCalledWith(body, {
    headers: { "Idempotency-Key": ids.target, Origin: identity.origin },
    signal: controller.signal,
  });
  for (const action of ["cancel", "reconcile"] as const) {
    vi.mocked(
      action === "cancel" ? media.cancelMediaPurge : media.reconcileMediaPurge,
    ).mockResolvedValue(job);
    await runLibraryPurge({
      ...identity,
      version: 1,
      key: ids.target,
      action,
      jobId: job.id,
      body: { revision: job.revision },
    });
    expect(
      action === "cancel" ? media.cancelMediaPurge : media.reconcileMediaPurge,
    ).toHaveBeenCalledWith(
      { job_id: job.id },
      { revision: job.revision },
      expect.objectContaining({
        headers: { "Idempotency-Key": ids.target, Origin: identity.origin },
      }),
    );
  }
});
it("不认可跨主体、job、scope、缺项、额外私有路径或错误任务顺序，保留unknown而不冒称成功", async () => {
  for (const value of [
    { ...job, current_actor_id: ids.target },
    { ...job, scope: { kind: "project", project_id: ids.project } },
    { ...job, items: [] },
    { ...job, object_key: "private" },
    { ...job, items: [{ ...job.items[0], item_id: ids.target }] },
    { ...job, items: [{ ...job.items[0], index: 1 }] },
  ]) {
    vi.mocked(media.createMediaPurge).mockResolvedValue(value);
    await expect(runLibraryPurge(original)).rejects.toThrow();
  }
  vi.mocked(media.cancelMediaPurge).mockResolvedValue({
    ...job,
    id: ids.target,
  });
  await expect(
    runLibraryPurge({
      ...identity,
      version: 1,
      key: ids.target,
      action: "cancel",
      jobId: job.id,
      body: { revision: 1 },
    }),
  ).rejects.toThrow();
});
it("任务分页与detail每次核当前授权，范围/分页不符与旧主体响应失败关闭", async () => {
  vi.mocked(media.getMediaPurge).mockResolvedValue(job);
  expect(await getLibraryPurge(identity, job.id)).toEqual(job);
  vi.mocked(media.listMediaPurges).mockResolvedValue({
    current_actor_id: ids.actor,
    current_org_id: ids.org,
    page: 2,
    page_size: 20,
    items: [job],
  });
  expect(await listLibraryPurges(identity, 2)).toEqual([job]);
  vi.mocked(media.listMediaPurges).mockResolvedValue({
    current_actor_id: ids.actor,
    current_org_id: ids.org,
    page: 1,
    page_size: 20,
    items: [job],
  });
  await expect(listLibraryPurges(identity, 2)).rejects.toThrow();
  vi.mocked(library.freshLibrary).mockRejectedValue(new Error("scope changed"));
  const calls = vi.mocked(media.getMediaPurge).mock.calls.length;
  await expect(getLibraryPurge(identity, job.id)).rejects.toThrow();
  expect(media.getMediaPurge).toHaveBeenCalledTimes(calls);
});
