import { render } from "../../../../../feedback";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { PlatformTeamInvitations } from "@/app/platform/teams/_components/platform-team-invitations";
import { request } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it("重新打开详情能读取历史邀请，并经平台路径按当前版本撤销", async () => {
  vi.mocked(request).mockResolvedValue({
    items: [
      {
        id: "invite",
        email: "history@example.com",
        team_role: "member",
        status: "pending",
        revision: "4",
      },
    ],
  });
  const choose = vi.fn();
  render(
    <PlatformTeamInvitations teamID="team" reload={0} onAction={choose} />,
  );
  await screen.findByText(/history@example.com/);
  expect(request).toHaveBeenCalledWith(
    "/api/platform/teams/team/invitations",
    expect.objectContaining({ method: "GET" }),
  );
  fireEvent.click(screen.getByText("撤销邀请"));
  await choose.mock.calls[0][0].run(
    "撤销历史邀请",
    new AbortController().signal,
    "key",
  );
  expect(request).toHaveBeenLastCalledWith(
    "/api/platform/teams/team/invitations/invite/revoke",
    expect.objectContaining({
      data: { expected_revision: "4", reason: "撤销历史邀请" },
    }),
  );
});
