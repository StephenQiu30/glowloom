import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ServiceTrial } from "@/app/providers/_components/service-trial";
const fixtures = vi.hoisted(() => ({
  start: vi.fn(),
  get: vi.fn(),
  turn: vi.fn(),
  recover: vi.fn(),
  cancel: vi.fn(),
  state: {} as API.TrialState,
}));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ actor: { id: "11111111-1111-4111-8111-111111111111" } }),
}));
vi.mock("@/api/settings", () => ({
  startAiTrial: fixtures.start,
  getAiTrial: fixtures.get,
  continueAiTrial: fixtures.turn,
  recoverAiTrial: fixtures.recover,
  cancelAiTrial: fixtures.cancel,
  getAiTrialArtifact: vi.fn(),
}));
const savedKey = "glowloom.ai-trial.v1.11111111-1111-4111-8111-111111111111";
beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  fixtures.get.mockImplementation(async () => fixtures.state);
  fixtures.start.mockImplementation(async (req: API.TrialRequest) => {
    fixtures.state = {
      state: "succeeded",
      kind: req.kind,
      model: req.model,
      turns: [
        {
          request_id: req.request_id,
          prompt: req.prompt,
          state: "succeeded",
          text: req.kind === "chat" ? "真实接口夹具回复" : "",
          models: ["fixture-model"],
          authenticated: true,
          image_generation: true,
        },
      ],
    };
    return fixtures.state;
  });
  fixtures.turn.mockImplementation(
    async (_: unknown, turn: API.TrialMessage) => {
      fixtures.state = {
        ...fixtures.state,
        turns: [
          ...fixtures.state.turns!,
          { ...turn, state: "succeeded", text: "同会话第二轮" },
        ],
      };
      return fixtures.state;
    },
  );
});
afterEach(cleanup);
it("检查本机后发起无计费聊天，并沿同一试验继续下一轮", async () => {
  render(<ServiceTrial open onOpenChange={() => {}} providers={[]} />);
  fireEvent.click(screen.getByRole("button", { name: "检查 Codex" }));
  await screen.findByText("已登录 · 支持生图");
  fireEvent.change(screen.getByLabelText("提示词"), {
    target: { value: "hello" },
  });
  fireEvent.click(screen.getByRole("button", { name: "开始试验" }));
  await screen.findByText("真实接口夹具回复");
  const started = fixtures.start.mock.calls[1][0];
  expect(started.source).toBe("codex");
  expect(started).not.toHaveProperty("price");
  expect(started).not.toHaveProperty("project_id");
  fireEvent.change(screen.getByLabelText("继续聊天"), {
    target: { value: "continue" },
  });
  fireEvent.click(screen.getByRole("button", { name: "发送" }));
  await screen.findByText("同会话第二轮");
  expect(fixtures.turn.mock.calls[0][0]).toEqual({ id: started.request_id });
  expect(fixtures.start).toHaveBeenCalledTimes(2);
});
it("刷新恢复未知图片，只查证原结果且不重新生成", async () => {
  const id = crypto.randomUUID();
  localStorage.setItem(savedKey, id);
  fixtures.state = {
    state: "unknown",
    kind: "image",
    model: "fixture-model",
    turns: [{ request_id: id, prompt: "circle", state: "unknown" }],
  };
  fixtures.recover.mockImplementation(async () => {
    fixtures.state = {
      ...fixtures.state,
      state: "succeeded",
      turns: [{ state: "succeeded", text: "已恢复" }],
    };
  });
  render(<ServiceTrial open onOpenChange={() => {}} providers={[]} />);
  await screen.findByText("结果待确认");
  fireEvent.click(screen.getByRole("button", { name: "查证结果" }));
  await screen.findByText("已恢复");
  expect(fixtures.recover).toHaveBeenCalledWith({ id });
  expect(fixtures.start).not.toHaveBeenCalled();
});
it("关闭对话框不取消任务，重开只查询原试验", async () => {
  const id = crypto.randomUUID();
  localStorage.setItem(savedKey, id);
  fixtures.state = {
    state: "succeeded",
    kind: "chat",
    model: "fixture-model",
    turns: [{ state: "succeeded", text: "恢复的对话" }],
  };
  const { rerender } = render(
    <ServiceTrial open={false} onOpenChange={() => {}} providers={[]} />,
  );
  expect(fixtures.get).not.toHaveBeenCalled();
  rerender(<ServiceTrial open onOpenChange={() => {}} providers={[]} />);
  await screen.findByText("恢复的对话");
  await waitFor(() => expect(fixtures.get).toHaveBeenCalled());
  expect(fixtures.start).not.toHaveBeenCalled();
  expect(fixtures.cancel).not.toHaveBeenCalled();
  expect(screen.getByRole("button", { name: "发送" })).toBeTruthy();
});

it("刷新时保留待确认轮次，只查询直到原轮次出现", async () => {
  const id = crypto.randomUUID(),
    turnID = crypto.randomUUID();
  localStorage.setItem(savedKey, id);
  localStorage.setItem(`${savedKey}.turn`, turnID);
  fixtures.state = {
    state: "succeeded",
    kind: "chat",
    model: "fixture-model",
    turns: [
      {
        request_id: turnID,
        prompt: "pending",
        state: "succeeded",
        text: "原轮次的最终回答",
      },
    ],
  };
  render(<ServiceTrial open onOpenChange={() => {}} providers={[]} />);
  await screen.findByText("原轮次的最终回答");
  await waitFor(() =>
    expect(localStorage.getItem(`${savedKey}.turn`)).toBeNull(),
  );
  expect(fixtures.start).not.toHaveBeenCalled();
  expect(fixtures.turn).not.toHaveBeenCalled();
});
