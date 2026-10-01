import { useEffect } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { createDirectorConfig } from "./model";
import { DirectorWorkbench } from "./workbench";
import type { DirectorRecord } from "./viewport";

const api = vi.hoisted(() => ({ record: vi.fn() }));
vi.mock("./viewport", () => ({
  DirectorViewport: ({
    onRecordReady,
  }: {
    onRecordReady: (record: DirectorRecord | null) => void;
  }) => {
    useEffect(() => {
      onRecordReady(api.record);
      return () => onRecordReady(null);
    }, [onRecordReady]);
    return <div aria-label="受控录制视口" />;
  },
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
function mount(save = vi.fn().mockResolvedValue(true)) {
  const capture = vi.fn();
  const view = render(
    <DirectorWorkbench
      projectId="10000000-0000-4000-8000-000000000000"
      nodes={[]}
      value={createDirectorConfig("合成场景")}
      readOnly={false}
      onClose={vi.fn()}
      onSave={save}
      onCapture={capture}
      onPrompt={vi.fn()}
      onGenerate={vi.fn().mockResolvedValue(true)}
    />,
  );
  return { ...view, save, capture };
}
it("场景真实保存确认之后才录制原始WebM并进入人工确认，不提前创建资产", async () => {
  api.record.mockResolvedValue(
    new File(["actual bytes"], "白膜.webm", { type: "video/webm" }),
  );
  const view = mount();
  fireEvent.click(screen.getByRole("button", { name: "录制白膜视频" }));
  await waitFor(() => expect(view.capture).toHaveBeenCalledOnce());
  expect(view.save.mock.invocationCallOrder[0]).toBeLessThan(
    api.record.mock.invocationCallOrder[0],
  );
  expect(view.capture.mock.calls[0][0][0].type).toBe("video/webm");
  expect(view.capture.mock.calls[0][1]).toMatchObject({
    sceneId: view.save.mock.calls[0][0].id,
  });
  expect(view.capture.mock.calls[0][2]).toBe("video");
});
it("保存尚未确认则不录制，卸载或取消阻断迟到回填", async () => {
  const blocked = mount(vi.fn().mockResolvedValue(false));
  fireEvent.click(screen.getByRole("button", { name: "录制白膜视频" }));
  await screen.findByText("导演台尚未确认保存，请修复画布错误后重试。");
  expect(api.record).not.toHaveBeenCalled();
  blocked.unmount();
  let finish!: (file: File) => void;
  api.record.mockImplementation(
    () =>
      new Promise<File>((resolve) => {
        finish = resolve;
      }),
  );
  const view = mount();
  fireEvent.click(screen.getByRole("button", { name: "录制白膜视频" }));
  await waitFor(() => expect(api.record).toHaveBeenCalledOnce());
  const signal = api.record.mock.calls[0][2] as AbortSignal;
  view.unmount();
  expect(signal.aborted).toBe(true);
  await act(async () => {
    finish(new File(["late bytes"], "late.webm", { type: "video/webm" }));
  });
  expect(view.capture).not.toHaveBeenCalled();
});
