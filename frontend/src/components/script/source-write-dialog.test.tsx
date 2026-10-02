import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { SourceWriteDialog } from "./source-write-dialog";
import {
  getSourceWrite,
  listSourceWrites,
  runSourceControl,
} from "./review-queries";
vi.mock("./review-queries", async (original) => ({
  ...(await original<typeof import("./review-queries")>()),
  getSourceWrite: vi.fn(),
  listSourceWrites: vi.fn(),
  runSourceControl: vi.fn(),
}));
const id = (n: number) =>
  `${String(n).repeat(8)}-${String(n).repeat(4)}-4${String(n).repeat(3)}-8${String(n).repeat(3)}-${String(n).repeat(12)}`;
const scope = {
  origin: window.location.origin,
  actorId: id(1),
  orgId: id(2),
  projectId: id(3),
};
const item = {
  id: id(4),
  revision: 7,
  action: "update" as const,
  status: "pending" as const,
  expected_script_revision: 5,
  object_count: 4,
  confirmed_object_count: 2,
  cancellation_requested: true,
  needs_reconciliation: false,
  active_io: true,
  can_control: true,
  created_at: "2026-10-02T00:00:00Z",
  updated_at: "2026-10-02T00:00:00Z",
};
beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  vi.mocked(listSourceWrites).mockResolvedValue({
    current_actor_id: scope.actorId,
    current_org_id: scope.orgId,
    items: [item],
  });
  vi.mocked(getSourceWrite).mockResolvedValue(item);
});
afterEach(cleanup);
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const onClose = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <SourceWriteDialog scope={scope} onClose={onClose} />
    </QueryClientProvider>,
  );
  return { client, onClose };
}
it("实际owner尚存在仍可明确reconcile，202仅受理且不假清理/覆盖GET修订", async () => {
  vi.mocked(runSourceControl).mockResolvedValue({
    intent_id: item.id,
    revision: 8,
    action: "reconcile",
    accepted: true,
  });
  const { client } = mount();
  const action = await screen.findByRole("button", { name: "明确核验原保存" });
  await waitFor(() => expect(action.matches(":disabled")).toBe(false));
  expect(runSourceControl).not.toHaveBeenCalled();
  fireEvent.click(action);
  await waitFor(() =>
    expect(runSourceControl).toHaveBeenCalledWith(
      scope,
      {
        intentId: item.id,
        action: "reconcile",
        body: { expected_revision: 7 },
      },
      expect.any(String),
    ),
  );
  expect(
    await screen.findByText(
      /^原控制已受理。当前停止与清理状态来自重新读取，不以受理回执替代。$/,
    ),
  ).toBeTruthy();
  expect(screen.getByText(/原 I\/O owner 尚存在/)).toBeTruthy();
  expect(screen.queryByText("已取消且清理完成 · 修订 8")).toBeNull();
  expect(
    client.getQueryData([
      ...[
        "project",
        scope.projectId,
        "script",
        scope.origin,
        scope.actorId,
        scope.orgId,
      ],
      "review",
      "source-writes",
      "detail",
      item.id,
    ]),
  ).toEqual(item);
});
it("丢失控制响应保原键并将焦点移到可用核验，不能关闭或借新key重提", async () => {
  vi.mocked(runSourceControl).mockRejectedValue(new Error("response lost"));
  const { onClose } = mount();
  const action = await screen.findByRole("button", { name: "明确核验原保存" });
  await waitFor(() => expect(action.matches(":disabled")).toBe(false));
  action.focus();
  fireEvent.click(action);
  const recovery = await screen.findByRole("button", {
    name: "人工使用原键核验控制",
  });
  await waitFor(() => expect(document.activeElement).toBe(recovery));
  expect(runSourceControl).toHaveBeenCalledTimes(1);
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  expect(onClose).not.toHaveBeenCalled();
  expect(action.matches(":disabled")).toBe(true);
});
