import { StrictMode } from "react";
import { act, cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TimelineThumbnail } from "@/components/canvas/timeline/timeline-thumbnail";
const waveform = vi.hoisted(() => ({ acquire: vi.fn() }));
vi.mock("@/components/canvas/timeline/timeline-waveform", () => ({
  acquireTimelineWaveform: waveform.acquire,
}));
import type { TimelineMedia } from "@/components/canvas/timeline/timeline-preview";
afterEach(() => {
  cleanup();
  waveform.acquire.mockReset();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
const media = (kind = "video", suffix = "one"): TimelineMedia => ({
  asset: { id: crypto.randomUUID(), kind, duration_ms: 7000, byte_size: 1000 },
  url: `https://media.example/${suffix}.mp4`,
});
it("updates a ready video thumbnail after trim and seeks back to zero on undo", () => {
  const source = media();
  const view = render(
    <TimelineThumbnail media={source} kind="video" startMS={1000} />,
  );
  const video = view.container.querySelector("video")!;
  Object.defineProperty(video, "readyState", {
    get: () => 2,
    configurable: true,
  });
  fireEvent.loadedMetadata(video);
  expect(video.currentTime).toBe(1);
  view.rerender(
    <TimelineThumbnail media={source} kind="video" startMS={2000} />,
  );
  expect(video.currentTime).toBe(2);
  view.rerender(<TimelineThumbnail media={source} kind="video" startMS={0} />);
  expect(video.currentTime).toBe(0);
});
it("waits for metadata then applies the latest pending seek after authorization refresh", () => {
  const source = media();
  const view = render(
    <TimelineThumbnail media={source} kind="video" startMS={1000} />,
  );
  const video = view.container.querySelector("video")!;
  expect(video.currentTime).toBe(0);
  const replacement = { ...source, url: "https://media.example/refreshed.mp4" };
  view.rerender(
    <TimelineThumbnail media={replacement} kind="video" startMS={3000} />,
  );
  expect(video.currentTime).toBe(0);
  Object.defineProperty(video, "readyState", {
    get: () => 1,
    configurable: true,
  });
  fireEvent.loadedMetadata(video);
  expect(video.currentTime).toBe(3);
  video.currentTime = 0;
  fireEvent.loadedData(video);
  expect(video.currentTime).toBe(3);
});
it("keeps the source under StrictMode and clamps the visual seek to available duration", () => {
  const source = media();
  const view = render(
    <StrictMode>
      <TimelineThumbnail media={source} kind="video" startMS={10000} />
    </StrictMode>,
  );
  const video = view.container.querySelector("video")!;
  Object.defineProperty(video, "readyState", {
    get: () => 2,
    configurable: true,
  });
  Object.defineProperty(video, "duration", {
    get: () => 7,
    configurable: true,
  });
  fireEvent.loadedData(video);
  expect(video.getAttribute("src")).toBe(source.url);
  expect(video.currentTime).toBe(7);
});
it("streams large compressed audio without the old 32 MiB gate", async () => {
  const release = vi.fn();
  waveform.acquire.mockReturnValue({
    promise: Promise.resolve(new Array(128).fill(1)),
    release,
  });
  const source = media("audio", "large");
  source.asset.byte_size = 1024 * 1024 * 1024;
  source.asset.duration_ms = 36000000;
  const view = render(<TimelineThumbnail media={source} kind="audio" />);
  await act(async () => {});
  expect(waveform.acquire).toHaveBeenCalledWith(source.url, 36000000);
  expect(
    view.container.querySelectorAll("svg.timeline-waveform path"),
  ).toHaveLength(128);
  view.unmount();
  expect(release).toHaveBeenCalledOnce();
});
it("preserves source-wide amplitude when displaying a trimmed subsection", async () => {
  const values = [...new Array(64).fill(1), ...new Array(64).fill(0.5)];
  waveform.acquire.mockImplementation(() => ({
    promise: Promise.resolve(values),
    release: vi.fn(),
  }));
  const source = media("audio");
  source.asset.duration_ms = 4000;
  const card = render(<TimelineThumbnail media={source} kind="audio" />);
  const crop = render(
    <TimelineThumbnail
      media={source}
      kind="audio"
      startMS={2000}
      durationMS={2000}
    />,
  );
  await act(async () => {});
  expect(card.container.querySelector("path")?.getAttribute("d")).toBe(
    "M1 2V48",
  );
  expect(crop.container.querySelector("path")?.getAttribute("d")).toBe(
    "M1 13.5V36.5",
  );
});
it("retries a failed waveform for explicit reload of the same URL", async () => {
  waveform.acquire
    .mockImplementationOnce(() => ({
      promise: Promise.reject(new Error("unsupported")),
      release: vi.fn(),
    }))
    .mockImplementation(() => ({
      promise: Promise.resolve(new Array(128).fill(1)),
      release: vi.fn(),
    }));
  const source = media("audio");
  const view = render(<TimelineThumbnail media={source} kind="audio" />);
  await act(async () => {});
  expect(view.container.querySelector("svg.timeline-waveform")).toBeNull();
  view.rerender(
    <TimelineThumbnail
      media={{ ...source, retryGeneration: 1 }}
      kind="audio"
    />,
  );
  await act(async () => {});
  expect(view.container.querySelector("svg.timeline-waveform")).toBeTruthy();
});
it("releases the lease and ignores a late waveform after unmount", async () => {
  let resolve!: (values: number[]) => void;
  const release = vi.fn();
  waveform.acquire.mockReturnValue({
    promise: new Promise<number[]>((done) => {
      resolve = done;
    }),
    release,
  });
  const view = render(
    <TimelineThumbnail media={media("audio")} kind="audio" />,
  );
  view.unmount();
  await act(async () => resolve(new Array(128).fill(1)));
  expect(release).toHaveBeenCalledOnce();
});
it("renders silence without invalid SVG coordinates", async () => {
  waveform.acquire.mockReturnValue({
    promise: Promise.resolve(new Array(128).fill(0)),
    release: vi.fn(),
  });
  const view = render(
    <TimelineThumbnail media={media("audio")} kind="audio" />,
  );
  await act(async () => {});
  const paths = Array.from(
    view.container.querySelectorAll("svg.timeline-waveform path"),
  );
  expect(paths).toHaveLength(128);
  expect(paths[0].getAttribute("d")).toBe("M1 25V25");
  expect(
    paths.every((path) => !path.getAttribute("d")?.match(/NaN|Infinity/)),
  ).toBe(true);
});
it("keeps image cards unchanged while repeating intrinsic-ratio track images at narrow and wide widths", () => {
  const source = media("image");
  const view = render(
    <div style={{ width: 30, height: 56 }}>
      <TimelineThumbnail media={source} kind="image" strip />
    </div>,
  );
  const strip = view.container.querySelector<HTMLElement>(
    "[data-thumbnail-strip]",
  )!;
  expect(strip.style.backgroundSize).toBe("auto 100%");
  expect(strip.style.backgroundRepeat).toBe("repeat-x");
  expect(strip.style.backgroundImage).toContain(source.url);
  view.rerender(
    <div style={{ width: 720, height: 56 }}>
      <TimelineThumbnail media={source} kind="image" strip />
    </div>,
  );
  expect(
    view.container.querySelectorAll("[data-thumbnail-strip]"),
  ).toHaveLength(1);
  expect(
    view.container.querySelector<HTMLElement>("[data-thumbnail-strip]")!.style
      .backgroundSize,
  ).toBe("auto 100%");
  view.rerender(<TimelineThumbnail media={source} kind="image" />);
  expect(view.container.querySelector("img")?.src).toBe(source.url);
  expect(view.container.querySelector("[data-thumbnail-strip]")).toBeNull();
});
