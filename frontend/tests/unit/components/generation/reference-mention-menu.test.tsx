import { Editor } from "@tiptap/core";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  ComposerSession,
  composerEditorOptions,
} from "@/components/generation/composer-session";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import { ReferenceControls } from "@/components/generation/reference-controls";

const editors: Editor[] = [];
afterEach(() => {
  cleanup();
  editors.splice(0).forEach((editor) => editor.destroy());
  vi.restoreAllMocks();
});

it("对话补充TXT保留真实文档类型、角色和待上传身份，不误判为text", () => {
  const editor = new Editor(
    composerEditorOptions({
      contextKey: "conversation/document",
      replicaId: "test",
      draft: emptyPromptDraft(),
    }),
  );
  editors.push(editor);
  const session = new ComposerSession(editor);
  render(
    <ReferenceControls
      session={session}
      draft={session.snapshot().draft}
      projectID="project"
      compact
      home
      allowDocuments
    />,
  );
  const input = screen.getByLabelText("上传参考文件");
  expect(input.getAttribute("accept")).toContain(".txt,.docx");
  fireEvent.change(input, {
    target: {
      files: [new File(["完整小说正文"], "novel.txt", { type: "text/plain" })],
    },
  });
  expect(session.snapshot().draft.references).toEqual([
    expect.objectContaining({
      mediaKind: "document",
      role: "reference_document",
      source: expect.objectContaining({ kind: "pending_upload" }),
    }),
  ]);
});

it("普通生成入口继续拒绝文档，不因对话能力扩展而放宽", () => {
  const editor = new Editor(
    composerEditorOptions({
      contextKey: "generation/no-document",
      replicaId: "test",
      draft: emptyPromptDraft(),
    }),
  );
  editors.push(editor);
  const session = new ComposerSession(editor);
  render(
    <ReferenceControls
      session={session}
      draft={session.snapshot().draft}
      projectID="project"
      compact
    />,
  );
  const input = screen.getByLabelText("上传参考文件");
  expect(input.getAttribute("accept")).not.toContain(".txt");
  fireEvent.change(input, {
    target: {
      files: [new File(["正文"], "novel.txt", { type: "text/plain" })],
    },
  });
  expect(session.snapshot().draft.references).toHaveLength(0);
});

it("Agent紧凑参考菜单提供小说文档而不显示未接入的音频", async () => {
  const editor = new Editor(
    composerEditorOptions({
      contextKey: "assistant/doc-menu",
      replicaId: "test",
      draft: emptyPromptDraft(),
    }),
  );
  editors.push(editor);
  const session = new ComposerSession(editor);
  render(
    <ReferenceControls
      session={session}
      draft={session.snapshot().draft}
      projectID="project"
      compact
      assistant
    />,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "从本地、画布或资产库添加" }),
  );
  fireEvent.click(
    await screen.findByRole("option", { name: "从参考素材添加" }),
  );
  expect(await screen.findByRole("option", { name: "小说文档" })).toBeTruthy();
  expect(screen.queryByRole("option", { name: "音频" })).toBeNull();
});

it("窄屏鼠标经过分类不重排，点击仅展开候选并保留@正文", async () => {
  const media = window.matchMedia("(max-width: 639px)");
  vi.spyOn(window, "matchMedia").mockReturnValue({ ...media, matches: true });
  const editor = new Editor(
    composerEditorOptions({
      contextKey: "mention/narrow",
      replicaId: "test",
      draft: emptyPromptDraft(),
    }),
  );
  editors.push(editor);
  const session = new ComposerSession(editor);
  render(
    <ReferenceControls
      session={session}
      draft={session.snapshot().draft}
      projectID="project"
      compact
    />,
  );
  act(() => {
    editor.commands.insertContent("@");
  });
  const image = await screen.findByRole("option", { name: "图片" });
  fireEvent.pointerMove(image, { pointerType: "mouse" });
  expect(image.getAttribute("aria-expanded")).toBe("false");
  fireEvent.click(image);
  expect(await screen.findByText("暂无相关节点")).toBeTruthy();
  expect(screen.getByRole("option", { name: "主体" })).toBeTruthy();
  expect(editor.getText()).toBe("@");
});

it("悬停分类展开独立空态子菜单，一级菜单保持可见", async () => {
  const editor = new Editor(
    composerEditorOptions({
      contextKey: "mention/empty",
      replicaId: "test",
      draft: emptyPromptDraft(),
    }),
  );
  editors.push(editor);
  const session = new ComposerSession(editor);
  render(
    <ReferenceControls
      session={session}
      draft={session.snapshot().draft}
      projectID="project"
      compact
    />,
  );
  act(() => {
    editor.commands.insertContent("@");
  });
  fireEvent.pointerMove(await screen.findByRole("option", { name: "主体" }), {
    pointerType: "mouse",
  });
  expect(await screen.findByText("暂无相关节点")).toBeTruthy();
  expect(
    screen.getByRole("option", { name: "主体" }).getAttribute("aria-expanded"),
  ).toBe("true");
  expect(screen.getByRole("option", { name: "图片" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "返回" })).toBeNull();
  fireEvent.keyDown(editor.view.dom, { key: "ArrowLeft" });
  expect(await screen.findByRole("option", { name: "图片" })).toBeTruthy();
  expect(editor.getText()).toBe("@");
});

it("输入@后方向键先选分类再选参考，Enter替换触发文本且只插入一次", async () => {
  const editor = new Editor(
    composerEditorOptions({
      contextKey: "mention/keyboard",
      replicaId: "test",
      draft: emptyPromptDraft(),
    }),
  );
  editors.push(editor);
  document.body.appendChild(editor.view.dom);
  const session = new ComposerSession(editor);
  session.insertReference(
    {
      id: "reference-image",
      bindingGeneration: 1,
      order: 0,
      mediaKind: "image",
      role: "reference_image",
      source: { kind: "media", sourceId: "asset", sourceVersion: "1" },
    },
    false,
  );
  render(
    <ReferenceControls
      session={session}
      draft={session.snapshot().draft}
      projectID="project"
      compact
    />,
  );
  act(() => {
    editor.commands.insertContent("保留正文 @");
  });
  await screen.findByRole("option", { name: "图片" });
  fireEvent.keyDown(editor.view.dom, { key: "ArrowDown" });
  fireEvent.keyDown(editor.view.dom, { key: "Enter" });
  await screen.findByRole("option", { name: "图片1" });
  fireEvent.keyDown(editor.view.dom, { key: "ArrowDown" });
  fireEvent.keyDown(editor.view.dom, { key: "Enter" });
  await waitFor(() => expect(editor.getText()).toBe("保留正文 图片1"));
  expect(
    editor.view.dom.querySelectorAll("[data-prompt-reference]"),
  ).toHaveLength(1);
  expect(session.snapshot().draft.references).toHaveLength(1);
  expect(screen.queryByRole("dialog", { name: "引用候选" })).toBeNull();
  editor.view.dom.remove();
});
