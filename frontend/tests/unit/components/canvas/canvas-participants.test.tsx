import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { Activity, StrictMode, Suspense } from "react";
import { mutate } from "swr";
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
  spaces: [{ id: "scope", team_id: "team" }],
  session: {
    actor: { id: "a", display_name: "真实甲", avatar_asset_id: "" },
    session_id: "session-a",
  },
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => mocks.session,
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({ spaces: mocks.spaces }),
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
  mocks.session = {
    actor: { id: "a", display_name: "真实甲", avatar_asset_id: "" },
    session_id: crypto.randomUUID(),
  };
  mocks.spaces = [{ id: "scope", team_id: "team" }];
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

it("strict lifecycle and reconnect preserve the participant cache and interactive details", async () => {
  const mounted = render(<StrictMode>{view()}</StrictMode>);
  await screen.findByRole("button", { name: "查看参与者 真实乙" });
  mounted.rerender(<StrictMode>{view(false)}</StrictMode>);
  mounted.rerender(<StrictMode>{view()}</StrictMode>);
  fireEvent.click(
    await screen.findByRole("button", { name: "查看参与者 真实乙" }),
  );
  expect(
    await screen.findByRole("dialog", { name: "参与者信息" }),
  ).toBeTruthy();
});

it("Activity hide/reveal retains a usable SWR lifecycle", async () => {
  const mounted = render(<Activity mode="visible">{view()}</Activity>);
  await screen.findByRole("button", { name: "查看参与者 真实乙" });
  mounted.rerender(<Activity mode="hidden">{view()}</Activity>);
  mounted.rerender(<Activity mode="visible">{view()}</Activity>);
  fireEvent.click(
    await screen.findByRole("button", { name: "查看参与者 真实乙" }),
  );
  expect(
    await screen.findByRole("dialog", { name: "参与者信息" }),
  ).toBeTruthy();
});

it("Suspense hide/reveal restores interactive participants without a disposed provider", async () => {
  let suspended = false;
  const pending = new Promise<never>(() => {});
  function Content() {
    if (suspended) throw pending;
    return view();
  }
  const mounted = render(
    <Suspense fallback={<p>loading</p>}>
      <Content />
    </Suspense>,
  );
  await screen.findByRole("button", { name: "查看参与者 真实乙" });
  suspended = true;
  mounted.rerender(
    <Suspense fallback={<p>loading</p>}>
      <Content />
    </Suspense>,
  );
  expect(screen.getByText("loading")).toBeTruthy();
  suspended = false;
  mounted.rerender(
    <Suspense fallback={<p>loading</p>}>
      <Content />
    </Suspense>,
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "查看参与者 真实乙" }),
  );
  expect(
    await screen.findByRole("dialog", { name: "参与者信息" }),
  ).toBeTruthy();
});

it("a new session cannot display cached profiles or a late response from the old session", async () => {
  let finishOld!: (value: unknown) => void;
  mocks.list.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finishOld = resolve;
      }),
  );
  const mounted = render(view());
  await waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(1));
  mocks.session = { ...mocks.session, session_id: "replacement-session" };
  mocks.list.mockResolvedValue({
    items: [{ user_id: "b", display_name: "新会话乙", avatar_asset_id: "" }],
  });
  mounted.rerender(view());
  await screen.findByRole("button", { name: "查看参与者 新会话乙" });
  await act(async () =>
    finishOld({
      items: [
        {
          user_id: "b",
          display_name: "旧私有姓名",
          avatar_asset_id: "old-avatar",
        },
      ],
    }),
  );
  expect(
    screen.queryByRole("button", { name: "查看参与者 旧私有姓名" }),
  ).toBeNull();
  expect(
    screen.getByRole("button", { name: "查看参与者 新会话乙" }),
  ).toBeTruthy();
  expect(mocks.avatar).not.toHaveBeenCalled();
});

it("scope and actor changes do not expose cached names or avatar requests", async () => {
  const mounted = render(view());
  await screen.findByRole("button", { name: "查看参与者 真实乙" });
  let finishNew!: (value: unknown) => void;
  mocks.list.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finishNew = resolve;
      }),
  );
  mocks.spaces = [{ id: "new-scope", team_id: "new-team" }];
  mocks.session = {
    ...mocks.session,
    actor: { id: "z", display_name: "新用户", avatar_asset_id: "" },
  };
  mounted.rerender(
    <CanvasParticipants
      client={client}
      actorID="z"
      scopeID="new-scope"
      connected
    />,
  );
  expect(
    screen.queryByRole("button", { name: "查看参与者 真实乙" }),
  ).toBeNull();
  expect(
    screen.queryByRole("button", { name: "查看参与者 真实甲（你）" }),
  ).toBeNull();
  await act(async () =>
    finishNew({
      items: [{ user_id: "b", display_name: "新空间乙", avatar_asset_id: "" }],
    }),
  );
  await screen.findByRole("button", { name: "查看参与者 新空间乙" });
  expect(mocks.list).toHaveBeenLastCalledWith({
    id: "new-team",
    limit: 100,
    status: "active",
    cursor: undefined,
  });
});

it("permission failure during revalidation removes readable cached collaborator details", async () => {
  render(view());
  const trigger = await screen.findByRole("button", {
    name: "查看参与者 真实乙",
  });
  fireEvent.click(trigger);
  expect(
    within(await screen.findByRole("dialog", { name: "参与者信息" })).getByText(
      "真实乙",
    ),
  ).toBeTruthy();
  mocks.list.mockRejectedValue(new Error("forbidden"));
  await act(async () => {
    await mutate(
      (key) =>
        Array.isArray(key) &&
        key[0] === "canvas-participants" &&
        key.includes(mocks.session.session_id),
    );
  });
  expect(
    screen.queryByRole("button", { name: "查看参与者 真实乙" }),
  ).toBeNull();
  expect(screen.queryByText("真实乙")).toBeNull();
  expect(screen.getByRole("alert").textContent).toContain(
    "暂时无法读取参与者信息",
  );
});

it("the same avatar identity is fetched anew in a different session", async () => {
  const mounted = render(view());
  await screen.findByRole("button", { name: "查看参与者 真实乙" });
  await waitFor(() => expect(mocks.avatar).toHaveBeenCalledTimes(1));
  mocks.session = { ...mocks.session, session_id: "another-session" };
  mounted.rerender(view());
  await screen.findByRole("button", { name: "查看参与者 真实乙" });
  await waitFor(() => expect(mocks.avatar).toHaveBeenCalledTimes(2));
  expect(mocks.list).toHaveBeenCalledTimes(2);
});

it("a scope-only switch cannot reuse the same actor and session's old private directory", async () => {
  const mounted = render(view());
  await screen.findByRole("button", { name: "查看参与者 真实乙" });
  let finish!: (value: unknown) => void;
  mocks.list.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  mocks.spaces = [{ id: "second-scope", team_id: "second-team" }];
  mounted.rerender(
    <CanvasParticipants
      client={client}
      actorID="a"
      scopeID="second-scope"
      connected
    />,
  );
  expect(
    screen.queryByRole("button", { name: "查看参与者 真实乙" }),
  ).toBeNull();
  await act(async () =>
    finish({
      items: [
        { user_id: "b", display_name: "其他空间成员", avatar_asset_id: "" },
      ],
    }),
  );
  await screen.findByRole("button", { name: "查看参与者 其他空间成员" });
  expect(mocks.list).toHaveBeenCalledTimes(2);
});

it("a request resolving while Activity is hidden can be revealed and opened safely", async () => {
  let finish!: (value: unknown) => void;
  mocks.list.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const mounted = render(<Activity mode="visible">{view()}</Activity>);
  await waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(1));
  mounted.rerender(<Activity mode="hidden">{view()}</Activity>);
  await act(async () =>
    finish({
      items: [
        { user_id: "b", display_name: "隐藏期间就绪", avatar_asset_id: "" },
      ],
    }),
  );
  mounted.rerender(<Activity mode="visible">{view()}</Activity>);
  fireEvent.click(
    await screen.findByRole("button", { name: "查看参与者 隐藏期间就绪" }),
  );
  expect(
    within(await screen.findByRole("dialog", { name: "参与者信息" })).getByText(
      "隐藏期间就绪",
    ),
  ).toBeTruthy();
});
