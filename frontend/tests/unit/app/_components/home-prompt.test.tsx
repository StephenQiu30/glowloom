import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { HomePrompt } from "@/app/_components/home-prompt";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import type { ComposerSession } from "@/components/generation/composer-session";

afterEach(cleanup);
it("聚焦与移动光标不是草稿修改，不取消恢复候选", async () => {
  const change = vi.fn();
  let session: ComposerSession | undefined;
  const draft = emptyPromptDraft();
  draft.document.content[0].content = [{ type: "text", text: "已有文字" }];
  render(
    <HomePrompt
      draft={draft}
      contextKey="a/s/home-input:image"
      onChange={change}
      placeholder="输入"
      onSubmit={() => {}}
      onReady={(value) => {
        session = value;
      }}
    />,
  );
  await waitFor(() => expect(session).toBeTruthy());
  act(() => {
    session!.editor.commands.setTextSelection(2);
  });
  fireEvent.focus(screen.getByLabelText("创作提示词"));
  expect(change).not.toHaveBeenCalled();
  fireEvent.paste(screen.getByLabelText("创作提示词"), {
    clipboardData: { files: [], getData: () => "新文字" },
  });
  expect(change).toHaveBeenCalled();
});
