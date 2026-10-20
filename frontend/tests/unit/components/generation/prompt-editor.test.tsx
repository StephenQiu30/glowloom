import { cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { PromptEditor } from "@/components/generation/prompt-editor";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import { ComposerSession } from "@/components/generation/composer-session";

afterEach(cleanup);
it("呈现属性更新保留同一编辑会话、正文与可访问名称", async () => {
  const onReady = vi.fn<(s: ComposerSession) => void>();
  const props = {
    contextKey: "u/p/presentation",
    replicaId: "presentation",
    draft: emptyPromptDraft(),
    onReady,
  };
  const { rerender } = render(
    <PromptEditor
      {...props}
      placeholder="输入图片需求"
      className="min-h-16 outline-none"
      inputGroup
    />,
  );
  await waitFor(() => expect(onReady).toHaveBeenCalledOnce());
  const session = onReady.mock.calls[0][0];
  expect(session.editor.view.dom.getAttribute("data-placeholder")).toBe(
    "输入图片需求",
  );
  expect(session.editor.view.dom.getAttribute("data-slot")).toBe(
    "input-group-control",
  );
  expect(session.editor.view.dom.getAttribute("aria-label")).toBe("创作提示词");
  session.editor.commands.insertContent("已输入正文");
  rerender(
    <PromptEditor
      {...props}
      placeholder="输入视频需求"
      className="min-h-24 outline-none"
      inputGroup
    />,
  );
  await waitFor(() =>
    expect(session.editor.view.dom.getAttribute("data-placeholder")).toBe(
      "输入视频需求",
    ),
  );
  expect(onReady).toHaveBeenCalledOnce();
  expect(session.editor.getText()).toBe("已输入正文");
});
it("同对象重渲染不以 seed 重置正文、模式或历史；换对象销毁旧会话", async () => {
  const onReady = vi.fn<(s: ComposerSession) => void>();
  const onDispose = vi.fn();
  const props = {
    contextKey: "u/p/d1",
    replicaId: "instance-a",
    draft: emptyPromptDraft(),
    onReady,
    onDispose,
  };
  const { rerender, unmount } = render(<PromptEditor {...props} />);
  await waitFor(() => expect(onReady).toHaveBeenCalledTimes(1));
  const session = onReady.mock.calls[0][0];
  session.editor.commands.insertContent("保留正文");
  session.resolveModeSelection(
    session.beginModeSelection({ mode: "omni", modelVersion: "m1" }),
  );
  rerender(
    <PromptEditor {...props} draft={{ ...emptyPromptDraft(), mode: "text" }} />,
  );
  expect(onReady).toHaveBeenCalledTimes(1);
  expect(session.snapshot().draft.mode).toBe("omni");
  expect(session.editor.getText()).toBe("保留正文");
  const request = session.beginModeSelection({
    mode: "old-target",
    modelVersion: "m1",
  });
  rerender(
    <PromptEditor {...props} contextKey="u/p/d2" replicaId="instance-b" />,
  );
  await waitFor(() => expect(onReady).toHaveBeenCalledTimes(2));
  expect(session.editor.isDestroyed).toBe(true);
  expect(session.resolveModeSelection(request)).toBe(false);
  expect(onReady.mock.calls[1][0].snapshot().draft).toEqual(emptyPromptDraft());
  unmount();
  await waitFor(() => expect(onDispose).toHaveBeenCalledTimes(2));
});

it("同实例更新保存回调时使用新回调，不重建正文", async () => {
  const onReady = vi.fn<(s: ComposerSession) => void>();
  const oldSave = vi.fn(),
    newSave = vi.fn();
  const props = {
    contextKey: "u/p/d",
    replicaId: "callback-test",
    draft: emptyPromptDraft(),
    onReady,
  };
  const { rerender } = render(
    <PromptEditor {...props} onRequestSave={oldSave} />,
  );
  await waitFor(() => expect(onReady).toHaveBeenCalledTimes(1));
  const s = onReady.mock.calls[0][0];
  s.editor.commands.insertContent("保存最新正文");
  rerender(<PromptEditor {...props} onRequestSave={newSave} />);
  s.editor.view.dom.dispatchEvent(
    new KeyboardEvent("keydown", {
      key: "s",
      ctrlKey: true,
      bubbles: true,
      cancelable: true,
    }),
  );
  expect(oldSave).not.toHaveBeenCalled();
  expect(newSave).toHaveBeenCalledTimes(1);
  expect(JSON.stringify(newSave.mock.calls[0][0])).toContain("保存最新正文");
  expect(onReady).toHaveBeenCalledTimes(1);
});
