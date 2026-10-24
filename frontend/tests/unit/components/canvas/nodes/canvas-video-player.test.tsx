import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { CanvasVideoPlayer } from "@/components/canvas/nodes/canvas-video-player";
import {
  CanvasNodeType,
  type CanvasNodeData,
} from "@/components/canvas/types/canvas";
const node: CanvasNodeData = {
  id: "video-test",
  type: CanvasNodeType.Video,
  title: "视频验收",
  position: { x: 0, y: 0 },
  width: 320,
  height: 180,
  metadata: { content: "/test-video.mp4" },
};
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
it("播放入口不触发节点拖动，播放后使用原生控件，结束后恢复入口", () => {
  const play = vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
  const drag = vi.fn();
  render(
    <div onPointerDown={drag}>
      <CanvasVideoPlayer node={node} />
    </div>,
  );
  const video = screen.getByLabelText("视频验收") as HTMLVideoElement;
  expect(video.controls).toBe(false);
  const trigger = screen.getByRole("button", { name: "播放视频" });
  fireEvent.pointerDown(trigger);
  fireEvent.click(trigger);
  expect(drag).not.toHaveBeenCalled();
  expect(play).toHaveBeenCalledOnce();
  fireEvent.play(video);
  expect(video.controls).toBe(true);
  expect(screen.queryByRole("button", { name: "播放视频" })).toBeNull();
  fireEvent.ended(video);
  expect(screen.getByRole("button", { name: "播放视频" })).toBeTruthy();
});
it("媒体失败呈现重试，并保留续播位置", () => {
  const play = vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
  const load = vi
    .spyOn(HTMLMediaElement.prototype, "load")
    .mockImplementation(() => {});
  render(<CanvasVideoPlayer node={node} />);
  const video = screen.getByLabelText("视频验收") as HTMLVideoElement;
  video.currentTime = 5;
  fireEvent.error(video);
  expect(screen.getByRole("status").textContent).toContain("暂时无法播放");
  fireEvent.click(screen.getByRole("button", { name: "重试播放视频" }));
  expect(load).toHaveBeenCalledOnce();
  expect(play).toHaveBeenCalledOnce();
  Object.defineProperty(video, "duration", { value: 12 });
  fireEvent.loadedMetadata(video);
  expect(video.currentTime).toBe(5);
  expect(screen.queryByRole("status")).toBeNull();
});

it("替换视频后重置播放状态，不继承旧媒体的错误与进度", () => {
  const { rerender } = render(<CanvasVideoPlayer node={node} />);
  const previous = screen.getByLabelText("视频验收") as HTMLVideoElement;
  fireEvent.play(previous);
  expect(previous.controls).toBe(true);
  rerender(
    <CanvasVideoPlayer
      node={{ ...node, metadata: { content: "/replacement.mp4" } }}
    />,
  );
  const current = screen.getByLabelText("视频验收") as HTMLVideoElement;
  expect(current).not.toBe(previous);
  expect(current.controls).toBe(false);
  expect(screen.getByRole("button", { name: "播放视频" })).toBeTruthy();
});

it("播放按钮与原生视频控件隔离鼠标、双击和快捷键，不拖动或打开节点编辑", () => {
  const mouse = vi.fn(),
    key = vi.fn(),
    double = vi.fn();
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
  render(
    <div onMouseDown={mouse} onKeyDown={key} onDoubleClick={double}>
      <CanvasVideoPlayer node={node} />
    </div>,
  );
  const trigger = screen.getByRole("button", { name: "播放视频" });
  fireEvent.mouseDown(trigger);
  fireEvent.keyDown(trigger, { key: " " });
  const video = screen.getByLabelText("视频验收");
  fireEvent.play(video);
  fireEvent.mouseDown(video);
  fireEvent.doubleClick(video);
  fireEvent.keyDown(video, { key: "ArrowRight" });
  expect(mouse).not.toHaveBeenCalled();
  expect(key).not.toHaveBeenCalled();
  expect(double).not.toHaveBeenCalled();
});

it("开始播放后立刻暂停导致的 AbortError 不显示媒体损坏提示", async () => {
  vi.spyOn(HTMLMediaElement.prototype, "play").mockRejectedValue(
    new DOMException("play interrupted by pause", "AbortError"),
  );
  render(<CanvasVideoPlayer node={node} />);
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "播放视频" }));
  });
  expect(screen.queryByRole("status")).toBeNull();
  expect(screen.getByRole("button", { name: "播放视频" })).toBeTruthy();
});
