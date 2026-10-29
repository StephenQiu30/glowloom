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
import { HomeCreation } from "@/app/_components/home-creation";
import type { RichGeneration } from "@/components/generation/rich-generation";

const state = vi.hoisted(() => ({
  session: null as { actor: { id: string } } | null,
  save: vi.fn<() => Promise<boolean>>(),
  opened: [] as { capability: string; draftID?: string }[],
  login: vi.fn(),
  generatedID: undefined as string | undefined,
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => state.session,
}));
vi.mock("@/components/layout/basic-layout", () => ({
  useLoginDialog: () => state.login,
}));
vi.mock("@/components/projects/new-canvas-button", () => ({
  NewCanvasButton: () => <Button>新建画布</Button>,
}));
vi.mock("next/dynamic", () => ({
  default: (loader: () => unknown) =>
    loader.toString().includes("agent-workspace")
      ? () => <p>Agent 会话</p>
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
      onClick={() => onChange({ id: "test-project", name: "验证项目" })}
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
  state.generatedID = undefined;
  localStorage.clear();
  state.opened = [];
  state.save.mockReset().mockResolvedValue(true);
  state.login.mockReset();
});
afterEach(cleanup);

it("未选项目时模型入口打开模型说明，仅显式选择项目才打开项目弹层", async () => {
  state.session = { actor: { id: "test-actor" } };
  render(<Page />);
  fireEvent.click(screen.getByRole("button", { name: /选择模型|生成模型/ }));
  expect(await screen.findByText("选择生成模型")).toBeTruthy();
  expect(
    screen
      .getByRole("button", { name: "选择验证项目" })
      .getAttribute("aria-expanded"),
  ).toBe("false");
  fireEvent.click(screen.getByRole("button", { name: "选择项目" }));
  await waitFor(() =>
    expect(
      screen
        .getByRole("button", { name: "选择验证项目" })
        .getAttribute("aria-expanded"),
    ).toBe("true"),
  );
  expect(screen.queryByText("选择生成模型")).toBeNull();
  expect(state.opened).toEqual([]);
});

async function tab(name: string) {
  fireEvent.keyDown(screen.getByRole("combobox", { name: "创作类型" }), {
    key: "ArrowDown",
  });
  fireEvent.click(
    await screen.findByRole("option", { name: new RegExp(name) }),
  );
}

it.each([
  ["参数", "生成参数"],
  ["添加参考素材", "添加参考素材"],
])(
  "未选项目时%s只打开对应内容，显式选择项目后才打开项目弹层",
  async (trigger, title) => {
    state.session = { actor: { id: "test-actor" } };
    render(<Page />);
    fireEvent.click(screen.getByRole("button", { name: trigger }));
    expect(await screen.findByText(title)).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: "选择验证项目" })
        .getAttribute("aria-expanded"),
    ).toBe("false");
    fireEvent.click(screen.getByRole("button", { name: "选择项目" }));
    await waitFor(() =>
      expect(
        screen
          .getByRole("button", { name: "选择验证项目" })
          .getAttribute("aria-expanded"),
      ).toBe("true"),
    );
    expect(state.opened).toEqual([]);
  },
);

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
  fireEvent.paste(screen.getByLabelText("创作提示词"), {
    clipboardData: { files: [], getData: () => "海边海报" },
  });
  fireEvent.click(screen.getByRole("button", { name: "开始生成" }));
  expect(state.login).toHaveBeenCalledOnce();
});

it("保存未确认禁止切换，确认后按类型恢复原草稿指针", async () => {
  state.session = { actor: { id: "test-actor" } };
  render(<Page />);
  fireEvent.click(screen.getByRole("button", { name: "选择验证项目" }));
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

it("报价冻结期间不能切换类型、项目或工作模式", async () => {
  state.session = { actor: { id: "test-actor" } };
  render(<Page />);
  fireEvent.click(screen.getByRole("button", { name: "选择验证项目" }));
  fireEvent.click(await screen.findByRole("button", { name: "冻结报价" }));
  expect(screen.getByRole("combobox", { name: "创作类型" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(screen.getByRole("button", { name: "选择验证项目" })).toHaveProperty(
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
    const editor = await screen.findByLabelText("创作提示词");
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
    expect(screen.getByLabelText("创作提示词").textContent).toBe(
      name + "独立内容",
    );
  }
});

it("账号切换立即隔离所有类型文字和项目指针", async () => {
  state.session = { actor: { id: "account-a" } };
  const page = render(<Page />);
  fireEvent.paste(await screen.findByLabelText("创作提示词"), {
    clipboardData: { files: [], getData: () => "A的私有内容" },
  });
  state.session = { actor: { id: "account-b" } };
  page.rerender(<Page />);
  expect(screen.getByLabelText("创作提示词").textContent).toBe("");
  fireEvent.click(screen.getByRole("button", { name: "选择验证项目" }));
  await screen.findByText("image 富文本编辑器");
  expect(state.opened.at(-1)?.draftID).toBeUndefined();
});

it("音频独立进入 audio 工作区且未选项目时没有素材引用入口", async () => {
  state.session = { actor: { id: "test-actor" } };
  render(<Page />);
  await tab("音频生成");
  expect(screen.queryByRole("button", { name: "添加参考素材" })).toBeNull();
  expect(
    screen.queryByRole("button", { name: "添加主体或素材引用" }),
  ).toBeNull();
  expect(screen.getByText("文本朗读")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "选择验证项目" }));
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
  await tab("动作模仿");
  expect(screen.queryByRole("textbox", { name: "创作提示词" })).toBeNull();
  expect(screen.getByText(/选择项目后添加角色图片与动作视频/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "选择验证项目" }));
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

it("项目音频草稿刷新恢复原ID，并按账号隔离", async () => {
  state.session = { actor: { id: "voice-owner" } };
  state.generatedID = "47b69106-3c3e-44ca-a50a-a89108d1c6c1";
  let view = render(<Page />);
  await tab("音频生成");
  fireEvent.click(screen.getByRole("button", { name: "选择验证项目" }));
  await screen.findByText("audio 富文本编辑器");
  view.unmount();
  view = render(<Page />);
  await tab("音频生成");
  fireEvent.click(screen.getByRole("button", { name: "选择验证项目" }));
  await screen.findByText("audio 富文本编辑器");
  expect(state.opened.at(-1)?.draftID).toBe(state.generatedID);
  view.unmount();
  state.session = { actor: { id: "other-owner" } };
  render(<Page />);
  await tab("音频生成");
  fireEvent.click(screen.getByRole("button", { name: "选择验证项目" }));
  await screen.findByText("audio 富文本编辑器");
  expect(state.opened.at(-1)?.draftID).toBeUndefined();
});
