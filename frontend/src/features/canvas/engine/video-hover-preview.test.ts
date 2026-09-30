import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  bindCanvasVideoHoverPreview,
  VIDEO_HOVER_DELAY_MS,
  VIDEO_HOVER_PREVIEW_MS,
} from "./video-hover-preview";

class VisibleObserver {
  static instances: VisibleObserver[] = [];
  constructor(readonly callback: IntersectionObserverCallback) {
    VisibleObserver.instances.push(this);
  }
  observe = vi.fn();
  disconnect = vi.fn();
  leave() {
    this.callback(
      [{ isIntersecting: false } as IntersectionObserverEntry],
      this as unknown as IntersectionObserver,
    );
  }
}
const cleanups: (() => void)[] = [];
function target() {
  const viewport = document.createElement("div");
  viewport.dataset.canvasViewport = "true";
  const node = document.createElement("div");
  viewport.append(node);
  document.body.append(viewport);
  return { viewport, node };
}
function enter(node: HTMLElement, pointerType = "mouse") {
  const event = new Event("pointerenter");
  Object.assign(event, { pointerType, buttons: 0 });
  node.dispatchEvent(event);
}
beforeEach(() => {
  vi.useFakeTimers();
  VisibleObserver.instances = [];
  vi.stubGlobal("IntersectionObserver", VisibleObserver);
  vi.stubGlobal("matchMedia", () => ({ matches: false }));
  vi.spyOn(document, "hidden", "get").mockReturnValue(false);
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
  vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
});
afterEach(() => {
  cleanups.splice(0).forEach((cleanup) => cleanup());
  document.body.replaceChildren();
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("源画布视频悬停租约", () => {
  it("350ms 后匿名静音播放，3秒结束后移除src与解码器", async () => {
    const { node } = target();
    const resolve = vi
      .fn()
      .mockResolvedValue("https://media.example/owned.mp4");
    cleanups.push(bindCanvasVideoHoverPreview(node, resolve));
    enter(node);
    await vi.advanceTimersByTimeAsync(VIDEO_HOVER_DELAY_MS - 1);
    expect(resolve).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    const video = node.querySelector("video")!;
    expect(video.crossOrigin).toBe("anonymous");
    expect(video.muted).toBe(true);
    expect(video.src).toBe("https://media.example/owned.mp4");
    video.dispatchEvent(new Event("playing"));
    await vi.advanceTimersByTimeAsync(VIDEO_HOVER_PREVIEW_MS);
    expect(node.querySelector("video")).toBeNull();
    expect(video.hasAttribute("src")).toBe(false);
    expect(video.pause).toHaveBeenCalled();
    expect(video.load).toHaveBeenCalled();
  });
  it("新悬停取消旧pending请求，迟到响应不能挂第二个video", async () => {
    const a = target().node,
      b = target().node;
    let signal: AbortSignal | undefined;
    let resolveA: (value: string) => void = () => {};
    const resolve = vi.fn((current: AbortSignal) => {
      signal = current;
      return new Promise<string>((done) => {
        resolveA = done;
      });
    });
    cleanups.push(bindCanvasVideoHoverPreview(a, resolve));
    cleanups.push(
      bindCanvasVideoHoverPreview(b, async () => "https://media.example/b.mp4"),
    );
    enter(a);
    await vi.advanceTimersByTimeAsync(350);
    enter(b);
    expect(signal?.aborted).toBe(true);
    resolveA("https://media.example/a.mp4");
    await vi.advanceTimersByTimeAsync(350);
    expect(a.querySelector("video")).toBeNull();
    expect(document.querySelectorAll("video")).toHaveLength(1);
  });
  it("8秒deadline取消卡住的授权请求", async () => {
    const { node } = target();
    let signal: AbortSignal | undefined;
    cleanups.push(
      bindCanvasVideoHoverPreview(node, (current) => {
        signal = current;
        return new Promise(() => {});
      }),
    );
    enter(node);
    await vi.advanceTimersByTimeAsync(8000);
    expect(signal?.aborted).toBe(true);
    expect(node.querySelector("video")).toBeNull();
  });
  it.each(["pointerdown", "wheel", "blur", "keydown"])(
    "%s 取消并释放当前媒体",
    async (event) => {
      const { node } = target();
      cleanups.push(
        bindCanvasVideoHoverPreview(
          node,
          async () => "https://media.example/a.mp4",
        ),
      );
      enter(node);
      await vi.advanceTimersByTimeAsync(350);
      window.dispatchEvent(new Event(event));
      expect(node.querySelector("video")).toBeNull();
    },
  );
  it("离屏、viewport交互和页面隐藏取消租约", async () => {
    const { node, viewport } = target();
    cleanups.push(
      bindCanvasVideoHoverPreview(
        node,
        async () => "https://media.example/a.mp4",
      ),
    );
    enter(node);
    await vi.advanceTimersByTimeAsync(350);
    VisibleObserver.instances.at(-1)!.leave();
    expect(node.querySelector("video")).toBeNull();
    enter(node);
    await vi.advanceTimersByTimeAsync(350);
    viewport.dataset.canvasViewportInteracting = "true";
    await Promise.resolve();
    expect(node.querySelector("video")).toBeNull();
    delete viewport.dataset.canvasViewportInteracting;
    enter(node);
    await vi.advanceTimersByTimeAsync(350);
    document.dispatchEvent(new Event("visibilitychange"));
    expect(node.querySelector("video")).toBeNull();
  });
  it("触屏、减少动态效果或显式播放正在运行时不自动悬停", async () => {
    const { node } = target();
    const resolve = vi.fn().mockResolvedValue("https://media.example/a.mp4");
    cleanups.push(bindCanvasVideoHoverPreview(node, resolve));
    enter(node, "touch");
    await vi.advanceTimersByTimeAsync(350);
    vi.stubGlobal("matchMedia", () => ({ matches: true }));
    enter(node);
    await vi.advanceTimersByTimeAsync(350);
    expect(resolve).not.toHaveBeenCalled();
    vi.stubGlobal("matchMedia", () => ({ matches: false }));
    const explicit = document.createElement("audio");
    Object.defineProperty(explicit, "paused", { value: false });
    document.body.append(explicit);
    enter(node);
    await vi.advanceTimersByTimeAsync(350);
    expect(resolve).not.toHaveBeenCalled();
  });
});
