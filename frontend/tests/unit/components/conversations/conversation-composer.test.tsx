import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import { ConversationComposer } from "@/components/conversations/conversation-composer";
import type { ConversationController } from "@/components/conversations/use-conversation";
vi.mock("@/components/generation/prompt-editor", () => ({
  PromptEditor: ({ onRequestSubmit }: { onRequestSubmit: () => void }) => (
    <button onClick={onRequestSubmit}>模拟键盘提交</button>
  ),
}));
vi.mock("@/components/generation/model-picker", () => ({
  ModelPicker: ({ onChooseProject }: { onChooseProject?: () => void }) => (
    <span>{onChooseProject ? "需要选择项目" : "使用当前项目目录"}</span>
  ),
}));
afterEach(cleanup);
function controller(active = false) {
  return {
    draft: emptyPromptDraft(),
    phase: "idle",
    active,
    stopping: false,
    canSend: !active,
    models: [],
    skills: [],
    settings: { budget: "1", skills: [] },
    quote: null,
    changeDraft: vi.fn(),
    changeSettings: vi.fn(),
    clearQuote: vi.fn(),
    send: vi.fn(),
    stop: vi.fn(),
  } as unknown as ConversationController;
}
it("submits from the keyboard only when sending is eligible", () => {
  const value = controller();
  render(
    <ConversationComposer
      conversation={value}
      contextKey="test"
      projectID="project"
      authorized
      onChooseProject={() => {}}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "模拟键盘提交" }));
  expect(value.send).toHaveBeenCalledTimes(1);
});
it("never interprets an editing shortcut as stopping the current run", () => {
  const value = controller(true);
  render(
    <ConversationComposer
      conversation={value}
      contextKey="test"
      projectID="project"
      authorized
      onChooseProject={() => {}}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "模拟键盘提交" }));
  expect(value.stop).not.toHaveBeenCalled();
  expect(value.send).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "停止" }));
  expect(value.stop).toHaveBeenCalledTimes(1);
});

it("offers the current project model catalog after authorization", () => {
  render(
    <ConversationComposer
      conversation={controller()}
      contextKey="test"
      projectID="project"
      authorized
      onChooseProject={() => {}}
    />,
  );
  expect(screen.getByText("使用当前项目目录")).toBeTruthy();
});
