import { StrictMode } from "react";
import { act, cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TimelinePreview } from "@/components/canvas/timeline/timeline-preview";
import {
  emptyTimelineConfig,
  editTimeline,
} from "@/components/canvas/timeline/timeline-model";
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
function fixture() {
  const asset: API.AssetSummary = {
    id: crypto.randomUUID(),
    kind: "video",
    duration_ms: 10000,
    width: 640,
    height: 480,
  };
  let timeline = editTimeline(emptyTimelineConfig(), {
    type: "add",
    assets: [{ asset }],
  });
  timeline = editTimeline(timeline, {
    type: "trim-left",
    id: timeline.clips[0].id,
    at: 2000,
  });
  timeline = editTimeline(timeline, {
    type: "clip",
    id: timeline.clips[0].id,
    patch: { volume: 0.5 },
  });
  timeline.aspect_ratio = "9:16";
  return {
    timeline,
    media: { [asset.id!]: { asset, url: "https://media.example/video.mp4" } },
    playhead: 2500,
    playing: true,
    muted: false,
    volume: 0.8,
    onError: vi.fn(),
  };
}
it("seeks to the trimmed source offset, applies clip gain and track mute, and excludes hidden tracks", () => {
  const play = vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
  const props = fixture();
  const { container, rerender } = render(<TimelinePreview {...props} />);
  const video = container.querySelector("video")!;
  expect(video.currentTime).toBe(4.5);
  expect(video.volume).toBeCloseTo(0.4);
  expect(play).toHaveBeenCalled();
  const muted = editTimeline(props.timeline, {
    type: "track",
    id: props.timeline.tracks[0].id,
    patch: { muted: true, locked: true },
  });
  rerender(<TimelinePreview {...props} timeline={muted} playing={false} />);
  expect(video.muted).toBe(true);
  expect(container.querySelector("video")).not.toBeNull();
  const hidden = editTimeline(muted, {
    type: "track",
    id: muted.tracks[0].id,
    patch: { visible: false },
  });
  rerender(<TimelinePreview {...props} timeline={hidden} />);
  expect(container.querySelector("video")).toBeNull();
});
it("shows an explicit portrait frame and only uses the observed 16:9 auto fallback while empty", () => {
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
  const props = fixture();
  const { getByLabelText, rerender } = render(<TimelinePreview {...props} />);
  expect(parseFloat(getByLabelText("时间线预览画面").style.aspectRatio)).toBe(
    1080 / 1920,
  );
  rerender(
    <TimelinePreview
      {...props}
      timeline={{ ...props.timeline, aspect_ratio: "auto" }}
    />,
  );
  expect(parseFloat(getByLabelText("时间线预览画面").style.aspectRatio)).toBe(
    640 / 480,
  );
  rerender(<TimelinePreview {...props} timeline={emptyTimelineConfig()} />);
  expect(parseFloat(getByLabelText("时间线预览画面").style.aspectRatio)).toBe(
    16 / 9,
  );
});

it("seeks every paused frame even when the movement is smaller than playback drift tolerance", () => {
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
  const props = { ...fixture(), playing: false };
  const { container, rerender } = render(<TimelinePreview {...props} />);
  const video = container.querySelector("video")!;
  const start = video.currentTime;
  rerender(<TimelinePreview {...props} playhead={props.playhead + 33} />);
  expect(video.currentTime).toBeCloseTo(start + 0.033);
});

it("restores the trimmed seek position after a media authorization URL refresh", () => {
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
  const props = { ...fixture(), playing: false };
  const { container, rerender } = render(<TimelinePreview {...props} />);
  const video = container.querySelector("video")!;
  video.currentTime = 0;
  const [id, source] = Object.entries(props.media)[0];
  rerender(
    <TimelinePreview
      {...props}
      media={{
        [id]: { ...source, url: "https://media.example/refreshed.mp4" },
      }}
    />,
  );
  expect(video.currentTime).toBe(4.5);
});

it("shows black after the visual track ends while independent audio continues", () => {
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
  const image: API.AssetSummary = {
    id: crypto.randomUUID(),
    kind: "image",
    width: 640,
    height: 480,
  };
  const audio: API.AssetSummary = {
    id: crypto.randomUUID(),
    kind: "audio",
    duration_ms: 7000,
  };
  const timeline = editTimeline(emptyTimelineConfig(), {
    type: "add",
    assets: [{ asset: image }, { asset: audio }],
  });
  const { container, queryByText, getByLabelText } = render(
    <TimelinePreview
      timeline={timeline}
      media={{
        [image.id!]: { asset: image, url: "https://media.example/image.png" },
        [audio.id!]: { asset: audio, url: "https://media.example/audio.mp3" },
      }}
      playhead={6000}
      playing={false}
      muted={false}
      volume={1}
      onError={vi.fn()}
    />,
  );
  expect(container.querySelector("audio")).not.toBeNull();
  expect(queryByText("没有媒体可供预览")).toBeNull();
  expect(getByLabelText("时间线预览画面").className).toContain("bg-black");
});

it.each(["pause", "unmount"])(
  "ignores a stale play rejection after %s and pauses the media",
  async (action) => {
    let reject!: (reason: Error) => void;
    vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(
      () =>
        new Promise<void>((_, fail) => {
          reject = fail;
        }),
    );
    const pause = vi
      .spyOn(HTMLMediaElement.prototype, "pause")
      .mockImplementation(() => {});
    const props = fixture();
    const view = render(<TimelinePreview {...props} />);
    if (action === "pause")
      view.rerender(<TimelinePreview {...props} playing={false} />);
    else view.unmount();
    await act(async () => {
      reject(new Error("interrupted"));
    });
    expect(props.onError).not.toHaveBeenCalled();
    expect(pause).toHaveBeenCalled();
  },
);

it("does not issue duplicate play requests while the first start is pending", () => {
  const play = vi
    .spyOn(HTMLMediaElement.prototype, "play")
    .mockReturnValue(new Promise(() => {}));
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
  const props = fixture();
  const view = render(<TimelinePreview {...props} />);
  view.rerender(<TimelinePreview {...props} playhead={props.playhead + 33} />);
  view.rerender(<TimelinePreview {...props} playhead={props.playhead + 66} />);
  expect(play).toHaveBeenCalledTimes(1);
});

it("isolates a failed playing audio source without restarting it on every clock tick", () => {
  const play = vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
  const pause = vi
    .spyOn(HTMLMediaElement.prototype, "pause")
    .mockImplementation(() => {});
  const audio = (): API.AssetSummary => ({
    id: crypto.randomUUID(),
    kind: "audio",
    duration_ms: 7000,
  });
  const first = audio(),
    second = audio();
  let timeline = editTimeline(emptyTimelineConfig(), {
    type: "add",
    assets: [{ asset: first }],
  });
  timeline = editTimeline(timeline, { type: "add-track", kind: "audio" });
  timeline = editTimeline(timeline, {
    type: "add",
    assets: [{ asset: second }],
    trackID: timeline.tracks.at(-1)!.id,
  });
  const props = {
    timeline,
    media: {
      [first.id!]: { asset: first, url: "https://media.example/one.mp3" },
      [second.id!]: { asset: second, url: "https://media.example/two.mp3" },
    },
    playhead: 1000,
    playing: true,
    muted: false,
    volume: 1,
    onError: vi.fn(),
  };
  const view = render(<TimelinePreview {...props} />);
  const [bad, good] = Array.from(view.container.querySelectorAll("audio"));
  fireEvent.error(bad);
  const calls = play.mock.calls.length;
  view.rerender(<TimelinePreview {...props} playhead={1033} />);
  expect(play).toHaveBeenCalledTimes(calls);
  expect(view.container.contains(good)).toBe(true);
  expect(pause.mock.instances).not.toContain(good);
  expect(props.onError).toHaveBeenCalledTimes(1);
  expect(props.onError).toHaveBeenCalledWith(
    `无法播放：${timeline.clips[0].title}`,
    props.media[first.id!],
  );
});

it("retries a failed media element when the user reloads the same authorized URL", () => {
  const play = vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
  const props = fixture();
  const view = render(<TimelinePreview {...props} />);
  fireEvent.error(view.container.querySelector("video")!);
  const count = play.mock.calls.length;
  const [id, source] = Object.entries(props.media)[0];
  view.rerender(
    <TimelinePreview
      {...props}
      media={{ [id]: { ...source, retryGeneration: 1 } }}
    />,
  );
  expect(play).toHaveBeenCalledTimes(count + 1);
});

it("combines preview volume, clip gain and fades while muting only the selected track", () => {
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
  const props = fixture();
  const audio: API.AssetSummary = {
    id: crypto.randomUUID(),
    kind: "audio",
    duration_ms: 7000,
  };
  let timeline = editTimeline(props.timeline, {
    type: "add",
    assets: [{ asset: audio }],
  });
  timeline = editTimeline(timeline, {
    type: "clip",
    id: timeline.clips[1].id,
    patch: { volume: 0.5, fade_in_ms: 2000 },
  });
  timeline = editTimeline(timeline, {
    type: "track",
    id: timeline.clips[0].track_id,
    patch: { muted: true },
  });
  const { container } = render(
    <TimelinePreview
      {...props}
      timeline={timeline}
      media={{
        ...props.media,
        [audio.id!]: { asset: audio, url: "https://media.example/audio.mp3" },
      }}
      playhead={1000}
      volume={0.8}
    />,
  );
  expect(container.querySelector("video")!.muted).toBe(true);
  expect(container.querySelector("audio")!.muted).toBe(false);
  expect(container.querySelector("audio")!.volume).toBeCloseTo(0.2);
});

it("retains media sources after StrictMode effect replay and pauses on real unmount", () => {
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
  const pause = vi
    .spyOn(HTMLMediaElement.prototype, "pause")
    .mockImplementation(() => {});
  const props = { ...fixture(), playing: false };
  const audio: API.AssetSummary = {
    id: crypto.randomUUID(),
    kind: "audio",
    duration_ms: 7000,
  };
  const timeline = editTimeline(props.timeline, {
    type: "add",
    assets: [{ asset: audio }],
  });
  const view = render(
    <StrictMode>
      <TimelinePreview
        {...props}
        timeline={timeline}
        media={{
          ...props.media,
          [audio.id!]: { asset: audio, url: "https://media.example/sound.mp3" },
        }}
      />
    </StrictMode>,
  );
  const video = view.container.querySelector("video")!,
    sound = view.container.querySelector("audio")!;
  expect(video.getAttribute("src")).toBe("https://media.example/video.mp4");
  expect(sound.getAttribute("src")).toBe("https://media.example/sound.mp3");
  pause.mockClear();
  view.unmount();
  expect(pause.mock.instances).toContain(video);
  expect(pause.mock.instances).toContain(sound);
});
