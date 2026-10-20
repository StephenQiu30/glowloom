import { Editor } from "@tiptap/core";
import { it, expect, vi } from "vitest";
import { composerEditorOptions } from "@/components/generation/composer-session";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
it("unmount before composition settlement releases global composition", () => {
  vi.useFakeTimers();
  const events: boolean[] = [];
  const editor = new Editor(
    composerEditorOptions({
      contextKey: "review",
      replicaId: "review",
      draft: emptyPromptDraft(),
      collaboration: {
        history: { undo: () => false, redo: () => false },
        pendingReference: () => false,
        composition: (active) => events.push(active),
      },
    }),
  );
  // Invoke actual ProseMirror DOM handlers in browser event order; destruction
  // before the 0ms settlement is the ordinary React unmount race.
  for (const plugin of editor.state.plugins)
    plugin.props.handleDOMEvents?.compositionstart?.call(
      plugin,
      editor.view,
      new CompositionEvent("compositionstart"),
    );
  for (const plugin of editor.state.plugins)
    plugin.props.handleDOMEvents?.compositionend?.call(
      plugin,
      editor.view,
      new CompositionEvent("compositionend"),
    );
  editor.destroy();
  vi.runAllTimers();
  vi.useRealTimers();
  expect(events.at(-1)).toBe(false);
});
