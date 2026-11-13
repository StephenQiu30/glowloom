import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { SWRConfig } from "swr";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useCanvasOperationTasks } from "@/components/canvas/hooks/use-canvas-operation-tasks";
const api = vi.hoisted(() => ({ list: vi.fn(), error: vi.fn() }));
vi.mock("@/api/operations", () => ({ listProjectOperations: api.list }));
vi.mock("@/components/feedback/notification", () => ({
  toast: { error: api.error },
}));
const projectID = "11111111-1111-4111-8111-111111111111";
const canvasID = "22222222-2222-4222-8222-222222222222";
const nodeID = "33333333-3333-4333-8333-333333333333";
const operationID = "44444444-4444-4444-8444-444444444444";
const oldOperationID = "55555555-5555-4555-8555-555555555555";
const otherNodeID = "66666666-6666-4666-8666-666666666666";
const props = { projectID, canvasID, identity: "actor/scope" };
function task(
  id = operationID,
  status: API.OperationStatus = "submitted",
  node = nodeID,
) {
  return {
    id,
    project_id: projectID,
    status,
    source: { canvas_id: canvasID, node_id: node },
  };
}
function wrapper() {
  const cache = new Map();
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <SWRConfig value={{ provider: () => cache, dedupingInterval: 0 }}>
        {children}
      </SWRConfig>
    );
  };
}
beforeEach(() => {
  api.list.mockReset().mockResolvedValue({ items: [] });
  api.error.mockReset();
});
afterEach(cleanup);
it("分页恢复各节点最新的正式任务，已完成任务不遮住结果", async () => {
  api.list
    .mockResolvedValueOnce({ items: [task()], next_cursor: "next" })
    .mockResolvedValueOnce({
      items: [
        task(oldOperationID),
        task(oldOperationID, "completed", otherNodeID),
      ],
    });
  const { result } = renderHook(() => useCanvasOperationTasks(props), {
    wrapper: wrapper(),
  });
  await waitFor(() =>
    expect(result.current.tasks).toEqual({ [nodeID]: operationID }),
  );
  expect(api.list.mock.calls[1][0]).toMatchObject({
    pid: projectID,
    canvas_id: canvasID,
    cursor: "next",
  });
});
it("新任务立即绑定，旧任务完成回执不能清掉新任务", async () => {
  const { result } = renderHook(() => useCanvasOperationTasks(props), {
    wrapper: wrapper(),
  });
  await waitFor(() => expect(api.list).toHaveBeenCalledOnce());
  await act(async () => {
    result.current.register(nodeID, operationID);
  });
  await waitFor(() => expect(result.current.tasks[nodeID]).toBe(operationID));
  await act(async () => {
    result.current.complete(nodeID, oldOperationID);
  });
  expect(result.current.tasks[nodeID]).toBe(operationID);
  await act(async () => {
    result.current.complete(nodeID, operationID);
  });
  await waitFor(() => expect(result.current.tasks[nodeID]).toBeUndefined());
});
it("拒绝跨画布来源并通过统一通知提供读取重试", async () => {
  api.list.mockResolvedValue({
    items: [{ ...task(), source: { canvas_id: operationID, node_id: nodeID } }],
  });
  const { result } = renderHook(() => useCanvasOperationTasks(props), {
    wrapper: wrapper(),
  });
  await waitFor(() => expect(api.error).toHaveBeenCalled());
  expect(result.current.tasks).toEqual({});
  expect(api.error.mock.calls[0][1].action.label).toBe("重试");
});
it("重复分页游标失败退出，不循环请求", async () => {
  api.list.mockResolvedValue({ items: [task()], next_cursor: "same" });
  renderHook(() => useCanvasOperationTasks(props), { wrapper: wrapper() });
  await waitFor(() => expect(api.error).toHaveBeenCalled());
  expect(api.list).toHaveBeenCalledTimes(2);
});
