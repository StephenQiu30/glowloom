import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { GovernanceLists } from "./governance-lists";
import {
  Confirmation,
  type ConfirmAction,
} from "@/components/governance/confirmation";
import { request } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
function View({ teamID }: { teamID?: string }) {
  const [action, setAction] = useState<ConfirmAction | null>(null);
  return (
    <>
      <GovernanceLists teamID={teamID} reload={0} onAction={setAction} />
      <Confirmation
        key={action?.title ?? "closed"}
        action={action}
        onClose={() => setAction(null)}
        onDone={() => {}}
      />
    </>
  );
}
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it.each([undefined, "team"])(
  "%s 邀请撤销收集5～500字原因并经生成客户端提交",
  async (teamID) => {
    vi.mocked(request).mockImplementation(async (url) =>
      url.endsWith("/invitations") || url.endsWith("/account-invitations")
        ? {
            items: [
              {
                id: "invite",
                email: "member@example.com",
                status: "pending",
                revision: "4",
              },
            ],
          }
        : { items: [] },
    );
    render(<View teamID={teamID} />);
    fireEvent.click(await screen.findByText("撤销"));
    const reason = screen.getByLabelText("操作原因");
    expect(reason.getAttribute("minlength")).toBe("5");
    expect(reason.getAttribute("maxlength")).toBe("500");
    fireEvent.change(reason, { target: { value: "撤销错误邀请" } });
    fireEvent.change(screen.getByLabelText("当前密码"), {
      target: { value: "password" },
    });
    fireEvent.click(screen.getByText("确认操作"));
    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        teamID
          ? "/api/teams/team/invitations/invite/revoke"
          : "/api/platform/account-invitations/invite/revoke",
        expect.objectContaining({
          data: { expected_revision: "4", reason: "撤销错误邀请" },
        }),
      ),
    );
  },
);
it("重发邀请保留只有版本的合同，不要求原因", async () => {
  vi.mocked(request).mockResolvedValue({
    items: [
      {
        id: "invite",
        email: "member@example.com",
        status: "pending",
        revision: "4",
      },
    ],
  });
  render(<View />);
  fireEvent.click(await screen.findByText("重发"));
  expect(screen.queryByLabelText("操作原因")).toBeNull();
  fireEvent.change(screen.getByLabelText("当前密码"), {
    target: { value: "password" },
  });
  fireEvent.click(screen.getByText("确认操作"));
  await waitFor(() =>
    expect(request).toHaveBeenCalledWith(
      "/api/platform/account-invitations/invite/resend",
      expect.objectContaining({ data: { expected_revision: "4" } }),
    ),
  );
});
