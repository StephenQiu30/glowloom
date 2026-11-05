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
