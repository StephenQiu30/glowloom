import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ConversationThread } from "@/components/conversations/conversation-thread";
import type { ConversationController } from "@/components/conversations/use-conversation";
vi.mock(
  "@/components/canvas/runtime/components/editor/markdown-content",
  () => ({ MarkdownContent: ({ value }: { value: string }) => <p>{value}</p> }),
);
vi.mock("@/components/conversations/conversation-result", () => ({
  ConversationResults: () => null,
}));
vi.mock("@/components/media/project-media", () => ({
  ProjectMediaPreview: ({ id }: { id: string }) => (
    <div data-testid="selected-shot" data-asset={id}>
      已选视频
    </div>
  ),
}));
const defaultResizeObserver = globalThis.ResizeObserver;
afterEach(() => {
  cleanup();
  vi.stubGlobal("ResizeObserver", defaultResizeObserver);
});
const source = {
  root_run_id: "old",
  session_id: "session",
  submission_id: "submission",
  goal: "不得显示的派生指令",
  input: { plain_text: "", media_pins: [] },
  manifest: {},
  execution_available: true,
  events: [],
  messages_page: { items: [], through: "1:1" },
  snapshot_cursor: "1:1",
  state: { status: "failed", execution_revision: 2 },
};
function controller() {
  return {
    summaries: [{ root_run_id: "old", accepted_revision: 1, status: "failed" }],
    runMap: {
      old: {
        snapshot: source,
        messages: [
          {
            message_id: "message",
            run_id: "old",
            part_index: 0,
            sequence: "1:1",
            kind: "assistant",
            content: "原来的回复",
          },
        ],
      },
    },
    activeRunID: "old",
    active: false,
    phase: "idle",
    stopping: false,
    unresolved: [],
    retry: vi.fn(),
    quoteMessage: vi.fn(),
    loadRun: vi.fn(),
    loadOlderMessages: vi.fn(),
    loadOlderRuns: vi.fn(),
    command: vi.fn(),
    refresh: vi.fn(),
  } as unknown as ConversationController;
}
it("keeps the original attempt and uses only the submitted plain text", () => {
  const c = controller();
  render(<ConversationThread conversation={c} projectID="project" />);
  expect(screen.getByText("原来的回复")).toBeTruthy();
  expect(screen.queryByText("不得显示的派生指令")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重新生成" }));
  expect(c.retry).toHaveBeenCalledWith("old");
});
it("shows pending Stop separately from execution completion", () => {
  const c = controller();
  c.stopping = true;
  c.active = true;
  c.runMap.old!.snapshot.state.status = "running";
  render(<ConversationThread conversation={c} projectID="project" />);
  expect(screen.getByText("正在停止，等待执行回执")).toBeTruthy();
  expect(screen.queryByText("已停止")).toBeNull();
});

it("follows late result-card resizing only while reading the newest messages", () => {
  let resized: (() => void) | undefined;
  const disconnect = vi.fn();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: () => void) {
        resized = callback;
      }
      observe() {}
      disconnect = disconnect;
    },
  );
  const { unmount } = render(
    <ConversationThread conversation={controller()} projectID="project" />,
  );
  const scroller = screen.getByLabelText("对话消息");
  Object.defineProperties(scroller, {
    clientHeight: { configurable: true, value: 400 },
    scrollHeight: { configurable: true, value: 900 },
  });
  act(() => resized?.());
  expect(scroller.scrollTop).toBe(900);
  Object.defineProperty(scroller, "scrollHeight", { value: 1100 });
  fireEvent.scroll(scroller);
  act(() => resized?.());
  expect(scroller.scrollTop).toBe(1100);
  scroller.scrollTop = 100;
  fireEvent.scroll(scroller);
  Object.defineProperty(scroller, "scrollHeight", { value: 1200 });
  act(() => resized?.());
  expect(scroller.scrollTop).toBe(100);
  expect(screen.getByRole("button", { name: "回到底部" })).toBeTruthy();
  unmount();
  expect(disconnect).toHaveBeenCalledOnce();
});

it("places the expandable public activity before the completed reply", () => {
  const c = controller();
  c.runMap.old!.snapshot.state = {
    ...c.runMap.old!.snapshot.state,
    status: "completed",
    plan: [{ id: "step", title: "完成文案", status: "completed" }],
  } as typeof c.runMap.old.snapshot.state;
  render(<ConversationThread conversation={c} projectID="project" />);
  const activity = screen.getByRole("button", { name: "已完成" });
  const reply = screen.getByText("原来的回复");
  expect(
    activity.compareDocumentPosition(reply) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  expect(screen.queryByText("完成文案")).toBeNull();
  fireEvent.click(activity);
  expect(screen.getByText("完成文案")).toBeTruthy();
  expect(screen.getByText("以上内容由 AI 生成")).toBeTruthy();
});

it("shows the exact selected shot and frozen edit before allowing approval", () => {
  const c = controller(),
    asset = "11111111-1111-4111-8111-111111111111";
  c.runMap.old!.snapshot.state = {
    ...c.runMap.old!.snapshot.state,
    status: "waiting_approval",
    proposal: {
      proposal_id: "proposal",
      authorization_revision: 1,
      candidate_hash: "hash",
      quote_micros: 0,
      candidate: {
        kind: "shot_delivery",
        asset_id: asset,
        input: {
          source_start_ms: 250,
          duration_ms: 500,
          aspect_ratio: "1:1",
          fps: 25,
          project_format: "fcpxml",
        },
      },
    },
  } as typeof c.runMap.old.snapshot.state;
  render(<ConversationThread conversation={c} projectID="project" />);
  expect(screen.getByTestId("selected-shot").getAttribute("data-asset")).toBe(
    asset,
  );
  expect(screen.getByText("裁剪起点 0.25 秒 · 时长 0.5 秒")).toBeTruthy();
  expect(screen.getByText("1:1 · 25 fps · FCPXML 工程")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "批准方案" }));
  expect(c.command).toHaveBeenCalledWith("approve");
});

it("blocks approval of an unreadable shot contract", () => {
  const c = controller();
  c.runMap.old!.snapshot.state = {
    ...c.runMap.old!.snapshot.state,
    status: "waiting_approval",
    proposal: {
      proposal_id: "proposal",
      authorization_revision: 1,
      candidate_hash: "hash",
      candidate: {
        kind: "shot_delivery",
        asset_id: "11111111-1111-4111-8111-111111111111",
        input: {
          source_start_ms: 0,
          duration_ms: 500,
          aspect_ratio: "1:1",
          fps: 25,
          project_format: "__proto__",
        },
      },
    },
  } as typeof c.runMap.old.snapshot.state;
  render(<ConversationThread conversation={c} projectID="project" />);
  expect(screen.getByRole("alert").textContent).toContain(
    "制作方案暂时无法读取",
  );
  expect(
    screen.getByRole("button", { name: "批准方案" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(screen.queryByTestId("selected-shot")).toBeNull();
});

it.each(["needs_reconciliation", "failed"])(
  "recovers the original %s root through pause/resume, refusing unavailable definitions",
  (status) => {
    const c = controller();
    c.runMap.old!.snapshot = {
      ...c.runMap.old!.snapshot,
      execution_available: true,
      state: { ...c.runMap.old!.snapshot.state, status },
      recovery:
        status === "failed"
          ? []
          : [{ kind: "run", parked_at: "2026-11-12T00:00:00Z" }],
    };
    const view = render(
      <ConversationThread conversation={c} projectID="project" writable />,
    );
    fireEvent.click(screen.getByRole("button", { name: "暂停以核对原任务" }));
    expect(c.command).toHaveBeenCalledWith("pause");
    c.runMap.old!.snapshot.state.status = "paused";
    view.rerender(
      <ConversationThread conversation={c} projectID="project" writable />,
    );
    fireEvent.click(screen.getByRole("button", { name: "继续创作" }));
    expect(c.command).toHaveBeenCalledWith("resume");
    c.runMap.old!.snapshot.execution_available = false;
    view.rerender(
      <ConversationThread conversation={c} projectID="project" writable />,
    );
    expect(
      (screen.getByRole("button", { name: "继续创作" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
  },
);
