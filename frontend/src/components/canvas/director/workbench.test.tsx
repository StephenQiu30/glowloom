import { useEffect } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createDirectorConfig } from "./model";
import { DirectorWorkbench } from "./workbench";
import type { DirectorCapture } from "./viewport";

const api = vi.hoisted(() => ({ capture: vi.fn() }));
vi.mock("./viewport", () => ({
  DirectorViewport: ({
    onCaptureReady,
  }: {
    onCaptureReady: (capture: DirectorCapture | null) => void;
  }) => {
    useEffect(() => {
      onCaptureReady(api.capture);
      return () => onCaptureReady(null);
    }, [onCaptureReady]);
    return <div aria-label="受控视口" />;
  },
}));
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
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
describe("导演台捕获生命周期", () => {
  it("保存并关闭时先确认场景保存，再捕获并交给正式人工上传链", async () => {
    api.capture.mockResolvedValue(
      new File(["pixels"], "scene.png", { type: "image/png" }),
    );
    const view = mount();
    fireEvent.click(screen.getByRole("button", { name: "保存并关闭" }));
    await waitFor(() => expect(view.capture).toHaveBeenCalledOnce());
    expect(view.save.mock.invocationCallOrder[0]).toBeLessThan(
      api.capture.mock.invocationCallOrder[0],
    );
    expect(view.capture.mock.calls[0][1]).toMatchObject({
      sceneId: view.save.mock.calls[0][0].id,
      shotId: view.save.mock.calls[0][0].activeShotId,
    });
    expect(view.save.mock.calls[0][0].cover).toBeUndefined();
  });
  it("迟到捕获不回填已经卸载的场景", async () => {
    let finish!: (file: File) => void;
    api.capture.mockImplementation(
      () =>
        new Promise<File>((resolve) => {
          finish = resolve;
        }),
    );
    const view = mount();
    fireEvent.click(screen.getByRole("button", { name: "场景截图" }));
    await waitFor(() => expect(api.capture).toHaveBeenCalledOnce());
    view.unmount();
    await act(async () => {
      finish(new File(["pixels"], "scene.png", { type: "image/png" }));
    });
    expect(view.capture).not.toHaveBeenCalled();
  });
  it("场景保存未确认时不捕获或回填输出", async () => {
    const view = mount(vi.fn().mockResolvedValue(false));
    fireEvent.click(screen.getByRole("button", { name: "场景截图" }));
    await screen.findByText("导演台尚未确认保存，请修复画布错误后重试。");
    expect(view.save).toHaveBeenCalledOnce();
    expect(api.capture).not.toHaveBeenCalled();
    expect(view.capture).not.toHaveBeenCalled();
  });
});
