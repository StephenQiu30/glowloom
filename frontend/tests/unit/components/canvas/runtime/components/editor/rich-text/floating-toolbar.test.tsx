import { afterEach, expect, it, vi } from "vitest";
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
