import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { CanvasMediaActions } from "./actions";
import { canvasMediaSource } from "./source";
import { createMediaNode } from "../document";
import { TooltipProvider } from "@/components/ui/tooltip";
vi.mock("./source", () => ({ canvasMediaSource: vi.fn() }));
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
it("离开节点后中止慢速原件下载，迟到结果不触发保存", async () => {
  let resolve!: (result: Awaited<ReturnType<typeof canvasMediaSource>>) => void;
  vi.mocked(canvasMediaSource).mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const project = crypto.randomUUID(),
    asset = { id: crypto.randomUUID(), project_id: project, kind: "image" };
  const click = vi
    .spyOn(HTMLAnchorElement.prototype, "click")
    .mockImplementation(() => {});
  const view = render(
    <TooltipProvider>
      <CanvasMediaActions
        node={createMediaNode(asset, project, 0)}
        projectID={project}
        onPreview={vi.fn()}
        onImageTool={vi.fn()}
        onCreate={vi.fn()}
        disabled={false}
      />
    </TooltipProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "下载原件" }));
  const signal = vi.mocked(canvasMediaSource).mock.calls[0][2];
  view.unmount();
  expect(signal.aborted).toBe(true);
  resolve({ blob: new Blob(["original"]), asset });
  await waitFor(() => expect(click).not.toHaveBeenCalled());
});
