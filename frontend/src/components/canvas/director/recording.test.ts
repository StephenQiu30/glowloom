import { afterEach, describe, expect, it, vi } from "vitest";
import {
  recordDirectorCanvas,
  validateDirectorRecordingDuration,
} from "./recording";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("owned director recording", () => {
  it("rejects a half-length actual decode but accepts a small frame scheduling error", () => {
    expect(() => validateDirectorRecordingDuration(5, 10, 24)).toThrow(
      "不完整",
    );
    expect(() => validateDirectorRecordingDuration(9.9, 10, 24)).not.toThrow();
    expect(() => validateDirectorRecordingDuration(10.21, 10, 24)).toThrow();
  });
  it("rejects duration and unsupported codecs before rendering or capturing", async () => {
    const renderFrame = vi.fn();
    const signal = new AbortController().signal;
    await expect(
      recordDirectorCanvas({
        duration: 60.01,
        fps: 24,
        aspectRatio: "16:9",
        renderFrame,
        signal,
      }),
    ).rejects.toThrow("60 秒");
    vi.stubGlobal("MediaRecorder", {
      isTypeSupported: () => false,
    });
    await expect(
      recordDirectorCanvas({
        duration: 1,
        fps: 24,
        aspectRatio: "16:9",
        renderFrame,
        signal,
      }),
    ).rejects.toThrow("VP8／VP9");
    expect(renderFrame).not.toHaveBeenCalled();
  });

  it("aborts an active recorder and stops its tracks without publishing a file", async () => {
    vi.useFakeTimers();
    const stopTrack = vi.fn();
    const stream = {
      getTracks: () => [{ stop: stopTrack }],
    } as unknown as MediaStream;
    const drawImage = vi.fn();
    const originalCreate = document.createElement.bind(document);
    vi.spyOn(document, "createElement").mockImplementation((tag, options) => {
      const element = originalCreate(tag, options);
      if (tag === "canvas") {
        Object.defineProperty(element, "getContext", {
          value: () => ({ drawImage }),
        });
        Object.defineProperty(element, "captureStream", {
          value: () => stream,
        });
      }
      return element;
    });
    const started = vi.fn();
    const stopped = vi.fn();
    class Recorder extends EventTarget {
      static isTypeSupported = () => true;
      state: RecordingState = "inactive";
      mimeType = "video/webm;codecs=vp9";
      start() {
        this.state = "recording";
        started();
      }
      stop() {
        this.state = "inactive";
        stopped();
        this.dispatchEvent(new Event("stop"));
      }
    }
    vi.stubGlobal("MediaRecorder", Recorder);
    const source = originalCreate("canvas");
    source.width = 640;
    source.height = 360;
    const controller = new AbortController();
    const pending = recordDirectorCanvas({
      duration: 1,
      fps: 24,
      aspectRatio: "16:9",
      renderFrame: async () => source,
      signal: controller.signal,
    });
    const rejected = expect(pending).rejects.toThrow("录制已取消");
    await Promise.resolve();
    expect(started).toHaveBeenCalledOnce();
    controller.abort();
    await rejected;
    expect(stopped).toHaveBeenCalledOnce();
    expect(stopTrack).toHaveBeenCalledOnce();
    expect(vi.getTimerCount()).toBe(0);
  });
});
