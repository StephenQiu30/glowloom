import { cleanup, render, screen, waitFor, act } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ComposerSession } from "@/components/generation/composer-session";
import { CanvasTextEditor } from "./text-editor";
let session: ComposerSession;
vi.mock("@/components/generation/prompt-editor", async (original) => {
  const { PromptEditor } =
    await original<typeof import("@/components/generation/prompt-editor")>();
  return {
    PromptEditor: (props: React.ComponentProps<typeof PromptEditor>) => (
      <PromptEditor
        {...props}
        onReady={(value) => {
          session = value;
          props.onReady(value);
        }}
      />
    ),
  };
});
afterEach(cleanup);
it("共享编辑器提交多行文本，外部撤销重做同步同一实例", async () => {
  const change = vi.fn();
  const view = render(
    <CanvasTextEditor id="text-node" text="" onChange={change} />,
  );
  await screen.findByRole("textbox", { name: "文本内容" });
  await waitFor(() => expect(session).toBeTruthy());
  act(() => {
    session.editor.commands.insertContent([
      { type: "paragraph", content: [{ type: "text", text: "第一行" }] },
      { type: "paragraph", content: [{ type: "text", text: "第二行" }] },
    ]);
  });
  expect(change).toHaveBeenLastCalledWith("第一行\n第二行");
  const editor = session.editor;
  view.rerender(
    <CanvasTextEditor id="text-node" text="撤销后的正文" onChange={change} />,
  );
  await waitFor(() => expect(editor.getText()).toBe("撤销后的正文"));
  expect(session.editor).toBe(editor);
});
