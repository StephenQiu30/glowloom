import { afterEach, expect, it, vi } from "vitest";
import { userBatchAction } from "@/components/governance/batch-actions";
import { teamBatchAction } from "@/app/platform/teams/_components/team-batch-actions";
import { request, RequestError } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
afterEach(() => vi.resetAllMocks());
const row = {
  id: "target",
  display_name: "Target",
  email: "target@example.invalid",
  revision: "3",
  status: "active",
} as API.PlatformUserView;
const signal = new AbortController().signal;
it("删除预检跳过自己、已删除及待交接账号，拒绝陈旧版本", async () => {
  const action = userBatchAction("delete", "self");
  expect(
    (await action.prepare({ ...row, id: "self" }, signal, false)).skip,
  ).toContain("当前");
  expect(request).not.toHaveBeenCalled();
  vi.mocked(request).mockResolvedValue({
    user: row,
    allowed_actions: [],
    blocked_actions: { delete: "deletion_duties" },
  });
  expect((await action.prepare(row, signal, false)).skip).toContain("交接");
  vi.mocked(request).mockResolvedValue({
    user: { ...row, status: "deleted" },
    allowed_actions: [],
    blocked_actions: { delete: "account_deleted" },
  });
  expect((await action.prepare(row, signal, false)).skip).toBe("账号已删除");
  vi.mocked(request).mockResolvedValue({
    user: { ...row, revision: "4" },
    allowed_actions: ["delete"],
  });
  await expect(action.prepare(row, signal, false)).rejects.toMatchObject({
    code: "revision_conflict",
  });
  expect((await action.prepare(row, signal, true)).row.revision).toBe("4");
});
it("批量删除只发送版本和原请求标识，回执必须绑定动作及目标", async () => {
  const action = userBatchAction("delete", "self");
  const entry = {
    id: row.id,
    row,
    label: "Target",
    requestId: "original-key",
    status: "ready" as const,
    message: "",
  };
  expect(action).toMatchObject({
    reason: false,
    password: false,
    compact: true,
    destructive: true,
  });
  vi.mocked(request).mockResolvedValue(undefined);
  await action.execute(entry, "", signal);
  expect(request).toHaveBeenCalledWith(
    "/api/platform/users/target/delete",
    expect.objectContaining({
      method: "POST",
      data: { expected_revision: "3" },
      headers: { "Idempotency-Key": "original-key" },
    }),
  );
  vi.mocked(request).mockResolvedValue({
    request_id: "original-key",
    target_id: "other",
    action: "delete",
  });
  await expect(action.recover!(entry, signal)).rejects.toBeInstanceOf(
    RequestError,
  );
  vi.mocked(request).mockResolvedValue({
    request_id: "original-key",
    target_id: "target",
    action: "delete",
  });
  expect(await action.recover!(entry, signal)).toMatchObject({
    message: expect.stringContaining("成功回执"),
  });
});
it("团队批量操作仅调用团队合同，不复用账号删除或重放策略", async () => {
  const action = teamBatchAction("suspend"),
    team = { id: "team", revision: "7", name: "产品", status: "active" };
  vi.mocked(request).mockResolvedValue(team);
  await action.prepare(team, signal, false);
  await action.execute(
    {
      id: "team",
      row: team,
      label: "产品",
      requestId: "unused",
      status: "ready",
      message: "",
    },
    "团队停用原因",
    signal,
  );
  expect(vi.mocked(request).mock.calls.map(([url]) => url)).toEqual([
    "/api/platform/teams/team",
    "/api/platform/teams/team/suspend",
  ]);
  expect(action.recover).toBeUndefined();
});
