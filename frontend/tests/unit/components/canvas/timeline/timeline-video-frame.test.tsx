import { StrictMode } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { TimelineThumbnail } from "@/components/canvas/timeline/timeline-thumbnail";
import { acquireTimelineVideoFrame } from "@/components/canvas/timeline/timeline-video-frame";
let videos: HTMLVideoElement[];
let draw: ReturnType<typeof vi.fn>;
let revoke: ReturnType<typeof vi.fn>;
beforeEach(() => {
  videos = [];
  draw = vi.fn();
  revoke = vi.fn();
  vi.stubGlobal(
    "URL",
    Object.assign(URL, {
      createObjectURL: vi.fn(() => "blob:frame"),
      revokeObjectURL: revoke,
    }),
  );
  vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(function (
    this: HTMLVideoElement,
  ) {
    if (this.hasAttribute("src") && !videos.includes(this)) videos.push(this);
  });
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
    drawImage: draw,
  } as unknown as CanvasRenderingContext2D);
  vi.spyOn(HTMLCanvasElement.prototype, "toBlob").mockImplementation(
    (callback) => callback(new Blob(["frame"], { type: "image/webp" })),
  );
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function ready(video: HTMLVideoElement) {
  Object.defineProperties(video, {
    readyState: { value: 2, configurable: true },
    duration: { value: 7 },
    videoWidth: { value: 1920 },
    videoHeight: { value: 1080 },
  });
  fireEvent.loadedMetadata(video);
  fireEvent.loadedData(video);
  fireEvent.seeked(video);
}
const media = {
  asset: { id: "video", kind: "video", duration_ms: 7000 },
  url: "https://media.example/video.mp4",
};
it("shares one decoded frame and revokes only the final lease", async () => {
  const a = acquireTimelineVideoFrame(media.url, 1000),
    b = acquireTimelineVideoFrame(media.url, 1000);
  await Promise.resolve();
  expect(videos).toHaveLength(1);
  ready(videos[0]);
  expect(videos[0].currentTime).toBe(1);
  await expect(a.promise).resolves.toBe("blob:frame");
  await expect(b.promise).resolves.toBe("blob:frame");
  expect(draw).toHaveBeenCalledTimes(1);
  expect(draw.mock.calls[0].slice(1)).toEqual([0, 0, 256, 144]);
  a.release();
  expect(revoke).not.toHaveBeenCalled();
  b.release();
  expect(revoke).toHaveBeenCalledWith("blob:frame");
  expect(videos[0].hasAttribute("src")).toBe(false);
});
it("repeats a bounded frame in StrictMode and captures a changed source inpoint", async () => {
  const view = render(
    <StrictMode>
      <TimelineThumbnail media={media} kind="video" strip />
    </StrictMode>,
  );
  await waitFor(() => expect(videos).toHaveLength(1));
  await act(async () => ready(videos[0]));
  expect(view.container.querySelector("video")).toBeNull();
  expect(
    view.container.querySelector<HTMLElement>("[data-thumbnail-strip]")?.style
      .backgroundRepeat,
  ).toBe("repeat-x");
  view.rerender(
    <StrictMode>
      <TimelineThumbnail media={media} kind="video" strip startMS={2000} />
    </StrictMode>,
  );
  await waitFor(() => expect(videos).toHaveLength(2));
  await act(async () => ready(videos[1]));
  expect(videos[1].currentTime).toBe(2);
  expect(draw).toHaveBeenCalledTimes(2);
  expect(revoke).toHaveBeenCalledTimes(1);
});
it("falls back to normal video when canvas extraction is forbidden", async () => {
  draw.mockImplementation(() => {
    throw new DOMException("tainted", "SecurityError");
  });
  const view = render(
    <TimelineThumbnail media={media} kind="video" strip startMS={1000} />,
  );
  await waitFor(() => expect(videos).toHaveLength(1));
  await act(async () => ready(videos[0]));
  const fallback = view.container.querySelector("video")!;
  expect(fallback.getAttribute("src")).toBe(media.url);
  expect(fallback.hasAttribute("crossorigin")).toBe(false);
  ready(fallback);
  expect(fallback.currentTime).toBe(1);
  expect(revoke).not.toHaveBeenCalled();
});
it("bounds concurrent decoding and cancels source work on final release", async () => {
  const leases = [0, 1, 2].map((n) =>
    acquireTimelineVideoFrame(`${media.url}?${n}`, 0),
  );
  const outcomes = Promise.allSettled(leases.map((lease) => lease.promise));
  await Promise.resolve();
  expect(videos).toHaveLength(2);
  leases[0].release();
  await waitFor(() => expect(videos).toHaveLength(3));
  expect(videos[0].hasAttribute("src")).toBe(false);
  leases[1].release();
  leases[2].release();
  expect(
    (await outcomes).every((outcome) => outcome.status === "rejected"),
  ).toBe(true);
});
it("caps active frame entries at 64 and retries a failed source with a fresh decoder", async () => {
  const leases = Array.from({ length: 65 }, (_, n) =>
    acquireTimelineVideoFrame(`${media.url}?capacity=${n}`, 0),
  );
  const outcomes = Promise.allSettled(leases.map((lease) => lease.promise));
  await expect(leases[64].promise).rejects.toThrow("Thumbnail capacity");
  leases.forEach((lease) => lease.release());
  expect(
    (await outcomes).every((outcome) => outcome.status === "rejected"),
  ).toBe(true);
  const failed = acquireTimelineVideoFrame(media.url, 0);
  const rejection = expect(failed.promise).rejects.toThrow("unavailable");
  await Promise.resolve();
  fireEvent.error(videos.at(-1)!);
  await rejection;
  failed.release();
  const retry = acquireTimelineVideoFrame(media.url, 0);
  await Promise.resolve();
  ready(videos.at(-1)!);
  await expect(retry.promise).resolves.toBe("blob:frame");
  retry.release();
});
