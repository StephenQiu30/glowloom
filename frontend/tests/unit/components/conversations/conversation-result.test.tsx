import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ConversationResults } from "@/components/conversations/conversation-result";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
function taskDraft() {
  return {
    ...emptyPromptDraft(),
    mode: "text2image",
    modelVersion: "fixture-model-version",
  };
}
vi.mock("next/dynamic", () => ({
  default:
    () =>
    ({
      id,
      onReuse,
      presentation,
    }: {
      id: string;
      presentation?: string;
      onReuse?: (task: { editor_draft: unknown }) => void;
    }) => (
      <button
        data-presentation={presentation}
        onClick={() => onReuse?.({ editor_draft: taskDraft() })}
      >
        {id}
      </button>
    ),
}));
afterEach(cleanup);
it("uses formal operation identities once and only returns a draft for explicit reuse", () => {
  const id = "11111111-1111-4111-8111-111111111111",
    reuse = vi.fn();
  const part = (content: string, kind = "operation") => ({
    run_id: "run",
    message_id: content,
    conversation_id: 1,
    sequence: "1:1",
    part_index: 0,
    kind,
    content,
  });
  render(
    <ConversationResults
      projectID="project"
      messages={[
        part(id),
        part(id),
        part("javascript:alert(1)"),
        part("assistant text", "assistant"),
      ]}
      onReuse={reuse}
    />,
  );
  expect(screen.getAllByRole("button")).toHaveLength(1);
  expect(screen.getByRole("button").getAttribute("data-presentation")).toBe(
    "asset",
  );
  expect(reuse).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button"));
  expect(reuse).toHaveBeenCalledWith(taskDraft());
});

it("defers offscreen operation reads until the result approaches the viewport", async () => {
  let intersect!: IntersectionObserverCallback;
  const disconnect = vi.fn();
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(callback: IntersectionObserverCallback) {
        intersect = callback;
      }
      observe() {}
      disconnect = disconnect;
    },
  );
  try {
    const id = "11111111-1111-4111-8111-111111111111";
    render(<ConversationResults projectID="project" operationID={id} />);
    expect(screen.queryByText(id)).toBeNull();
    await act(async () =>
      intersect(
        [{ isIntersecting: true } as IntersectionObserverEntry],
        {} as IntersectionObserver,
      ),
    );
    expect(screen.getByText(id)).toBeTruthy();
  } finally {
    vi.unstubAllGlobals();
  }
});
