import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { StrictMode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { InvitationsPage } from "./invitations-page";
import { request } from "@/lib/request";
import { clearInvitation } from "@/components/auth/invitation-continuity";
const state = vi.hoisted(() => ({
  session: null as { actor: { id: string } } | null,
}));
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => state.session,
}));
afterEach(() => {
  cleanup();
  clearInvitation();
  state.session = null;
  window.history.replaceState(null, "", "/");
  vi.resetAllMocks();
});
it("公共邀请在StrictMode清除fragment后保留注册登录连续性，并且不自动授予成员关系", async () => {
  window.history.replaceState(null, "", `/invitations#token=${"a".repeat(43)}`);
  vi.mocked(request).mockResolvedValue({
    kind: "team",
    team_id: "team-id",
    team_role: "member",
    expires_at: "2026-10-17T00:00:00Z",
  });
  render(
    <StrictMode>
      <InvitationsPage />
    </StrictMode>,
  );
  await screen.findByText("收到邀请");
  expect(window.location.hash).toBe("");
  expect(
    screen.getByRole("link", { name: "验证邮箱并注册" }).getAttribute("href"),
  ).toBe("/register");
  expect(screen.queryByText("接受邀请")).toBeNull();
  expect(
    vi
      .mocked(request)
      .mock.calls.every(([url]) => url === "/api/invitations/preview"),
  ).toBe(true);
});
it("已登录用户也必须显式确认邀请，接受后清除令牌", async () => {
  state.session = { actor: { id: "recipient" } };
  window.history.replaceState(null, "", `/invitations#token=${"a".repeat(43)}`);
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/invitations/preview"
      ? { kind: "team", team_id: "team-id", team_role: "member" }
      : { items: [] },
  );
  render(<InvitationsPage />);
  fireEvent.click(await screen.findByText("接受邀请"));
  expect(
    vi
      .mocked(request)
      .mock.calls.some(([url]) => url === "/api/invitations/accept"),
  ).toBe(false);
  fireEvent.click(screen.getByText("确认操作"));
  await waitFor(() =>
    expect(request).toHaveBeenCalledWith(
      "/api/invitations/accept",
      expect.objectContaining({ data: { token: "a".repeat(43) } }),
    ),
  );
  await screen.findByText("邀请已接受。");
});
