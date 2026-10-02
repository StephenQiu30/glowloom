import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import * as queries from "./source-queries";
import { useSourceWriter } from "./use-source-writer";
vi.mock("./source-queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./source-queries")>()),
  getWorkspace: vi.fn(),
  runSourceIntent: vi.fn(),
}));
const scope = {
  origin: window.location.origin,
  actorId: "11111111-1111-4111-8111-111111111111",
  orgId: "22222222-2222-4222-8222-222222222222",
  projectId: "33333333-3333-4333-8333-333333333333",
};
const workspace = {
  current_actor_id: scope.actorId,
  current_org_id: scope.orgId,
  state: {
    project_id: scope.projectId,
    org_id: scope.orgId,
    revision: 0,
    updated_at: "0001-01-01T00:00:00Z",
  },
};
const command = {
  action: "create" as const,
  body: {
    expected_revision: 0,
    rights_confirmed: true as const,
    source_kind: "chapter" as const,
    title: "章",
    status: "draft" as const,
    document: { type: "doc" as const },
    provenance: {},
  },
};
const receipt = {
  script_revision: 1,
  project_revision: 9,
  version_id: "44444444-4444-4444-8444-444444444444",
  split_set_id: "55555555-5555-4555-8555-555555555555",
  source_mappings: [
    {
      source_lineage_id: "66666666-6666-4666-8666-666666666666",
      new_source_id: "66666666-6666-4666-8666-666666666666",
    },
  ],
  changed: true,
  duplicate: false,
};
beforeEach(() => {
  vi.resetAllMocks();
  sessionStorage.clear();
  vi.mocked(queries.getWorkspace).mockResolvedValue(workspace);
});
it("未知原保存重放遇撤权403仍保原键正文，不把后来拒绝当首次零写证明", async () => {
  vi.mocked(queries.runSourceIntent).mockRejectedValueOnce(
    new ApiError(0, "network_error"),
  );
  const hook = setup();
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  await act(() => hook.result.current.submit(command));
  const original = hook.result.current.intent;
  vi.mocked(queries.runSourceIntent).mockRejectedValueOnce(
    new ApiError(403, "forbidden"),
  );
  await act(() => hook.result.current.replay());
  expect(hook.result.current.intent).toEqual(original);
  expect(hook.result.current.locked).toBe(true);
  await act(() => hook.result.current.submit(command));
  expect(queries.runSourceIntent).toHaveBeenCalledTimes(2);
});
afterEach(cleanup);
function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const accepted = vi.fn();
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const hook = renderHook(() => useSourceWriter(scope, accepted), { wrapper });
  return { ...hook, client, accepted, wrapper };
}
it("真实unknown跨组件刷新持久原key/body，人工原键核验且不降较新缓存", async () => {
  vi.mocked(queries.runSourceIntent).mockRejectedValueOnce(
    new ApiError(0, "network_error"),
  );
  const first = setup();
  await waitFor(() => expect(first.result.current.ready).toBe(true));
  await act(async () => {
    await first.result.current.submit(command);
  });
  const original = first.result.current.intent;
  expect(original?.body.expected_revision).toBe(0);
  expect(first.result.current.locked).toBe(true);
  const event = new Event("beforeunload", { cancelable: true });
  window.dispatchEvent(event);
  expect(event.defaultPrevented).toBe(true);
  first.unmount();
  const second = setup();
  await waitFor(() => expect(second.result.current.intent).toEqual(original));
  second.client.setQueryData(queries.scriptWorkspaceKey(scope.projectId), {
    ...workspace,
    state: { ...workspace.state, revision: 99 },
  });
  vi.mocked(queries.runSourceIntent).mockResolvedValueOnce(receipt);
  await act(async () => {
    await second.result.current.replay();
  });
  expect(queries.runSourceIntent).toHaveBeenLastCalledWith(original);
  expect(second.result.current.intent).toBeNull();
  expect(
    second.client.getQueryData(queries.scriptWorkspaceKey(scope.projectId)),
  ).toMatchObject({ state: { revision: 99 } });
  expect(second.accepted).toHaveBeenCalledWith(receipt);
});
it("409保持草稿命令，读取最新后需明确解开CAS；非法响应保持unknown", async () => {
  vi.mocked(queries.runSourceIntent).mockRejectedValueOnce(
    new ApiError(409, "stale_revision"),
  );
  const hook = setup();
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  await act(async () => {
    await hook.result.current.submit(command);
  });
  expect(hook.result.current.rejected?.intent.body).toEqual(command.body);
  expect(hook.result.current.conflicted).toBe(true);
  await act(async () => {
    await hook.result.current.submit(command);
  });
  expect(queries.runSourceIntent).toHaveBeenCalledTimes(1);
  act(() => hook.result.current.acknowledgeLatest());
  vi.mocked(queries.runSourceIntent).mockRejectedValueOnce(
    new ApiError(502, "invalid_response"),
  );
  await act(async () => {
    await hook.result.current.submit(command);
  });
  expect(hook.result.current.intent).not.toBeNull();
  expect(hook.result.current.locked).toBe(true);
});
it("freshGET scope变更在发送前锁住原意图，恢复权限后仍须人工原键；存储失败不发送", async () => {
  const hook = setup();
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  vi.mocked(queries.getWorkspace).mockResolvedValueOnce({
    ...workspace,
    current_actor_id: scope.orgId,
  });
  await act(async () => {
    await hook.result.current.submit(command);
  });
  expect(queries.runSourceIntent).not.toHaveBeenCalled();
  expect(hook.result.current.intent).not.toBeNull();
  hook.unmount();
  sessionStorage.clear();
  const bad = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("quota");
  });
  const second = setup();
  await waitFor(() => expect(second.result.current.ready).toBe(true));
  await act(async () => {
    await second.result.current.submit(command);
  });
  expect(queries.runSourceIntent).not.toHaveBeenCalled();
  expect(second.result.current.storageError).toMatch(/存储/);
  bad.mockRestore();
});
