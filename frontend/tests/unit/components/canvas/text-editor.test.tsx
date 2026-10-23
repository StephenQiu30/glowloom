import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ComponentProps } from "react";
import type { ComposerSession } from "@/components/generation/composer-session";
import type { PromptEditor as PromptEditorType } from "@/components/generation/prompt-editor";
import { CanvasTextEditor } from "@/components/canvas/text-editor";
import {
  createNode,
  emptyDocument,
  parseDocument,
} from "@/components/canvas/document";
import type { CanvasNodeData } from "@/components/canvas/types";
const state = vi.hoisted(() => ({ sessions: [] as ComposerSession[] }));
vi.mock("@/components/generation/prompt-editor", async (importOriginal) => {
  const { PromptEditor } = await importOriginal<{
    PromptEditor: typeof PromptEditorType;
  }>();
  return {
    PromptEditor: (props: ComponentProps<typeof PromptEditorType>) => (
      <PromptEditor
        {...props}
        onReady={(session) => {
          state.sessions.push(session);
          props.onReady(session);
        }}
      />
    ),
  };
});
afterEach(() => {
  cleanup();
  state.sessions = [];
});
it("展开保留会话和选区，失败保留引用，保存重开与文件导入保持稳定身份", async () => {
  const node = createNode("text", { x: 0, y: 0 });
  const saved: CanvasNodeData[] = [];
  const onClose = vi.fn();
  const onSave = vi.fn((n: CanvasNodeData) => {
    saved.push(structuredClone(n));
    return Promise.resolve(saved.length > 1);
  });
  const anchorRef = { current: document.createElement("div") };
  const view = render(
    <CanvasTextEditor
      node={node}
      anchorRef={anchorRef}
      onSave={onSave}
      onClose={onClose}
    />,
  );
  await waitFor(() => expect(state.sessions).toHaveLength(1));
  const session = state.sessions[0];
  act(() => {
    session.editor.commands.insertContent("中文😀参考");
    session.insertReference({
      id: "reference-one",
      source: {
        kind: "pending_upload",
        clientUploadId: "u1",
        uploadAttemptId: "attempt1",
      },
      bindingGeneration: 1,
      mediaKind: "image",
      role: "reference_image",
      order: 0,
    });
    session.insertReference(session.snapshot().draft.references[0]);
  });
  const selection = session.editor.state.selection.toJSON();
  fireEvent.click(screen.getByRole("button", { name: "展开编辑器" }));
  expect(state.sessions).toHaveLength(1);
  expect(session.editor.state.selection.toJSON()).toEqual(selection);
  expect(session.snapshot().draft.references).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "收起编辑器" }));
  expect(state.sessions).toHaveLength(1);
  expect(session.editor.state.selection.toJSON()).toEqual(selection);
  expect(
    session.editor.view.dom.querySelectorAll("[data-prompt-reference]"),
  ).toHaveLength(2);
  fireEvent.click(screen.getByRole("button", { name: "保存文本" }));
  await screen.findByRole("alert");
  expect(onClose).not.toHaveBeenCalled();
  expect(session.snapshot().draft).toEqual(saved[0].metadata!.composer);
  fireEvent.click(screen.getByRole("button", { name: "保存文本" }));
  await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
  const exported = parseDocument(
    JSON.stringify({ ...emptyDocument, nodes: [saved[1]] }),
  );
  expect(exported.nodes[0].metadata?.composer).toEqual(
    saved[0].metadata?.composer,
  );
  view.unmount();
  render(
    <CanvasTextEditor
      node={exported.nodes[0]}
      onSave={onSave}
      onClose={onClose}
    />,
  );
  await waitFor(() => expect(state.sessions).toHaveLength(2));
  expect(state.sessions[1].snapshot().draft).toEqual(
    saved[0].metadata?.composer,
  );
  expect(
    state.sessions[1].editor.view.dom.querySelectorAll(
      "[data-prompt-reference]",
    ),
  ).toHaveLength(2);
});
it("输入法组合期间保存和 Escape 不会清空或提交正文", async () => {
  const onSave = vi.fn(),
    onClose = vi.fn();
  render(
    <CanvasTextEditor
      node={createNode("text", { x: 0, y: 0 })}
      onSave={onSave}
      onClose={onClose}
    />,
  );
  await waitFor(() => expect(state.sessions).toHaveLength(1));
  const input = screen.getByRole("textbox", { name: "文本内容" });
  fireEvent.compositionStart(input);
  fireEvent.keyDown(input, { key: "Escape", isComposing: true, keyCode: 229 });
  fireEvent.click(screen.getByRole("button", { name: "保存文本" }));
  expect(onSave).not.toHaveBeenCalled();
  expect(onClose).not.toHaveBeenCalled();
  fireEvent.compositionEnd(input);
});

it("等待保存回执时锁住正文，拒绝后恢复同一会话", async () => {
  let resolve!: (value: boolean) => void;
  const onSave = vi.fn(
    () =>
      new Promise<boolean>((done) => {
        resolve = done;
      }),
  );
  render(
    <CanvasTextEditor
      node={createNode("text", { x: 0, y: 0 })}
      onSave={onSave}
      onClose={vi.fn()}
    />,
  );
  await waitFor(() => expect(state.sessions).toHaveLength(1));
  const input = screen.getByRole("textbox", { name: "文本内容" });
  fireEvent.click(screen.getByRole("button", { name: "保存文本" }));
  await waitFor(() =>
    expect(input.getAttribute("contenteditable")).toBe("false"),
  );
  await act(async () => resolve(false));
  await waitFor(() =>
    expect(input.getAttribute("contenteditable")).toBe("true"),
  );
  expect(state.sessions).toHaveLength(1);
});
