import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TimelineDialog } from "./timeline-dialog";
import { CanvasNodeType } from "./model";
import { createTimeline } from "./timeline";

vi.mock("./timeline-preview", () => ({ TimelinePreview: () => null }));
vi.mock("./media-export-panel", () => ({
  MediaExportPanel: ({ disabled }: { disabled: boolean }) => (
    <button disabled={disabled}>导出门禁</button>
  ),
}));
vi.mock("./transcription-panel", () => ({
  TranscriptionPanel: ({
    disabled,
    onBusy,
  }: {
    disabled: boolean;
    onBusy: (busy: boolean) => void;
  }) => (
    <>
      <button disabled={disabled} onClick={() => onBusy(true)}>
        字幕请求未知
      </button>
      <button disabled={disabled} onClick={() => onBusy(false)}>
        字幕请求已核验
      </button>
    </>
  ),
}));
afterEach(cleanup);
it("字幕请求未确认时关闭、时间线写入和导出受阻，核验入口保持可操作", async () => {
  const close = vi.fn(),
    save = vi.fn().mockResolvedValue(true);
  render(
    <TimelineDialog
      node={{
        id: "00000000-0000-4000-8000-000000000001",
        type: CanvasNodeType.Timeline,
        title: "正式时间线",
        position: { x: 0, y: 0 },
        width: 720,
        height: 460,
        zIndex: 0,
        timeline: createTimeline(),
      }}
      nodes={[]}
      projectId="00000000-0000-4000-8000-000000000002"
      canvasId="00000000-0000-4000-8000-000000000003"
      readOnly={false}
      onClose={close}
      onSave={save}
      onExportResult={vi.fn().mockResolvedValue(true)}
      source={() => ({
        canvas_id: "00000000-0000-4000-8000-000000000003",
        node_id: "00000000-0000-4000-8000-000000000001",
        revision: 4,
      })}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "字幕请求未知" }));
  for (const name of ["关闭", "保存时间线", "导出门禁"])
    expect(
      (screen.getByRole("button", { name }) as HTMLButtonElement).disabled,
    ).toBe(true);
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  expect(close).not.toHaveBeenCalled();
  expect(save).not.toHaveBeenCalled();
  expect(
    (
      screen.getByRole("button", {
        name: "字幕请求已核验",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "字幕请求已核验" }));
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "关闭" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "关闭" }));
  expect(close).toHaveBeenCalledOnce();
});
