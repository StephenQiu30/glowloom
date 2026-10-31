import { afterEach, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import type { Editor } from "@tiptap/core";
import { RichTextEditor } from "@/components/canvas/runtime/components/editor/rich-text/rich-text-editor";
import { documentText } from "@/components/canvas/runtime/components/editor/lib/rich-text-document";
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
it("rejects an oversized paste before applying it and retains the prior editable text", async () => {
  const change = vi.fn();
  render(
    <RichTextEditor
      value="已有输入"
      documentMode
      onChange={change}
      acceptDocument={(doc) => documentText(doc.content).length <= 12}
    />,
  );
  await waitFor(() => expect(state.editor).not.toBeNull());
  const editor = state.editor!;
  act(() => {
    editor.commands.insertContent("超大粘贴内容".repeat(100));
  });
  expect(editor.getText()).toBe("已有输入");
  expect(change).not.toHaveBeenCalled();
  act(() => {
    editor.commands.insertContent("新输入");
  });
  expect(editor.getText()).toContain("新输入");
  expect(change).toHaveBeenCalledOnce();
});

it("continues to apply authoritative content while local editing is disabled", async () => {
  const change = vi.fn();
  const view = render(
    <RichTextEditor
      value="旧内容"
      documentMode
      disabled
      onChange={change}
      acceptDocument={() => false}
    />,
  );
  await waitFor(() => expect(state.editor).not.toBeNull());
  view.rerender(
    <RichTextEditor
      value="服务器新内容"
      documentMode
      disabled
      onChange={change}
      acceptDocument={() => false}
    />,
  );
  expect(state.editor!.getText()).toBe("服务器新内容");
  expect(change).not.toHaveBeenCalled();
});

it("对齐改变通过原正文回调保存，撤销与重开保留格式", async () => {
  const change = vi.fn();
  const view = render(
    <RichTextEditor value="对齐正文" documentMode onChange={change} />,
  );
  await waitFor(() => expect(state.editor).not.toBeNull());
  expect(screen.getByRole("button", { name: "段落对齐" })).not.toBeNull();
  const editor = state.editor!;
  act(() => {
    editor.commands.setTextAlign("center");
  });
  expect(editor.getJSON().content?.[0].attrs?.textAlign).toBe("center");
  const saved = change.mock.calls.at(-1)![1];
  expect(saved.content.content[0].attrs.textAlign).toBe("center");
  act(() => {
    editor.commands.undo();
  });
  expect(editor.getJSON().content?.[0].attrs?.textAlign).toBeNull();
  act(() => {
    editor.commands.redo();
  });
  expect(editor.getJSON().content?.[0].attrs?.textAlign).toBe("center");
  view.unmount();
  render(
    <RichTextEditor
      value="对齐正文"
      document={saved}
      documentMode
      disabled
      onChange={change}
    />,
  );
  await waitFor(() => expect(state.editor?.isDestroyed).toBe(false));
  expect(state.editor!.getJSON().content?.[0].attrs?.textAlign).toBe("center");
  expect(
    screen.getByRole("button", { name: "段落对齐" }).hasAttribute("disabled"),
  ).toBe(true);
});
