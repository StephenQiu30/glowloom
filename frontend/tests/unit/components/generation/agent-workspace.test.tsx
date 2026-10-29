import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AgentWorkspace } from "@/components/generation/agent-workspace";

const calls = vi.hoisted(() => ({
  list: vi.fn(),
  read: vi.fn(),
  advance: vi.fn(),
  confirm: vi.fn(),
  create: vi.fn(),
  models: vi.fn(),
}));
vi.mock("@/api/agent", () => ({
  listAgentSessions: calls.list,
  getAgentSession: calls.read,
  advanceAgentSession: calls.advance,
  confirmAgentStage: calls.confirm,
  createAgentSession: calls.create,
  startAgentTurn: vi.fn(),
  stopAgentSession: vi.fn(),
}));
vi.mock("@/api/models", () => ({ listProjectModels: calls.models }));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: () => null,
}));
const base: API.Session = {
  id: "session",
  project_id: "project",
  revision: 3,
  title: "海边海报",
  turns: [
    {
      id: "turn",
      phase: "plan",
      status: "awaiting_confirmation",
      input: { message: "制作海报" },
      stages: [
        {
          operation_id: "original-operation",
          phase: "plan",
          quote_micros: 100,
          status: "quoted",
        },
      ],
    },
  ],
};
beforeEach(() => {
  vi.clearAllMocks();
  calls.list.mockResolvedValue([{ ...base, turns: [] }]);
  calls.read.mockResolvedValue(base);
  calls.models.mockResolvedValue({ items: [] });
});
afterEach(cleanup);
function mount() {
  render(
    <AgentWorkspace
      projectID="project"
      initialMessage=""
      headerActions={null}
      projectControl={null}
      onLock={() => {}}
    />,
  );
}
it("重新打开恢复原报价，显式确认只绑定该阶段", async () => {
  calls.confirm.mockResolvedValue({
    ...base,
    revision: 4,
    turns: [{ ...base.turns![0], status: "stopped" }],
  });
  mount();
  fireEvent.click(await screen.findByRole("button", { name: "确认规划" }));
  await waitFor(() =>
    expect(calls.confirm).toHaveBeenCalledWith(
      { pid: "project", sid: "session" },
      {
        turn_id: "turn",
        operation_id: "original-operation",
        expected_revision: 3,
      },
    ),
  );
  expect(calls.create).not.toHaveBeenCalled();
  expect(calls.advance).not.toHaveBeenCalled();
});
it("结果未确认时不自动再次确认或创建任务", async () => {
  calls.read.mockResolvedValue({
    ...base,
    turns: [{ ...base.turns![0], status: "needs_reconciliation" }],
  });
  mount();
  expect(
    await screen.findByRole("button", { name: "查询原任务" }),
  ).toBeTruthy();
  expect(calls.confirm).not.toHaveBeenCalled();
  expect(calls.advance).not.toHaveBeenCalled();
  expect(calls.create).not.toHaveBeenCalled();
});

it("未发送目标交还类型草稿，恢复值直接反映到编辑器", async () => {
  calls.list.mockResolvedValue([]);
  const change = vi.fn();
  const props = {
    projectID: "project",
    headerActions: null,
    projectControl: null,
    onLock: () => {},
    onMessageChange: change,
  };
  const page = render(<AgentWorkspace {...props} initialMessage="原目标" />);
  const editor = await screen.findByRole("textbox", { name: "Agent 创作目标" });
  fireEvent.change(editor, { target: { value: "海边海报" } });
  expect(change).toHaveBeenLastCalledWith("海边海报");
  page.rerender(<AgentWorkspace {...props} initialMessage="恢复后的目标" />);
  expect((editor as HTMLTextAreaElement).value).toBe("恢复后的目标");
  expect(calls.create).not.toHaveBeenCalled();
});
