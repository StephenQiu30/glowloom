import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { createAgent } from "@ui-sdk/react/agent";
import { SWRConfig } from "swr";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CanvasOperationStatus } from "@/components/canvas/nodes/canvas-operation-status";
const api = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock("@/api/operations", () => ({ getOperation: api.get }));
const props = {
  projectID: "11111111-1111-4111-8111-111111111111",
  canvasID: "22222222-2222-4222-8222-222222222222",
  nodeID: "33333333-3333-4333-8333-333333333333",
  operationID: "44444444-4444-4444-8444-444444444444",
  identity: "actor/scope",
};
function task(status: string) {
  return {
    id: props.operationID,
    project_id: props.projectID,
    status,
    source: { canvas_id: props.canvasID, node_id: props.nodeID },
    outputs: [],
  };
}
function open(extra = {}) {
  return render(
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
      <CanvasOperationStatus {...props} {...extra} />
    </SWRConfig>,
  );
}
beforeEach(() => api.get.mockReset().mockResolvedValue(task("submitted")));
afterEach(cleanup);
it("生成中只显示动效，保留可访问进度且不出现状态文字或取消按钮", async () => {
  const { container } = open();
  await screen.findByRole("progressbar", { name: "生成中" });
  expect(container.textContent).toBe("");
  expect(screen.queryByRole("button")).toBeNull();
  expect(screen.getByRole("progressbar").hasAttribute("aria-valuenow")).toBe(
    false,
  );
  const agent = createAgent();
  expect(agent.snapshot().some((item) => item.label === "生成中")).toBe(true);
});
it("完成后将经过来源校验的结果交给原节点，且不渲染采用按钮", async () => {
  const completed = task("completed");
  api.get.mockResolvedValue(completed);
  const onCompleted = vi.fn();
  const { container } = open({ onCompleted });
  await waitFor(() =>
    expect(onCompleted).toHaveBeenCalledWith(
      props.nodeID,
      props.operationID,
      completed,
    ),
  );
  expect(container.textContent).toBe("");
  expect(screen.queryByRole("button")).toBeNull();
});
it("来源不符时不允许结果写回", async () => {
  api.get.mockResolvedValue({
    ...task("completed"),
    source: {
      canvas_id: props.canvasID,
      node_id: "55555555-5555-4555-8555-555555555555",
    },
  });
  const onCompleted = vi.fn();
  open({ onCompleted });
  await waitFor(() => expect(api.get).toHaveBeenCalled());
  expect(onCompleted).not.toHaveBeenCalled();
});
