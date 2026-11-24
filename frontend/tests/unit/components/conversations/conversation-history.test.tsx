import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ConversationHistory } from "@/components/conversations/conversation-history";
import { listAgentSessions } from "@/api/agent";

const context = { path: "/users", revision: 0, sessionId: "session" };
vi.mock("next/navigation", () => ({
  usePathname: () => context.path,
  useSearchParams: () => new URLSearchParams(),
  useRouter: () => ({ replace: vi.fn() }),
}));
vi.mock("@/api/agent", () => ({ listAgentSessions: vi.fn() }));
vi.mock("@/components/auth/session", () => ({
  // Route revalidation produces a fresh receipt for the same identity.
  useOptionalSession: () => ({
    actor: { id: "actor", revision: "1" },
    session_id: context.sessionId,
  }),
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({ selected: { id: "space", writable: true } }),
}));
vi.mock("@/components/conversations/conversation-context", () => ({
  useConversationContext: () => ({ listRevision: context.revision }),
}));
vi.mock("@/components/conversations/use-conversation-scope", () => ({
  useConversationScope: () => ({ data: { id: "project" } }),
}));

afterEach(cleanup);

it("换页保留已确认的空列表，业务刷新保留布局，切换登录会话重新初读", async () => {
  const empty = { items: [] };
  const list = vi.mocked(listAgentSessions);
  list.mockResolvedValueOnce(empty);
  const history = () => <ConversationHistory onNavigate={() => {}} />;
  const view = render(history());
  const placeholder = await screen.findByText("暂无历史对话");
  for (const path of ["/platform/teams", "/conversations", "/models"]) {
    context.path = path;
    view.rerender(history());
    expect(screen.getByText("暂无历史对话")).toBe(placeholder);
  }
  expect(list).toHaveBeenCalledTimes(1);
  let complete!: (value: typeof empty) => void;
  list.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        complete = resolve;
      }),
  );
  context.revision++;
  view.rerender(history());
  expect(list).toHaveBeenCalledTimes(2);
  expect(screen.getByText("暂无历史对话")).toBe(placeholder);
  expect(view.container.querySelector('[data-slot="skeleton"]')).toBeNull();
  await act(async () => complete(empty));

  list.mockResolvedValueOnce(empty);
  context.sessionId = "new-session";
  view.rerender(history());
  expect(screen.queryByText("暂无历史对话")).toBeNull();
  expect(view.container.querySelector('[data-slot="skeleton"]')).toBeTruthy();
  await screen.findByText("暂无历史对话");
  expect(list).toHaveBeenCalledTimes(3);
});
