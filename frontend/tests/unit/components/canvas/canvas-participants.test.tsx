import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  CanvasParticipants,
  participantUserIDs,
} from "@/components/canvas/canvas-participants";
import type {
  CanvasYjsLiveSession,
  YjsPeer,
} from "@/components/canvas/yjs-live-session";

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  avatar: vi.fn(),
  ownAvatar: vi.fn(),
  session: {
    actor: { id: "a", display_name: "真实甲", avatar_asset_id: "" },
    session_id: "session-a",
  },
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => mocks.session,
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({ spaces: [{ id: "scope", team_id: "team" }] }),
}));
vi.mock("@/api/identity", () => ({
  listTeamMembers: mocks.list,
  teamMemberAvatar: mocks.avatar,
}));
vi.mock("@/api/media", () => ({ getMyAvatar: mocks.ownAvatar }));
let peers: YjsPeer[];
const subscribers = new Set<() => void>();
const client = {
  subscribePresence: (fn: () => void) => {
    subscribers.add(fn);
    return () => subscribers.delete(fn);
  },
  getPresence: () => peers,
} as unknown as CanvasYjsLiveSession;
function peer(actorId: string, clientId: string): YjsPeer {
  return { actorId, clientId, cursor: null, updatedAt: "" };
}
function view(connected = true) {
  return (
    <CanvasParticipants
      client={client}
      actorID="a"
      scopeID="scope"
      connected={connected}
    />
  );
}
beforeEach(() => {
  vi.clearAllMocks();
  peers = [peer("a", "another-a"), peer("b", "b-1"), peer("b", "b-2")];
  mocks.list.mockResolvedValue({
    items: [
      { user_id: "a", display_name: "真实甲", avatar_asset_id: "" },
      { user_id: "b", display_name: "真实乙", avatar_asset_id: "avatar-b" },
    ],
  });
  mocks.avatar.mockResolvedValue({
    url: "/api/real-avatar",
    expires_at: new Date(Date.now() + 60000).toISOString(),
  });
});
afterEach(cleanup);
it("按用户合并多个连接并保持自己在前", () => {
  expect(participantUserIDs("a", peers)).toEqual(["a", "b"]);
  expect(
    participantUserIDs("a", [peer("c", "1"), peer("b", "2"), peer("c", "3")]),
  ).toEqual(["a", "b", "c"]);
});
it("普通成员使用授权目录读取真实姓名头像，光标更新不重复取资料", async () => {
  render(view());
  const trigger = await screen.findByRole("button", {
    name: "查看参与者 真实乙",
  });
  expect(screen.getByLabelText("2 位参与者在线")).toBeTruthy();
  expect(trigger.getAttribute("data-slot")).toBe("avatar");
  await waitFor(() =>
    expect(mocks.avatar).toHaveBeenCalledWith({ id: "team", uid: "b" }),
  );
  fireEvent.click(trigger);
  const dialog = await screen.findByRole("dialog", { name: "参与者信息" });
  expect(within(dialog).getByText("真实乙")).toBeTruthy();
  expect(within(dialog).getByText("在线")).toBeTruthy();
  act(() => {
    peers = peers.map((p) => ({ ...p, cursor: { x: 2, y: 4 } }));
    subscribers.forEach((fn) => fn());
  });
  expect(mocks.list).toHaveBeenCalledTimes(1);
  expect(mocks.avatar).toHaveBeenCalledTimes(1);
  fireEvent.keyDown(dialog, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(trigger));
});
it("显示三人及溢出数量，点击可查看全部，离开后立即更新", async () => {
  peers = [peer("b", "1"), peer("c", "2"), peer("d", "3"), peer("e", "4")];
  mocks.list.mockResolvedValue({
    items: ["a", "b", "c", "d", "e"].map((id) => ({
      user_id: id,
      display_name: `用户${id}`,
      avatar_asset_id: "",
    })),
  });
  render(view());
  await screen.findByRole("button", { name: "查看参与者 用户b" });
  expect(screen.getByText("+2")).toBeTruthy();
  fireEvent.click(
    screen.getAllByRole("button", { name: "查看全部 5 位参与者" })[0],
  );
  const list = await screen.findByRole("dialog", { name: "在线参与者" });
  expect(within(list).getAllByRole("listitem")).toHaveLength(5);
  act(() => {
    peers = [peer("b", "1")];
    subscribers.forEach((fn) => fn());
  });
  expect(screen.getByLabelText("2 位参与者在线")).toBeTruthy();
  expect(screen.queryByText("+2")).toBeNull();
});
it("断线不把旧参与者当作在线", async () => {
  const mounted = render(view());
  await screen.findByRole("button", { name: "查看参与者 真实乙" });
  mounted.rerender(view(false));
  expect(screen.queryByLabelText("2 位参与者在线")).toBeNull();
  expect(screen.getByRole("status").textContent).toContain("在线状态未知");
});
it("目录分页查找在线用户，并且不展示邮箱或原始ID", async () => {
  mocks.list
    .mockResolvedValueOnce({ items: [], next_cursor: "next" })
    .mockResolvedValueOnce({
      items: [
        {
          user_id: "b",
          display_name: "乙分页",
          email: "private@example.test",
          avatar_asset_id: "",
        },
      ],
    });
  render(view());
  fireEvent.click(
    await screen.findByRole("button", { name: "查看参与者 乙分页" }),
  );
  expect(mocks.list).toHaveBeenLastCalledWith({
    id: "team",
    limit: 100,
    status: "active",
    cursor: "next",
  });
  expect(screen.queryByText("private@example.test")).toBeNull();
});
it("读取失败显示默认头像和正文重试，重试后恢复真实信息", async () => {
  mocks.list.mockRejectedValueOnce(new Error("denied"));
  render(view());
  fireEvent.click(
    await screen.findByRole("button", { name: "查看参与者 协作者" }),
  );
  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("暂时无法读取参与者信息");
  fireEvent.click(within(alert).getByRole("button", { name: "重新读取" }));
  await screen.findByRole("button", { name: "查看参与者 真实乙" });
  expect(screen.queryByRole("alert")).toBeNull();
});
