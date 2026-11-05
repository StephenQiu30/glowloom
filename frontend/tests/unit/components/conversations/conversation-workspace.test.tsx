import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ConversationWorkspace } from "@/components/conversations/conversation-workspace";
import type { ConversationController } from "@/components/conversations/use-conversation";

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
  skill_id: "storyboard",
  version: "2",
  resource_hash: "storyboard-hash",
  name: "叙事短片导演分镜",
  description: "完成分镜与创作提示词。",
  enabled: true,
};
const existing = {
  skill_id: "other",
  version: "1",
  resource_hash: "other-hash",
};

function setup(overrides: Partial<ConversationController> = {}) {
  const conversation = {
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
  fireEvent.click(
    screen.getByRole("button", { name: "试一试：叙事短片导演分镜" }),
  );
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

it("keeps the empty composer available without showing recommendations from an unavailable project", () => {
  setup();
  render(<ConversationWorkspace {...props} enabled={false} />);
  expect(screen.getByText("创作输入")).toBeTruthy();
  expect(screen.queryByRole("button", { name: /试一试/ })).toBeNull();
});

it("keeps history pages separate from the new conversation landing", () => {
  setup();
  render(<ConversationWorkspace {...props} sessionID="session" />);
  expect(
    screen.queryByRole("heading", { name: "你好，想创作什么？" }),
  ).toBeNull();
  expect(screen.queryByRole("button", { name: /试一试/ })).toBeNull();
});
