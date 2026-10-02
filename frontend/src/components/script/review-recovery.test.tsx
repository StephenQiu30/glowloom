import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ReviewRecovery } from "./review-recovery";
import type { useReviewWriter } from "./use-review-writer";
afterEach(cleanup);
const id = "11111111-1111-4111-8111-111111111111";
function control(): ReturnType<typeof useReviewWriter> {
  return {
    ready: true,
    intent: null,
    busy: false,
    error: undefined,
    storageError: undefined,
    rejected: undefined,
    locked: false,
    submit: vi.fn(async () => {}),
    replay: vi.fn(async () => {}),
    restoreStorage: vi.fn(async () => {}),
    acknowledgeLatest: vi.fn(),
  };
}
it("unknown禁用fieldset内原输入后，焦点转到可用原键核验；可用原焦点不抢占", async () => {
  const writer = control();
  const intent = {
    version: 1 as const,
    origin: window.location.origin,
    actorId: id,
    orgId: id,
    projectId: id,
    key: id,
    action: "resplit" as const,
    body: {
      version_id: id,
      expected_revision: 2,
      expected_split_revision: 1,
      candidate_set_id: id,
      ack_invalidate: false,
    },
  };
  const ui = render(
    <>
      <fieldset>
        <input aria-label="原审核输入" />
      </fieldset>
      <button>可用阅读按钮</button>
      <ReviewRecovery writer={writer} />
    </>,
  );
  const original = screen.getByRole("textbox", { name: "原审核输入" });
  original.focus();
  ui.rerender(
    <>
      <fieldset disabled>
        <input aria-label="原审核输入" />
      </fieldset>
      <button>可用阅读按钮</button>
      <ReviewRecovery writer={{ ...writer, locked: true, intent }} />
    </>,
  );
  const recovery = screen.getByRole("button", { name: "人工使用原键核验审核" });
  await waitFor(() => expect(document.activeElement).toBe(recovery));
  const reader = screen.getByRole("button", { name: "可用阅读按钮" });
  reader.focus();
  ui.rerender(
    <>
      <fieldset disabled>
        <input aria-label="原审核输入" />
      </fieldset>
      <button>可用阅读按钮</button>
      <ReviewRecovery
        writer={{ ...writer, locked: true, intent, error: "尚未确认" }}
      />
    </>,
  );
  expect(document.activeElement).toBe(reader);
});
