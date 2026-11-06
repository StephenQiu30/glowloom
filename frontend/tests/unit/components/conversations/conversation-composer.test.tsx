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
  expect(
    screen
      .getByRole("group", { name: "创作输入" })
      .contains(screen.getByRole("button", { name: "模拟键盘提交" })),
  ).toBe(true);
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

it("searches available skill descriptions and keeps a selected version while filtering", () => {
  const value = controller();
  value.skills = [
    {
      skill_id: "script",
      version: "1",
      resource_hash: "script-hash",
      enabled: true,
      name: "创作分镜",
      description: "安排镜头和节奏",
    },
    {
      skill_id: "poster",
      version: "2",
      resource_hash: "poster-hash",
      enabled: true,
      name: "海报设计",
      description: "组织商品主图",
    },
  ];
  value.settings.skills = [
    { skill_id: "script", version: "1", resource_hash: "script-hash" },
  ];
  render(
    <ConversationComposer
      conversation={value}
      contextKey="test"
      projectID="project"
      authorized
      onChooseProject={() => {}}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "选择技能" }));
  const search = screen.getByRole("searchbox", { name: "搜索技能" });
  fireEvent.change(search, { target: { value: "  商品  " } });
  expect(screen.getByText("海报设计")).toBeTruthy();
  expect(screen.queryByText("创作分镜")).toBeNull();
  fireEvent.change(search, { target: { value: "无匹配" } });
  expect(screen.getByText("没有匹配的技能")).toBeTruthy();
  fireEvent.change(search, { target: { value: "" } });
  expect(
    screen
      .getByRole("checkbox", { name: /创作分镜/ })
      .getAttribute("aria-checked"),
  ).toBe("true");
  expect(value.changeSettings).not.toHaveBeenCalled();
});
