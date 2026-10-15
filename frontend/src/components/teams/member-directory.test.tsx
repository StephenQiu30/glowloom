import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { MemberDirectory } from "./member-directory";
import { request } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => ({ actor: { id: "self" } }),
}));
const team = {
  id: "team",
  name: "Team",
  status: "active",
  role: "team_admin",
  revision: "7",
  membership_revision: "3",
  member_count: 1,
  governance_blocked: false,
  invitation_enabled: true,
  capabilities: ["members:manage"],
} satisfies API.TeamView;
const member = {
  user_id: "other",
  display_name: "Other",
  avatar_asset_id: "",
  role: "member",
  status: "active",
  revision: "5",
  access_available: true,
};
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it("角色变更使用成员版本，管理员转交使用团队版本并保留同一幂等键", async () => {
  vi.mocked(request).mockResolvedValue({ items: [member], next_cursor: "" });
  const choose = vi.fn();
  render(<MemberDirectory team={team} reload={0} onAction={choose} />);
  fireEvent.click(await screen.findByText("设为管理员"));
  const role = choose.mock.calls.at(-1)![0];
  await role.run("调整角色", new AbortController().signal, "request-key");
  expect(request).toHaveBeenLastCalledWith(
    "/api/teams/team/members/other/role",
    expect.objectContaining({
      data: { expected_revision: "5", reason: "调整角色", role: "team_admin" },
    }),
  );
  fireEvent.click(screen.getByText("转交管理"));
  await choose.mock.calls
    .at(-1)![0]
    .run("管理交接", new AbortController().signal, "request-key");
  expect(request).toHaveBeenLastCalledWith(
    "/api/teams/team/admin-transfers",
    expect.objectContaining({
      data: {
        expected_revision: "7",
        reason: "管理交接",
        target_user_id: "other",
      },
      headers: { "Idempotency-Key": "request-key" },
    }),
  );
});
it("普通成员目录不显示管理动作，即使列表含有可见版本字段", async () => {
  vi.mocked(request).mockResolvedValue({ items: [member], next_cursor: "" });
  render(
    <MemberDirectory
      team={{ ...team, role: "member", capabilities: ["members:read"] }}
      reload={0}
      onAction={vi.fn()}
    />,
  );
  await screen.findByText(/Other/);
  expect(screen.queryByText("停用")).toBeNull();
  expect(screen.queryByText("移出团队")).toBeNull();
});

it("降级团队管理员发送 member，独立于全局 creator 身份", async () => {
  vi.mocked(request).mockResolvedValue({
    items: [{ ...member, role: "team_admin" }],
    next_cursor: "",
  });
  const choose = vi.fn();
  render(<MemberDirectory team={team} reload={0} onAction={choose} />);
  fireEvent.click(await screen.findByText("改为普通成员"));
  await choose.mock.calls
    .at(-1)![0]
    .run("完成管理交接", new AbortController().signal, "request-key");
  expect(request).toHaveBeenLastCalledWith(
    "/api/teams/team/members/other/role",
    expect.objectContaining({
      data: { expected_revision: "5", reason: "完成管理交接", role: "member" },
    }),
  );
});
