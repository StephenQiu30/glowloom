import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { RecoveryField } from "@glowloom/canvas/recovery";
import { CanvasLocalDrafts } from "@/components/canvas/controls/canvas-local-drafts";

afterEach(cleanup);

it("keeps a deleted node's palette private on refresh and creates a copy only on request", async () => {
  const field: RecoveryField = {
    key: "palette",
    id: "node",
    targetId: "node",
    field: "native",
    collection: "nodes",
    generation: 1,
    copyRequired: true,
    before: { exists: true },
    current: { exists: false },
    after: {
      exists: true,
      value: {
        config: {
          cinema_kit: { palette: { name: "沙漠史诗色调" } },
          draft: { composer: { modelVersion: "internal-id" } },
        },
      },
    },
  };
  const apply = vi.fn().mockResolvedValue(undefined),
    discard = vi.fn().mockResolvedValue(undefined);
  render(
    <CanvasLocalDrafts
      sources={[{ sourceId: "source", fields: [field] }]}
      commitSeq="63"
      writable
      apply={apply}
      discard={discard}
    />,
  );
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "本机草稿" }));
  expect(await screen.findByText("沙漠史诗色调")).toBeTruthy();
  expect(document.body.textContent).not.toContain("internal-id");
  fireEvent.click(screen.getByRole("button", { name: "放弃草稿" }));
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(discard).not.toHaveBeenCalled();
  expect(apply).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "另存为新节点" }));
  await waitFor(() =>
    expect(apply).toHaveBeenCalledWith(
      "source",
      [{ ...field, copyNode: true }],
      "63",
    ),
  );
});
