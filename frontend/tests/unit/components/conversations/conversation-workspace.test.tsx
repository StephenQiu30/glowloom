import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ConversationWorkspace } from "@/components/conversations/conversation-workspace";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import { draftText } from "@/components/conversations/conversation-state";
import type { ConversationController } from "@/components/conversations/use-conversation";
import { render as renderFeedback, findFeedback } from "../../../feedback";

const { useConversation } = vi.hoisted(() => ({ useConversation: vi.fn() }));
vi.mock("@/components/conversations/use-conversation", () => ({
  useConversation,
}));
vi.mock("@/components/conversations/conversation-composer", () => ({
  ConversationComposer: () => <div>创作输入</div>,
}));
vi.mock("@/components/conversations/conversation-thread", () => ({
  ConversationThread: () => <div>历史消息</div>,
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const featured = {
  skill_id: "image-series",
  version: "2",
  resource_hash: "image-series-hash",
  name: "系列套图生成",
  description: "完成系列图片方案。",
  enabled: true,
};
const existing = {
  skill_id: "other",
  version: "1",
  resource_hash: "other-hash",
};

function setup(overrides: Partial<ConversationController> = {}) {
  const conversation = {
    draft: emptyPromptDraft(),
    summaries: [],
    loading: false,
    denied: false,
    active: false,
    connection: "connected",
    unresolved: [],
    phase: "idle",
    skills: [featured],
    settings: { budget: "1", skills: [existing] },
    changeSettings: vi.fn(),
    changeDraft: vi.fn(),
    send: vi.fn(),
    ...overrides,
  };
  useConversation.mockReturnValue(conversation);
  return conversation;
}

const props = {
  ownerKey: "actor/space",
  projectID: "project",
  enabled: true,
  onChooseProject: vi.fn(),
};

it("发送结果待确认时，即使错误文本被刷新清空也保留查询入口", async () => {
  const refresh = vi.fn();
  const c = setup({
    phase: "unknown",
    error: "",
    refresh,
    canRetrySubmission: false,
  });
  renderFeedback(<ConversationWorkspace {...props} />);
  expect((await findFeedback()).textContent).toBe("发送结果待确认");
  fireEvent.click(screen.getByRole("button", { name: "查看状态" }));
  expect(refresh).toHaveBeenCalledOnce();
  expect(c.send).not.toHaveBeenCalled();
});

it("模型目录读取失败时在输入区保留重试，且不会提交对话", () => {
  const refresh = vi.fn();
  const c = setup({ modelCatalogError: "服务暂时不可用", refresh });
  const view = render(<ConversationWorkspace {...props} />);

  expect(screen.getByRole("status").textContent).toContain("无法读取对话模型");
  fireEvent.click(screen.getByRole("button", { name: "重试" }));
  expect(refresh).toHaveBeenCalledOnce();
  expect(c.send).not.toHaveBeenCalled();

  useConversation.mockReturnValue({ ...c, modelCatalogError: "" });
  view.rerender(<ConversationWorkspace {...props} />);
  expect(screen.queryByText("无法读取对话模型")).toBeNull();
});

it("selects the exact enabled skill version without changing the draft or sending", () => {
  const c = setup({
    skills: [
      featured,
      { ...featured, skill_id: "disabled", name: "停用技能", enabled: false },
      {
        ...featured,
        skill_id: "incomplete",
        name: "未绑定技能",
        resource_hash: undefined,
      },
    ],
  });
  render(<ConversationWorkspace {...props} />);
  expect(
    screen.getByRole("heading", { name: "你好，想创作什么？" }),
  ).toBeTruthy();
  expect(screen.queryByText("停用技能")).toBeNull();
  expect(screen.queryByText("未绑定技能")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "试一试：系列套图生成" }));
  expect(c.changeSettings).toHaveBeenCalledWith({
    budget: "1",
    skills: [
      existing,
      {
        skill_id: featured.skill_id,
        version: featured.version,
        resource_hash: featured.resource_hash,
      },
    ],
  });
  expect(c.changeDraft).not.toHaveBeenCalled();
  expect(c.send).not.toHaveBeenCalled();
});

it("首页 Agent 只显示当前项目可用的版本化技能，点击只选择技能", () => {
  const c = setup({
    skills: [
      featured,
      { ...featured, skill_id: "disabled", name: "停用技能", enabled: false },
      {
        ...featured,
        skill_id: "missing",
        name: "未绑定技能",
        resource_hash: undefined,
      },
    ],
  });
  const view = render(
    <ConversationWorkspace {...props} embedded homeGeneration />,
  );
  expect(screen.getByRole("group", { name: "推荐技能" })).toBeTruthy();
  expect(screen.queryByText("停用技能")).toBeNull();
  expect(screen.queryByText("未绑定技能")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: featured.name }));
  expect(c.changeSettings).toHaveBeenCalledWith({
    budget: "1",
    skills: [
      existing,
      {
        skill_id: featured.skill_id,
        version: featured.version,
        resource_hash: featured.resource_hash,
      },
    ],
  });
  expect(c.send).not.toHaveBeenCalled();
  view.rerender(
    <ConversationWorkspace
      {...props}
      enabled={false}
      embedded
      homeGeneration
    />,
  );
  expect(screen.queryByRole("group", { name: "推荐技能" })).toBeNull();
});

it("shows the available starters without a project and fills a real prompt without selecting a skill or sending", () => {
  const c = setup();
  render(<ConversationWorkspace {...props} enabled={false} />);
  expect(screen.getByText("创作输入")).toBeTruthy();
  expect(screen.getAllByRole("button", { name: /试一试/ })).toHaveLength(2);
  fireEvent.click(screen.getByRole("button", { name: "试一试：系列套图生成" }));
  const draft = vi.mocked(c.changeDraft).mock.calls[0]![0];
  expect(draftText(draft)).toContain("图片方案");
  expect(c.changeSettings).not.toHaveBeenCalled();
  expect(c.send).not.toHaveBeenCalled();
});

it("keeps existing input intact when trying a starter", () => {
  const draft = emptyPromptDraft();
  draft.document.content = [
    { type: "paragraph", content: [{ type: "text", text: "原有创作草稿" }] },
  ];
  const c = setup({ draft, skills: [] });
  render(<ConversationWorkspace {...props} />);
  fireEvent.click(screen.getByRole("button", { name: "试一试：系列套图生成" }));
  expect(draftText(vi.mocked(c.changeDraft).mock.calls[0]![0])).toMatch(
    /^原有创作草稿\n请根据/,
  );
  expect(c.send).not.toHaveBeenCalled();
});

it("keeps history pages separate from the new conversation landing", () => {
  setup();
  render(<ConversationWorkspace {...props} sessionID="session" />);
  expect(
    screen.queryByRole("heading", { name: "你好，想创作什么？" }),
  ).toBeNull();
  expect(screen.queryByRole("button", { name: /试一试/ })).toBeNull();
});
