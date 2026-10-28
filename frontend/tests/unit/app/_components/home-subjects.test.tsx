import { useState } from "react";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { HomeCreation } from "@/app/_components/home-creation";
import type { GenerationSeed } from "@/components/generation/generation-dialog";

const state = vi.hoisted(() => ({
  actor: "11111111-1111-4111-8111-111111111111",
  scope: "22222222-2222-4222-8222-222222222222",
  list: vi.fn(),
  validate: vi.fn(),
  seed: undefined as GenerationSeed | undefined,
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => ({ actor: { id: state.actor } }),
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({ selected: { id: state.scope } }),
}));
vi.mock("@/components/layout/basic-layout", () => ({
  useLoginDialog: () => vi.fn(),
}));
vi.mock("@/api/subjects", () => ({ listSubjects: state.list }));
vi.mock("@/api/promptEditor", () => ({
  validatePromptFragment: state.validate,
}));
vi.mock("@/components/subjects/unified-media", () => ({
  UnifiedSubjectMedia: () => <span>缩略图</span>,
}));
vi.mock("@/components/projects/project-picker", () => ({
  ProjectPicker: ({
    onChange,
  }: {
    onChange: (value: { id: string; name: string }) => void;
  }) => (
    <button onClick={() => onChange({ id: "project", name: "验证项目" })}>
      选择验证项目
    </button>
  ),
}));
vi.mock("next/dynamic", () => ({
  default: (loader: () => Promise<unknown>) =>
    loader.toString().includes("rich-generation")
      ? (props: { seed: GenerationSeed }) => {
          state.seed = props.seed;
          return <p>项目创作编辑器</p>;
        }
      : (props: { onClose?: () => void; onSaved?: () => void }) => (
          <button onClick={props.onClose}>取消设置</button>
        ),
}));
function Page() {
  const [prompt, setPrompt] = useState("");
  return <HomeCreation prompt={prompt} onPromptChange={setPrompt} />;
}
const subject = {
  id: "33333333-3333-4333-8333-333333333333",
  owner_scope_id: state.scope,
  version_id: "44444444-4444-4444-8444-444444444444",
  revision: 1,
  name: "几何主体",
  description: "合成几何图",
  primary: {
    binding_id: "55555555-5555-4555-8555-555555555555",
    asset_id: "66666666-6666-4666-8666-666666666666",
    asset_revision: 1,
    kind: "image",
    mime_type: "image/png",
    file_name: "geometry.png",
  },
  references: [],
  favorite: false,
  created_at: "2026-10-28T00:00:00Z",
  updated_at: "2026-10-28T00:00:00Z",
};
beforeEach(() => {
  state.scope = "22222222-2222-4222-8222-222222222222";
  state.seed = undefined;
  state.list.mockResolvedValue({
    current_actor_id: state.actor,
    current_scope_id: state.scope,
    items: [subject],
    page: 1,
    page_size: 24,
    total: 1,
    capabilities: { intelligent_voice: { available: false } },
  });
});

it("无项目的参考菜单保留主体入口，项目素材需显式选择项目", async () => {
  render(<Page />);
  fireEvent.click(await screen.findByRole("button", { name: "添加参考" }));
  expect(await screen.findByRole("option", { name: "添加主体" })).toBeTruthy();
  expect(
    screen.getByRole("option", { name: "选择项目后添加项目素材" }),
  ).toBeTruthy();
  expect(screen.queryByRole("option", { name: "上传参考内容" })).toBeNull();
  expect(screen.queryByRole("option", { name: "从参考素材添加" })).toBeNull();
  expect(state.validate).not.toHaveBeenCalled();
});

it("切换空间不会将原空间的未提交主体身份带入新项目", async () => {
  const page = render(<Page />);
  const trigger = await screen.findByRole("button", {
    name: "添加主体或素材引用",
  });
  await waitFor(() => expect(trigger).toHaveProperty("disabled", false));
  fireEvent.click(trigger);
  fireEvent.click(await screen.findByRole("option", { name: /几何主体/ }));
  await waitFor(() =>
    expect(
      screen.getByRole("textbox", { name: "创作提示词" }).textContent,
    ).toBe("主体1"),
  );
  state.scope = "77777777-7777-4777-8777-777777777777";
  page.rerender(<Page />);
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "添加参考" })).toBeTruthy(),
  );
  fireEvent.click(screen.getByRole("button", { name: "选择验证项目" }));
  await screen.findByText("项目创作编辑器");
  expect(state.seed?.composer?.references ?? []).toHaveLength(0);
  expect(JSON.stringify(state.seed)).not.toContain(subject.id);
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

it.each(["创建主体", "全部主体"])(
  "未选项目时@进入%s，取消返回正文和焦点",
  async (entry) => {
    render(<Page />);
    const editor = await screen.findByRole("textbox", { name: "创作提示词" });
    fireEvent.paste(editor, {
      clipboardData: { files: [], getData: () => "保留正文 @" },
    });
    fireEvent.click(await screen.findByRole("option", { name: entry }));
    fireEvent.click(
      await screen.findByRole("button", {
        name: entry === "创建主体" ? "取消设置" : "关闭对话框",
      }),
    );
    await waitFor(() => expect(document.activeElement).toBe(editor));
    expect(editor.textContent).toBe("保留正文 @");
    expect(state.validate).not.toHaveBeenCalled();
    expect(state.seed).toBeUndefined();
  },
);

it("未选项目工具栏可引用当前空间主体，选项目时完整绑定随草稿传入", async () => {
  render(<Page />);
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "添加主体或素材引用" }),
    ).toHaveProperty("disabled", false),
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "添加主体或素材引用" }),
  );
  fireEvent.click(await screen.findByRole("option", { name: /几何主体/ }));
  await waitFor(() =>
    expect(
      screen.getByRole("textbox", { name: "创作提示词" }).textContent,
    ).toBe("主体1"),
  );
  expect(state.seed).toBeUndefined();
  expect(state.validate).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "选择验证项目" }));
  await screen.findByText("项目创作编辑器");
  expect(state.seed?.composer?.references[0].source).toMatchObject({
    kind: "subject",
    sourceId: subject.id,
    sourceVersion: subject.version_id,
    media: [{ bindingId: subject.primary.binding_id }],
  });
});
