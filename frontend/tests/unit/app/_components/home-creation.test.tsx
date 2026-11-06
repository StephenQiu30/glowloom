import { useEffect } from "react";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { Button } from "@/components/ui/button";
import { ConversationWorkspace } from "@/components/conversations/conversation-workspace";
import { ConversationProvider } from "@/components/conversations/conversation-context";
import { HomeCreation } from "@/app/_components/home-creation";
import type { RichGeneration } from "@/components/generation/rich-generation";

const state = vi.hoisted(() => ({
  session: null as { actor: { id: string } } | null,
  save: vi.fn<() => Promise<boolean>>(),
  opened: [] as { capability: string; draftID?: string }[],
  login: vi.fn(),
  projectID: "test-project",
  generatedID: undefined as string | undefined,
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => state.session,
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({
    selected: { id: "22222222-2222-4222-8222-222222222222", writable: true },
  }),
}));
vi.mock("@/api/projects", () => ({
  ensureIndependentCreation: vi.fn(async () => ({
    id: "33333333-3333-4333-8333-333333333333",
    canvas_id: null,
    name: "独立创作",
    aspect_ratio: "16:9",
    style_key: null,
    revision: 1,
    update_time: "2026-11-06T00:00:00Z",
  })),
}));
vi.mock("@/components/layout/basic-layout", () => ({
  useLoginDialog: () => state.login,
}));
vi.mock("@/components/projects/new-canvas-button", () => ({
  NewCanvasButton: () => <Button>新建画布</Button>,
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("next/dynamic", () => ({
  default: (loader: () => unknown) =>
    loader.toString().includes("conversation-workspace")
      ? ConversationWorkspace
      : WorkspaceStub,
}));
vi.mock("@/components/projects/project-picker", () => ({
  ProjectPicker: ({
    onChange,
    disabled,
    open,
  }: {
    onChange: (p: { id: string; name: string }) => void;
    disabled: boolean;
    open: boolean;
  }) => (
    <Button
      disabled={disabled}
      aria-expanded={open}
      onClick={() => onChange({ id: state.projectID, name: "验证项目" })}
    >
      选择验证项目
    </Button>
  ),
}));
function WorkspaceStub(props: React.ComponentProps<typeof RichGeneration>) {
  const { onCloseGuard } = props;
  useEffect(() => {
    state.opened.push({
      capability: props.seed.capability,
      draftID: props.draftID,
    });
    props.onDraftReady?.(
      props.draftID ?? state.generatedID ?? `${props.seed.capability}-draft`,
    );
    // This test verifies the page's save-before-switch contract; editor persistence has its own real tests.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  useEffect(() => {
    onCloseGuard?.(state.save);
    return () => onCloseGuard?.(null);
  }, [onCloseGuard]);
  return (
    <div>
      <p>{props.seed.capability} 富文本编辑器</p>
      {props.headerActions}
      {props.projectControl}
      <Button onClick={() => props.onLock?.(true)}>冻结报价</Button>
    </div>
  );
}
function Page() {
  return <HomeCreation />;
}
beforeEach(() => {
  state.session = null;
  state.projectID = "test-project";
  state.generatedID = undefined;
  localStorage.clear();
  state.opened = [];
  state.save.mockReset().mockResolvedValue(true);
  state.login.mockReset();
});
afterEach(cleanup);

it("登录后直接进入生图和生视频，不显示项目选择器", async () => {
  state.session = { actor: { id: "test-actor" } };
  render(<Page />);
  await screen.findByText("image 富文本编辑器");
  expect(screen.queryByRole("button", { name: /选择.*项目/ })).toBeNull();
  await tab("视频生成");
  await screen.findByText("video 富文本编辑器");
  expect(screen.queryByRole("button", { name: /选择.*项目/ })).toBeNull();
});

async function tab(name: string) {
  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "创作类型" })).toHaveProperty(
      "disabled",
      false,
    ),
  );
  fireEvent.keyDown(screen.getByRole("combobox", { name: "创作类型" }), {
    key: "ArrowDown",
  });
  fireEvent.click(
    await screen.findByRole("option", { name: new RegExp(name) }),
  );
}

it("使用紧凑类型菜单，游客切换输入不丢失且没有推荐技能行", async () => {
  render(<Page />);
  expect(screen.getByRole("combobox", { name: "创作类型" })).toBeTruthy();
  fireEvent.paste(await screen.findByLabelText("创作提示词"), {
    clipboardData: { files: [], getData: () => "保留这段想法" },
  });
  await tab("视频生成");
  await waitFor(() =>
    expect(
      screen.getByRole("combobox", { name: "创作类型" }).textContent,
    ).toContain("视频生成"),
  );
  expect(screen.getByLabelText("创作提示词")).toHaveProperty("textContent", "");
  await tab("图片生成");
  expect(screen.getByLabelText("创作提示词").textContent).toBe("保留这段想法");
  fireEvent.click(screen.getByRole("button", { name: "开始生成" }));
  expect(state.login).toHaveBeenCalledOnce();
  expect(screen.queryByLabelText("推荐提示技能")).toBeNull();
  expect(state.opened).toEqual([]);
});

it("Agent 入口先登录和选择项目，不再显示暂未开放", async () => {
  render(<Page />);
  await tab("Agent 模式");
  expect(screen.queryByText(/暂未开放/)).toBeNull();
  fireEvent.paste(await screen.findByLabelText("对话创作输入"), {
    clipboardData: { files: [], getData: () => "海边海报" },
  });
  fireEvent.click(screen.getByRole("button", { name: "发送" }));
  expect(state.login).toHaveBeenCalledOnce();
});

it("保存未确认禁止切换，确认后按类型恢复原草稿指针", async () => {
  state.session = { actor: { id: "test-actor" } };
  render(<Page />);
  await screen.findByText("image 富文本编辑器");
  state.save.mockResolvedValue(false);
  await tab("视频生成");
  await waitFor(() => expect(state.save).toHaveBeenCalledOnce());
  expect(screen.getByText("image 富文本编辑器")).toBeTruthy();
  expect(state.opened).toHaveLength(1);
  state.save.mockResolvedValue(true);
  await tab("视频生成");
  await screen.findByText("video 富文本编辑器");
  await tab("图片生成");
  await screen.findByText("image 富文本编辑器");
  expect(state.opened.at(-1)).toEqual({
    capability: "image",
    draftID: "image-draft",
  });
});

it("报价冻结期间不能切换类型或工作模式", async () => {
  state.session = { actor: { id: "test-actor" } };
  render(<Page />);
  fireEvent.click(await screen.findByRole("button", { name: "冻结报价" }));
  expect(screen.getByRole("combobox", { name: "创作类型" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(screen.getByRole("radio", { name: "画布" })).toHaveProperty(
    "disabled",
    true,
  );
});

it("切换画布后保留输入，再切回生成仍可编辑且不提交请求", async () => {
  render(<Page />);
  fireEvent.paste(await screen.findByLabelText("创作提示词"), {
    clipboardData: { files: [], getData: () => "一片安静的海" },
  });
  fireEvent.click(screen.getByRole("radio", { name: "画布" }));
  await waitFor(() =>
    expect(screen.getByLabelText("画布创作想法").textContent).toBe(""),
  );
  fireEvent.paste(screen.getByLabelText("画布创作想法"), {
    clipboardData: { files: [], getData: () => "，傍晚" },
  });
  expect(screen.getByLabelText("画布创作想法").textContent).toBe("，傍晚");
  fireEvent.click(screen.getByRole("radio", { name: "生成" }));
  await waitFor(() =>
    expect(screen.getByLabelText("创作提示词").textContent).toBe(
      "一片安静的海",
    ),
  );
  expect(state.login).not.toHaveBeenCalled();
  expect(state.opened).toEqual([]);
});

it("六类文字草稿往返隔离，首次选择目标类型为空", async () => {
  render(<Page />);
  for (const name of [
    "图片生成",
    "视频生成",
    "Agent 模式",
    "音乐生成",
    "音频生成",
    "数字人",
  ]) {
    await tab(name);
    const editor = await screen.findByLabelText(
      name === "Agent 模式" ? "对话创作输入" : "创作提示词",
    );
    expect(editor.textContent).toBe("");
    fireEvent.paste(editor, {
      clipboardData: { files: [], getData: () => name + "独立内容" },
    });
  }
  for (const name of [
    "图片生成",
    "视频生成",
    "Agent 模式",
    "音乐生成",
    "音频生成",
    "数字人",
  ]) {
    await tab(name);
    expect(
      (
        await screen.findByLabelText(
          name === "Agent 模式" ? "对话创作输入" : "创作提示词",
        )
      ).textContent,
    ).toBe(name + "独立内容");
  }
});

it("账号切换立即隔离独立创作的草稿指针", async () => {
  state.session = { actor: { id: "account-a" } };
  const page = render(<Page />);
  await screen.findByText("image 富文本编辑器");
  await screen.findByText("image 富文本编辑器");
  await tab("视频生成");
  await screen.findByText("video 富文本编辑器");
  await tab("图片生成");
  await screen.findByText("image 富文本编辑器");
  expect(state.opened.at(-1)?.draftID).toBe("image-draft");
  state.session = { actor: { id: "account-b" } };
  page.rerender(<Page />);
  await screen.findByText("image 富文本编辑器");
  expect(state.opened.at(-1)?.draftID).toBeUndefined();
});

it("音频独立进入 audio 工作区且未选项目时没有素材引用入口", async () => {
  state.session = { actor: { id: "test-actor" } };
  render(<Page />);
  await screen.findByText("image 富文本编辑器");
  await tab("音频生成");
  expect(screen.queryByRole("button", { name: "添加参考素材" })).toBeNull();
  expect(
    screen.queryByRole("button", { name: "添加主体或素材引用" }),
  ).toBeNull();
  expect(await screen.findByText("文本朗读")).toBeTruthy();
  fireEvent.click(await screen.findByRole("button", { name: "选择验证项目" }));
  await screen.findByText("audio 富文本编辑器");
  await tab("视频生成");
  await screen.findByText("video 富文本编辑器");
  await tab("音频生成");
  await screen.findByText("audio 富文本编辑器");
  expect(state.opened.at(-1)).toEqual({
    capability: "audio",
    draftID: "audio-draft",
  });
});

it("动作模仿未选项目时解释双参考要求，选择项目后进入视频工作区", async () => {
  state.session = { actor: { id: "test-actor" } };
  render(<Page />);
  await screen.findByText("image 富文本编辑器");
  await tab("动作模仿");
  expect(screen.queryByRole("textbox", { name: "创作提示词" })).toBeNull();
  expect(
    await screen.findByText(/选择项目后添加角色图片与动作视频/),
  ).toBeTruthy();
  fireEvent.click(await screen.findByRole("button", { name: "选择验证项目" }));
  await screen.findByText("video 富文本编辑器");
  expect(state.opened.at(-1)?.capability).toBe("video");
});

it("数字人台词与音乐选项各自保留且不进入其他类型", async () => {
  render(<Page />);
  await tab("数字人");
  fireEvent.paste(await screen.findByLabelText("创作提示词"), {
    clipboardData: { files: [], getData: () => "你好，欢迎来到这里" },
  });
  expect(screen.getByText(/选择项目后添加角色图片与讲话音频/)).toBeTruthy();
  await tab("音乐生成");
  expect(screen.getByLabelText("创作提示词").textContent).toBe("");
  fireEvent.click(screen.getByRole("radio", { name: "纯音乐" }));
  await tab("数字人");
  expect(screen.getByLabelText("创作提示词").textContent).toBe(
    "你好，欢迎来到这里",
  );
  expect(screen.queryByRole("radio", { name: "纯音乐" })).toBeNull();
  await tab("音乐生成");
  expect(
    screen.getByRole("radio", { name: "纯音乐" }).getAttribute("aria-checked"),
  ).toBe("true");
});

it("项目音频草稿页面内复用，重新打开不复用本机指针", async () => {
  state.session = { actor: { id: "voice-owner" } };
  state.generatedID = "47b69106-3c3e-44ca-a50a-a89108d1c6c1";
  let view = render(<Page />);
  await screen.findByText("image 富文本编辑器");
  await tab("音频生成");
  fireEvent.click(await screen.findByRole("button", { name: "选择验证项目" }));
  await screen.findByText("audio 富文本编辑器");
  await tab("视频生成");
  await screen.findByText("video 富文本编辑器");
  await tab("音频生成");
  await screen.findByText("audio 富文本编辑器");
  expect(state.opened.at(-1)?.draftID).toBe(state.generatedID);
  view.unmount();
  view = render(<Page />);
  await screen.findByText("image 富文本编辑器");
  await tab("音频生成");
  fireEvent.click(await screen.findByRole("button", { name: "选择验证项目" }));
  await screen.findByText("audio 富文本编辑器");
  expect(state.opened.at(-1)?.draftID).toBeUndefined();
  view.unmount();
  state.session = { actor: { id: "other-owner" } };
  render(<Page />);
  await screen.findByText("image 富文本编辑器");
  await tab("音频生成");
  fireEvent.click(await screen.findByRole("button", { name: "选择验证项目" }));
  await screen.findByText("audio 富文本编辑器");
  expect(state.opened.at(-1)?.draftID).toBeUndefined();
});

it("keeps a guest Agent draft through login and first project selection", async () => {
  const view = render(
    <ConversationProvider>
      <Page />
    </ConversationProvider>,
  );
  await tab("Agent 模式");
  fireEvent.paste(await screen.findByLabelText("对话创作输入"), {
    clipboardData: { files: [], getData: () => "访客尚未发送的正文" },
  });
  state.session = { actor: { id: "signed-in" } };
  view.rerender(
    <ConversationProvider>
      <Page />
    </ConversationProvider>,
  );
  await screen.findByText("image 富文本编辑器");
  await tab("Agent 模式");
  expect((await screen.findByLabelText("对话创作输入")).textContent).toBe(
    "访客尚未发送的正文",
  );
  fireEvent.click(await screen.findByRole("button", { name: "选择验证项目" }));
  expect((await screen.findByLabelText("对话创作输入")).textContent).toBe(
    "访客尚未发送的正文",
  );
});

it("keeps Agent drafts separate when switching projects", async () => {
  state.session = { actor: { id: "draft-owner" } };
  render(
    <ConversationProvider>
      <Page />
    </ConversationProvider>,
  );
  await screen.findByText("image 富文本编辑器");
  await tab("Agent 模式");
  fireEvent.click(await screen.findByRole("button", { name: "选择验证项目" }));
  fireEvent.paste(await screen.findByLabelText("对话创作输入"), {
    clipboardData: { files: [], getData: () => "项目A草稿" },
  });
  state.projectID = "second-project";
  fireEvent.click(await screen.findByRole("button", { name: "选择验证项目" }));
  await waitFor(() =>
    expect(screen.getByLabelText("对话创作输入").textContent).toBe(""),
  );
  fireEvent.paste(screen.getByLabelText("对话创作输入"), {
    clipboardData: { files: [], getData: () => "项目B草稿" },
  });
  state.projectID = "test-project";
  fireEvent.click(await screen.findByRole("button", { name: "选择验证项目" }));
  await waitFor(() =>
    expect(screen.getByLabelText("对话创作输入").textContent).toBe("项目A草稿"),
  );
});
