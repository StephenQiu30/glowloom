import { useRef, useState } from "react";
import { afterEach, beforeAll, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import type { Editor } from "@tiptap/core";
import { RichTextEditor } from "@/components/canvas/runtime/components/editor/rich-text/rich-text-editor";

// Tiptap scrolls its selection; JSDOM has no text-range layout.
beforeAll(() => {
  Range.prototype.getClientRects = () => [] as unknown as DOMRectList;
  Range.prototype.getBoundingClientRect = () => new DOMRect();
});
const state = vi.hoisted(() => ({ editor: null as Editor | null }));
vi.mock("@tiptap/react", async (original) => {
  const tiptap = await original<typeof import("@tiptap/react")>();
  return {
    ...tiptap,
    useEditor: (...args: Parameters<typeof tiptap.useEditor>) => {
      const editor = tiptap.useEditor(...args);
      state.editor = editor;
      return editor;
    },
  };
});
afterEach(() => {
  cleanup();
  state.editor = null;
});

it("formats the selected text without bubbling a portal click into canvas node actions", async () => {
  const nodeAction = vi.fn();
  const change = vi.fn();
  render(
    <div onMouseDown={nodeAction} onClick={nodeAction}>
      <RichTextEditor
        value="保留选区"
        documentMode
        floatingToolbar
        onChange={change}
      />
    </div>,
  );
  await waitFor(() => expect(state.editor).not.toBeNull());
  act(() => {
    state.editor!.commands.setTextSelection({ from: 1, to: 5 });
  });
  // JSDOM has no layout; Radix treats its zero-sized anchor as detached.
  const bold = await screen.findByTitle("加粗");
  fireEvent.mouseDown(bold);
  fireEvent.click(bold);
  expect(nodeAction).not.toHaveBeenCalled();
  expect(
    state.editor!.getJSON().content?.[0].content?.[0].marks,
  ).toContainEqual({ type: "bold" });
  expect(change).toHaveBeenCalled();
  expect(state.editor!.getText()).toBe("保留选区");
});

it("offers body and three heading levels without losing the selected text", async () => {
  render(
    <RichTextEditor
      value="章节内容"
      documentMode
      floatingToolbar
      onChange={vi.fn()}
    />,
  );
  await waitFor(() => expect(state.editor).not.toBeNull());
  act(() => {
    state.editor!.commands.setTextSelection({ from: 1, to: 5 });
  });
  fireEvent.keyDown(await screen.findByTitle("文本样式"), { key: "ArrowDown" });
  fireEvent.click(screen.getByRole("menuitemradio", { name: "标题 1" }));
  expect(state.editor!.getJSON().content?.[0]).toMatchObject({
    type: "heading",
    attrs: { level: 1 },
  });
  expect(state.editor!.state.selection.from).toBe(1);
  expect(state.editor!.state.selection.to).toBe(5);
  fireEvent.keyDown(screen.getByTitle("文本样式"), { key: "ArrowDown" });
  fireEvent.click(screen.getByRole("menuitemradio", { name: "标题 3" }));
  expect(state.editor!.getJSON().content?.[0]).toMatchObject({
    type: "heading",
    attrs: { level: 3 },
  });
  fireEvent.keyDown(screen.getByTitle("文本样式"), { key: "ArrowDown" });
  fireEvent.click(screen.getByRole("menuitemradio", { name: "正文" }));
  expect(state.editor!.getJSON().content?.[0].type).toBe("paragraph");
  expect(state.editor!.getText().trim()).toBe("章节内容");
});

it("keeps one document editor and its undo history across full screen and close", async () => {
  render(
    <RichTextEditor
      value="保留正文"
      documentMode
      floatingToolbar
      onChange={vi.fn()}
    />,
  );
  await waitFor(() => expect(state.editor).not.toBeNull());
  const original = state.editor!;
  act(() => {
    original.commands.setTextSelection({ from: 1, to: 5 });
    original.commands.setHeading({ level: 2 });
  });
  fireEvent.click(await screen.findByTitle("展开文档编辑"));
  const dialog = await screen.findByRole("dialog", { name: "编辑文档" });
  expect(dialog.classList.contains("canvas-document-fullscreen")).toBe(true);
  expect(state.editor).toBe(original);
  expect(screen.getAllByRole("textbox")).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "关闭文档编辑" }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog", { name: "编辑文档" })).toBeNull(),
  );
  expect(state.editor).toBe(original);
  expect(original.getJSON().content?.[0]).toMatchObject({
    type: "heading",
    attrs: { level: 2 },
  });
  act(() => {
    original.commands.undo();
  });
  expect(original.getJSON().content?.[0].type).toBe("paragraph");
});

it("preserves prompt references and undo when expanding, rejects IME escape and restores trigger focus", async () => {
  const change = vi.fn();
  function Prompt() {
    const [open, setOpen] = useState(false);
    const trigger = useRef<HTMLButtonElement>(null);
    return (
      <>
        <button ref={trigger} onClick={() => setOpen(true)}>
          放大编辑
        </button>
        <RichTextEditor
          value="提示词"
          onChange={change}
          expandedView={{
            open,
            onOpenChange: setOpen,
            onCloseAutoFocus: () =>
              trigger.current?.focus({ preventScroll: true }),
            header: <span>参考素材</span>,
            footer: <button>生成</button>,
          }}
        />
      </>
    );
  }
  render(<Prompt />);
  await waitFor(() => expect(state.editor).not.toBeNull());
  const original = state.editor!;
  act(() => {
    original.commands.insertContent({
      type: "reference",
      attrs: { referenceId: "asset-stable-id" },
    });
  });
  const before = original.getJSON();
  fireEvent.click(screen.getByRole("button", { name: "放大编辑" }));
  const dialog = await screen.findByRole("dialog", { name: "编辑提示词" });
  expect(state.editor).toBe(original);
  expect(screen.getAllByRole("textbox")).toHaveLength(1);
  expect(original.getJSON()).toEqual(before);
  fireEvent.keyDown(screen.getByRole("textbox"), {
    key: "Escape",
    isComposing: true,
  });
  expect(dialog.isConnected).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "收起编辑" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "放大编辑" }),
    ),
  );
  expect(original.getJSON()).toEqual(before);
  act(() => {
    original.commands.undo();
  });
  expect(
    original
      .getJSON()
      .content?.[0].content?.some((node) => node.type === "reference"),
  ).toBe(false);
});

it("retains undo when a save receipt reorders equivalent document fields", async () => {
  const change = vi.fn();
  const view = render(
    <RichTextEditor value="原文" documentMode onChange={change} />,
  );
  await waitFor(() => expect(state.editor).not.toBeNull());
  const editor = state.editor!;
  act(() => {
    editor.commands.setHeading({ level: 1 });
  });
  const [value, document] = change.mock.calls.at(-1)!;
  // JSON object order differs across the Go receipt and Tiptap serialization.
  const reorder = (value: unknown): unknown =>
    Array.isArray(value)
      ? value.map(reorder)
      : value && typeof value === "object"
        ? Object.fromEntries(
            Object.entries(value)
              .reverse()
              .map(([key, item]) => [key, reorder(item)]),
          )
        : value;
  view.rerender(
    <RichTextEditor
      value={value}
      document={reorder(document) as typeof document}
      documentMode
      onChange={change}
    />,
  );
  expect(editor.can().undo()).toBe(true);
  act(() => {
    editor.commands.undo();
  });
  expect(editor.getJSON().content?.[0].type).toBe("paragraph");
});

it("紧凑画布面板隐藏格式栏但保留标题下划线，展开复用原编辑器", async () => {
  function Compact() {
    const [open, setOpen] = useState(false);
    return (
      <>
        <button onClick={() => setOpen(true)}>展开验收</button>
        <RichTextEditor
          value="标题"
          documentMode
          compactToolbar
          onChange={vi.fn()}
          expandedView={{ open, onOpenChange: setOpen }}
          document={{
            schemaVersion: 1,
            content: {
              type: "doc",
              content: [
                {
                  type: "heading",
                  attrs: { level: 2 },
                  content: [
                    {
                      type: "text",
                      text: "标题",
                      marks: [{ type: "underline" }],
                    },
                  ],
                },
              ],
            },
          }}
        />
      </>
    );
  }
  render(<Compact />);
  await waitFor(() => expect(state.editor).not.toBeNull());
  const original = state.editor;
  expect(screen.queryByRole("toolbar", { name: "文档格式" })).toBeNull();
  expect(
    state.editor!.getJSON().content?.[0].content?.[0].marks,
  ).toContainEqual({ type: "underline" });
  fireEvent.click(screen.getByRole("button", { name: "展开验收" }));
  expect(await screen.findByRole("toolbar", { name: "文档格式" })).toBeTruthy();
  expect(state.editor).toBe(original);
  expect(screen.getAllByRole("textbox")).toHaveLength(1);
});

it("文本样式菜单集中导入导出，同时保留格式编辑", async () => {
  render(
    <RichTextEditor
      value="菜单验收"
      documentMode
      floatingToolbar
      onChange={vi.fn()}
    />,
  );
  await waitFor(() => expect(state.editor).not.toBeNull());
  expect(screen.queryByTitle("更多文档操作")).toBeNull();
  fireEvent.keyDown(await screen.findByTitle("文本样式"), { key: "ArrowDown" });
  expect(screen.getByRole("menuitem", { name: "导入结构化文档" })).toBeTruthy();
  expect(screen.getByRole("menuitem", { name: "导出结构化文档" })).toBeTruthy();
  fireEvent.click(screen.getByRole("menuitemradio", { name: "标题 2" }));
  expect(state.editor!.getJSON().content?.[0]).toMatchObject({
    type: "heading",
    attrs: { level: 2 },
  });
});
