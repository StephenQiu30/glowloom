import { afterEach, expect, it, vi } from "vitest";
import * as media from "@/api/media";
import { getLibraryStorageUsage } from "./library-storage-query";
import {
  transferIdentity as identity,
  transferIDs as ids,
} from "@/components/library/transfer-test-fixtures";
vi.mock("@/api/media", () => ({ getMediaLibraryStorageUsage: vi.fn() }));
const usage = {
  current_actor_id: ids.actor,
  current_org_id: ids.org,
  library_id: identity.libraryId,
  scope: identity.scope,
  used_bytes: 125000,
  object_count: 3,
  limit_bytes: null,
  calculated_at: "2026-10-02T01:00:00Z",
};
afterEach(() => vi.clearAllMocks());
it("真实容量只用完整服务器事实，null额度保持null，scope及abort交给正式SDK", async () => {
  vi.mocked(media.getMediaLibraryStorageUsage).mockResolvedValue(usage);
  const controller = new AbortController();
  expect(await getLibraryStorageUsage(identity, controller.signal)).toEqual(
    usage,
  );
  expect(media.getMediaLibraryStorageUsage).toHaveBeenCalledWith(
    { scope: "personal" },
    { signal: controller.signal },
  );
});
it("拒绝跨主体、跨库、跨scope、非安全整数和私有key；缺额字段不造quota", async () => {
  for (const body of [
    { ...usage, current_actor_id: ids.target },
    { ...usage, library_id: ids.target },
    { ...usage, scope: { kind: "project" as const, project_id: ids.project } },
    { ...usage, used_bytes: Number.MAX_SAFE_INTEGER + 1 },
    { ...usage, object_key: "private" },
    { ...usage, limit_bytes: undefined },
  ]) {
    vi.mocked(media.getMediaLibraryStorageUsage).mockResolvedValue(body);
    await expect(getLibraryStorageUsage(identity)).rejects.toThrow();
  }
});
