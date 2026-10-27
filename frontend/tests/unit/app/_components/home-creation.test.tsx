import { useEffect, useState } from "react";
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
vi.mock("next/dynamic", () => ({ default: () => WorkspaceStub }));
vi.mock("@/components/projects/project-picker", () => ({
  ProjectPicker: ({
    onChange,
    disabled,
  }: {
    onChange: (p: { id: string; name: string }) => void;
    disabled: boolean;
  }) => (
    <Button
      disabled={disabled}
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
    props.onDraftReady?.(props.draftID ?? `${props.seed.capability}-draft`);
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
  const [prompt, setPrompt] = useState("");
  return <HomeCreation prompt={prompt} onPromptChange={setPrompt} />;
}
beforeEach(() => {
  state.session = null;
  state.opened = [];
  state.save.mockReset().mockResolvedValue(true);
  state.login.mockReset();
});
afterEach(cleanup);
async function tab(name: string) {
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
  expect(screen.getByLabelText("创作提示词")).toHaveProperty(
    "textContent",
    "保留这段想法",
  );
  fireEvent.click(screen.getByRole("button", { name: "开始生成" }));
  expect(state.login).toHaveBeenCalledOnce();
  expect(screen.queryByLabelText("推荐提示技能")).toBeNull();
  expect(state.opened).toEqual([]);
});

it("未接入类型直接解释未开放状态，不要求先登录或提交生成", async () => {
  render(<Page />);
  await tab("音乐生成");
  fireEvent.click(
    await screen.findByRole("button", { name: "音乐生成暂未开放" }),
  );
  expect(await screen.findByRole("dialog")).toBeTruthy();
  expect(state.login).not.toHaveBeenCalled();
  expect(state.opened).toEqual([]);
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
    expect(screen.getByLabelText("画布创作想法").textContent).toBe(
      "一片安静的海",
    ),
  );
  fireEvent.paste(screen.getByLabelText("画布创作想法"), {
    clipboardData: { files: [], getData: () => "，傍晚" },
  });
  const text = screen.getByLabelText("画布创作想法").textContent;
  fireEvent.click(screen.getByRole("radio", { name: "生成" }));
  await waitFor(() =>
    expect(screen.getByLabelText("创作提示词").textContent).toBe(text),
  );
  expect(state.login).not.toHaveBeenCalled();
  expect(state.opened).toEqual([]);
});
