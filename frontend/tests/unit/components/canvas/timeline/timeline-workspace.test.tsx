import {
  act,
  cleanup,
  fireEvent,
  render,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CanvasTimelineWorkspace } from "@/components/canvas/timeline/canvas-timeline-workspace";
import { TimelinePreview } from "@/components/canvas/timeline/timeline-preview";
import { TimelineUploadDialog } from "@/components/canvas/timeline/timeline-upload-dialog";
import type {
  CanvasYjsLiveSession,
  YjsView,
} from "@/components/canvas/yjs-live-session";
import {
  editTimeline,
  emptyTimelineConfig,
  type Timeline,
} from "@/components/canvas/timeline/timeline-model";
vi.mock("next/dynamic", () => ({ default: () => () => null }));
vi.mock("@/components/canvas/timeline/use-timeline-media", () => ({
  useTimelineMedia: () => ({
    media: {},
    errors: {},
    error: "",
    retry: vi.fn(),
  }),
}));
vi.mock("@/components/canvas/timeline/timeline-preview", () => ({
  TimelinePreview: vi.fn(() => null),
}));
vi.mock("@/components/canvas/timeline/timeline-thumbnail", () => ({
  TimelineThumbnail: () => null,
}));
vi.mock("@/components/canvas/timeline/timeline-export", () => ({
  TimelineExport: () => null,
}));
vi.mock("@/components/canvas/timeline/timeline-aspect-select", () => ({
  TimelineAspectSelect: () => null,
}));
vi.mock("@/components/canvas/timeline/timeline-upload-dialog", () => ({
  TimelineUploadDialog: vi.fn(() => <div data-testid="batch-upload" />),
}));
beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.spyOn(HTMLElement.prototype, "setPointerCapture").mockImplementation(
    () => {},
  );
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  Reflect.deleteProperty(document, "fullscreenElement");
  Reflect.deleteProperty(document, "exitFullscreen");
  Reflect.deleteProperty(HTMLElement.prototype, "requestFullscreen");
});
function setup(audio = false, emptyAudioLane = true, compact = false) {
  let timeline = editTimeline(emptyTimelineConfig(), {
    type: "add",
    assets: [
      {
        asset: {
          id: crypto.randomUUID(),
          kind: audio ? "audio" : "video",
          duration_ms: 4000,
          file_name: "测试片段",
        },
      },
    ],
  });
  if (audio && emptyAudioLane)
    timeline = editTimeline(timeline, { type: "add-track", kind: "audio" });
  const nodeID = crypto.randomUUID();
  let epoch = "epoch-1";
  let view = {
    writable: true,
    migration: false,
    document: {
      nodes: [
        { id: nodeID, type: "timeline", serviceData: { config: { timeline } } },
      ],
      connections: [],
    },
  } as unknown as YjsView;
  const listeners = new Set<() => void>();
  const client = {
    subscribe: (fn: () => void) => {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
    getSnapshot: () => view,
    getDocumentEpoch: () => epoch,
    retainedTimeline: () => null,
    timelineHistoryState: () => ({ canUndo: true, canRedo: true }),
    timelineHistory: vi.fn(),
    changeTimeline: vi.fn((_id: string, next: Timeline) => {
      if (!view.writable) throw new Error("不可编辑");
      timeline = next;
      publish();
    }),
  };
  function publish(writable = view.writable) {
    view = {
      ...view,
      writable,
      document: {
        ...view.document!,
        nodes: view.document!.nodes.map((n) => ({
          ...n,
          serviceData: { ...n.serviceData!, config: { timeline } },
        })),
      },
    };
    listeners.forEach((fn) => fn());
  }
  const ui = render(
    <CanvasTimelineWorkspace
      client={client as unknown as CanvasYjsLiveSession}
      nodeID={nodeID}
      canvasID="canvas"
      projectID="project"
      compact={compact}
    />,
  );
  const clip = ui.getByRole("button", { name: /测试片段，/ });
  const shell = clip.closest<HTMLElement>(".timeline-clip-shell")!;
  fireEvent.click(clip);
  return {
    ui,
    client,
    nodeID,
    clip,
    shell,
    read: () => timeline,
    replaceTimeline: (next: Timeline) => {
      timeline = next;
      publish();
    },
    setWritable: (value: boolean) => act(() => publish(value)),
    replaceEpoch: () => {
      epoch = "epoch-2";
    },
  };
}
it("disables deletion after the selected clip has been removed", () => {
  const f = setup();
  const remove = f.ui.getByRole("button", { name: "删除" });
  fireEvent.click(remove);
  expect(f.read().clips).toHaveLength(0);
  expect((remove as HTMLButtonElement).disabled).toBe(true);
  f.client.changeTimeline.mockClear();
  fireEvent.click(remove);
  expect(f.client.changeTimeline).not.toHaveBeenCalled();
});
it("keeps one dropped batch open through multiple completions and exposes invalid files", async () => {
  const f = setup();
  const files = [
    new File(["video"], "a.mp4", { type: "video/mp4" }),
    new File(["invalid"], "notes.txt", { type: "text/plain" }),
  ];
  fireEvent.drop(f.ui.container.querySelector(".timeline-assets")!, {
    dataTransfer: { files, getData: () => "" },
  });
  const latest = () => vi.mocked(TimelineUploadDialog).mock.calls.at(-1)![0];
  expect(latest().initialFiles).toEqual(files);
  expect(latest().isCurrent?.()).toBe(true);
  for (let index = 0; index < 2; index++) {
    await act(async () =>
      latest().onUploaded(
        {
          id: crypto.randomUUID(),
          project_id: "project",
          kind: "image",
          file_name: `图片${index}.png`,
        } as API.AssetSummary,
        { signal: new AbortController().signal },
      ),
    );
    expect(f.ui.getByTestId("batch-upload")).toBeTruthy();
  }
  expect(f.read().imported_assets).toHaveLength(3);
  expect(f.read().clips).toHaveLength(1);
  act(() => latest().onClose());
  expect(f.ui.queryByTestId("batch-upload")).toBeNull();
});
it("rejects a dropped batch receipt after permission or epoch changes", async () => {
  const f = setup();
  fireEvent.drop(f.ui.container.querySelector(".timeline-assets")!, {
    dataTransfer: { files: [new File(["image"], "a.png")], getData: () => "" },
  });
  const latest = () => vi.mocked(TimelineUploadDialog).mock.calls.at(-1)![0];
  const asset = {
    id: crypto.randomUUID(),
    project_id: "project",
    kind: "image",
  } as API.AssetSummary;
  f.setWritable(false);
  expect(latest().isCurrent?.()).toBe(false);
  await expect(
    latest().onUploaded(asset, { signal: new AbortController().signal }),
  ).rejects.toThrow("不可写入");
  f.setWritable(true);
  f.replaceEpoch();
  expect(latest().isCurrent?.()).toBe(false);
  await act(async () => {
    await expect(
      latest().onUploaded(asset, { signal: new AbortController().signal }),
    ).rejects.toThrow("恢复到其他版本");
  });
  expect(f.client.changeTimeline).not.toHaveBeenCalled();
});
it("routes undo from a focused timeline slider to this node and blocks read-only undo", () => {
  const f = setup();
  const slider = f.ui.getByRole("slider", { name: /右边缘裁剪/ });
  fireEvent.keyDown(slider, { key: "z", ctrlKey: true });
  expect(f.client.timelineHistory).toHaveBeenCalledWith(f.nodeID, "undo");
  f.setWritable(false);
  f.client.timelineHistory.mockClear();
  fireEvent.keyDown(f.clip, { key: "z", metaKey: true });
  expect(f.client.timelineHistory).not.toHaveBeenCalled();
});
it("previews trim without saving and cancels on Escape, pointer cancellation and permission loss", () => {
  const f = setup();
  const initial = f.shell.style.width;
  for (const cancellation of ["Escape", "pointer", "permission"]) {
    const handle = f.ui.getByRole("slider", { name: /右边缘裁剪/ });
    fireEvent.pointerDown(handle, { clientX: 100, pointerId: 1 });
    fireEvent.pointerMove(handle, { clientX: 78.6, pointerId: 1 });
    expect(f.shell.style.width).not.toBe(initial);
    expect(f.client.changeTimeline).not.toHaveBeenCalled();
    if (cancellation === "Escape") fireEvent.keyDown(handle, { key: "Escape" });
    else if (cancellation === "pointer")
      fireEvent.pointerCancel(handle, { pointerId: 1 });
    else f.setWritable(false);
    expect(f.shell.style.width).toBe(initial);
    expect(f.client.changeTimeline).not.toHaveBeenCalled();
  }
});
it("commits one scaled trim and refuses a gesture captured before epoch replacement", () => {
  const f = setup();
  const root = f.clip.closest<HTMLElement>(".canvas-timeline")!;
  Object.defineProperty(root, "offsetWidth", {
    value: 600,
    configurable: true,
  });
  vi.spyOn(root, "getBoundingClientRect").mockReturnValue({
    width: 300,
  } as DOMRect);
  let handle = f.ui.getByRole("slider", { name: /右边缘裁剪/ });
  fireEvent.pointerDown(handle, { clientX: 100, pointerId: 1 });
  fireEvent.pointerMove(handle, { clientX: 89.3, pointerId: 1 });
  fireEvent.pointerUp(handle, { clientX: 89.3, pointerId: 1 });
  expect(f.client.changeTimeline).toHaveBeenCalledTimes(1);
  expect(f.read().clips[0].duration_ms).toBe(3000);
  handle = f.ui.getByRole("slider", { name: /右边缘裁剪/ });
  fireEvent.pointerDown(handle, { clientX: 100, pointerId: 2 });
  f.replaceEpoch();
  fireEvent.pointerUp(handle, { clientX: 89.3, pointerId: 2 });
  expect(f.client.changeTimeline).toHaveBeenCalledTimes(1);
  expect(f.ui.getByText(/画布已恢复到其他版本/)).toBeTruthy();
});
it("moves audio into the hovered audio lane using canvas-scaled pointer coordinates", () => {
  const f = setup(true);
  const root = f.clip.closest<HTMLElement>(".canvas-timeline")!;
  Object.defineProperty(root, "offsetWidth", {
    value: 600,
    configurable: true,
  });
  vi.spyOn(root, "getBoundingClientRect").mockReturnValue({
    width: 300,
  } as DOMRect);
  const lanes = Array.from(
    root.querySelectorAll<HTMLElement>('.timeline-lane[data-kind="audio"]'),
  );
  lanes.forEach((lane, index) =>
    vi.spyOn(lane, "getBoundingClientRect").mockReturnValue({
      top: 100 + index * 50,
      bottom: 132 + index * 50,
    } as DOMRect),
  );
  const destination = lanes[1].dataset.trackId;
  fireEvent.pointerDown(f.clip, { clientX: 100, clientY: 110, pointerId: 1 });
  fireEvent.pointerMove(f.clip, { clientX: 110.7, clientY: 160, pointerId: 1 });
  fireEvent.pointerUp(f.clip, { clientX: 110.7, clientY: 160, pointerId: 1 });
  expect(f.read().clips[0]).toMatchObject({
    track_id: destination,
    start_ms: 1000,
  });
  expect(f.client.changeTimeline).toHaveBeenCalledTimes(1);
});

it("addresses the playhead in one-second keyboard steps within the real duration and keeps node history", () => {
  const f = setup();
  const playhead = f.ui.getByRole("slider", { name: "时间线播放头" });
  expect(playhead.getAttribute("aria-valuemin")).toBe("0");
  expect(playhead.getAttribute("aria-valuemax")).toBe("4000");
  expect(playhead.getAttribute("aria-valuenow")).toBe("0");
  fireEvent.keyDown(playhead, { key: "ArrowRight" });
  expect(playhead.getAttribute("aria-valuenow")).toBe("1000");
  expect(playhead.getAttribute("aria-valuetext")).toBe("00:01:00");
  fireEvent.keyDown(playhead, { key: "ArrowRight" });
  expect(playhead.getAttribute("aria-valuenow")).toBe("2000");
  fireEvent.keyDown(playhead, { key: "ArrowLeft" });
  expect(playhead.getAttribute("aria-valuenow")).toBe("1000");
  fireEvent.keyDown(playhead, { key: "End" });
  expect(playhead.getAttribute("aria-valuenow")).toBe("4000");
  fireEvent.keyDown(playhead, { key: "ArrowRight" });
  expect(playhead.getAttribute("aria-valuenow")).toBe("4000");
  fireEvent.keyDown(playhead, { key: "Home" });
  fireEvent.keyDown(playhead, { key: "ArrowLeft" });
  expect(playhead.getAttribute("aria-valuenow")).toBe("0");
  fireEvent.keyDown(playhead, { key: "z", metaKey: true });
  fireEvent.keyDown(playhead, { key: "z", ctrlKey: true, shiftKey: true });
  expect(f.client.timelineHistory.mock.calls).toEqual([
    [f.nodeID, "undo"],
    [f.nodeID, "redo"],
  ]);
  expect(f.client.changeTimeline).not.toHaveBeenCalled();
});

it.each([6, 25])(
  "creates an audio lane only within the narrow boundary: %d px below",
  (offset) => {
    const f = setup(true, false);
    const root = f.clip.closest<HTMLElement>(".canvas-timeline")!;
    const original = structuredClone(f.read());
    const lanes = Array.from(
      root.querySelectorAll<HTMLElement>(".timeline-lane"),
    );
    lanes.forEach((lane, index) =>
      vi.spyOn(lane, "getBoundingClientRect").mockReturnValue({
        top: index ? 100 : 0,
        bottom: index ? 132 : 56,
      } as DOMRect),
    );
    fireEvent.pointerDown(f.clip, { clientX: 100, clientY: 110, pointerId: 1 });
    fireEvent.pointerMove(f.clip, {
      clientX: 100,
      clientY: 132 + offset,
      pointerId: 1,
    });
    fireEvent.pointerUp(f.clip, {
      clientX: 100,
      clientY: 132 + offset,
      pointerId: 1,
    });
    expect(f.ui.queryByRole("button", { name: "添加音频轨" })).toBeNull();
    if (offset === 6) {
      expect(f.client.changeTimeline).toHaveBeenCalledTimes(1);
      expect(f.read().clips[0].track_id).not.toBe(original.clips[0].track_id);
      expect(f.read().clips[0].start_ms).toBe(0);
      expect(f.read().tracks).toHaveLength(2);
      expect(f.read().tracks[0]).toEqual(original.tracks[0]);
    } else {
      expect(f.client.changeTimeline).not.toHaveBeenCalled();
      expect(f.read()).toEqual(original);
    }
  },
);
it("does not start an audio boundary creation gesture while read-only", () => {
  const f = setup(true, false);
  const root = f.clip.closest<HTMLElement>(".canvas-timeline")!;
  const original = structuredClone(f.read());
  const lane = root.querySelector<HTMLElement>(
    '.timeline-lane[data-kind="audio"]',
  )!;
  vi.spyOn(lane, "getBoundingClientRect").mockReturnValue({
    top: 100,
    bottom: 132,
  } as DOMRect);
  f.setWritable(false);
  fireEvent.pointerDown(f.clip, { clientX: 100, clientY: 110, pointerId: 1 });
  fireEvent.pointerMove(f.clip, { clientX: 100, clientY: 138, pointerId: 1 });
  fireEvent.pointerUp(f.clip, { clientX: 100, clientY: 138, pointerId: 1 });
  expect(f.client.changeTimeline).not.toHaveBeenCalled();
  expect(f.read()).toEqual(original);
});

function fullscreenAPI() {
  let element: Element | null = null;
  const change = (next: Element | null) => {
    element = next;
    fireEvent(document, new Event("fullscreenchange"));
  };
  const request = vi.fn(async function (this: HTMLElement) {
    change(this);
  });
  const exit = vi.fn(async () => {
    change(null);
  });
  Object.defineProperty(document, "fullscreenElement", {
    configurable: true,
    get: () => element,
  });
  Object.defineProperty(document, "exitFullscreen", {
    configurable: true,
    value: exit,
  });
  Object.defineProperty(HTMLElement.prototype, "requestFullscreen", {
    configurable: true,
    value: request,
  });
  return { request, exit, change };
}
it("toggles player fullscreen through the real API contract and reflects external fullscreen exit", async () => {
  const api = fullscreenAPI();
  const f = setup();
  await act(async () => {
    fireEvent.click(f.ui.getByRole("button", { name: "全屏播放" }));
  });
  expect(api.request).toHaveBeenCalledTimes(1);
  const exit = f.ui.getByRole("button", { name: "退出全屏播放" });
  expect(exit.getAttribute("aria-pressed")).toBe("true");
  await act(async () => {
    fireEvent.click(exit);
  });
  expect(api.exit).toHaveBeenCalledTimes(1);
  expect(
    f.ui.getByRole("button", { name: "全屏播放" }).getAttribute("aria-pressed"),
  ).toBe("false");
  await act(async () => {
    fireEvent.click(f.ui.getByRole("button", { name: "全屏播放" }));
  });
  act(() => api.change(null));
  expect(f.ui.getByRole("button", { name: "全屏播放" })).toBeTruthy();
  expect(f.client.changeTimeline).not.toHaveBeenCalled();
});
it.each(["before", "after"])(
  "keeps the editor open when Escape arrives %s fullscreenchange",
  async (order) => {
    const api = fullscreenAPI();
    const f = setup(false, true, true);
    fireEvent.click(f.ui.getByRole("button", { name: "全屏编辑" }));
    const dialog = f.ui.getByRole("dialog", { name: "时间线" });
    await act(async () => {
      fireEvent.click(within(dialog).getByRole("button", { name: "全屏播放" }));
    });
    if (order === "after") act(() => api.change(null));
    fireEvent.keyDown(document, { key: "Escape", code: "Escape" });
    expect(f.ui.queryByRole("dialog", { name: "时间线" })).not.toBeNull();
    expect(api.exit).toHaveBeenCalledTimes(order === "before" ? 1 : 0);
    expect(document.fullscreenElement).toBeNull();
    expect(
      within(dialog).getByRole("button", { name: "全屏播放" }),
    ).toBeTruthy();
    const later = performance.now() + 500;
    vi.spyOn(performance, "now").mockReturnValue(later);
    fireEvent.keyDown(document, { key: "Escape", code: "Escape" });
    expect(f.ui.queryByRole("dialog", { name: "时间线" })).toBeNull();
  },
);
it("keeps normal editor state after a denied fullscreen request", async () => {
  const api = fullscreenAPI();
  api.request.mockRejectedValueOnce(new Error("not allowed"));
  const f = setup();
  await act(async () => {
    fireEvent.click(f.ui.getByRole("button", { name: "全屏播放" }));
  });
  expect(f.ui.getByText("浏览器未允许全屏播放")).toBeTruthy();
  expect(f.ui.queryByRole("button", { name: "退出全屏播放" })).toBeNull();
});

it("releases owned fullscreen state when its workspace unmounts", async () => {
  const api = fullscreenAPI();
  const f = setup();
  await act(async () => {
    fireEvent.click(f.ui.getByRole("button", { name: "全屏播放" }));
  });
  f.ui.unmount();
  expect(api.exit).toHaveBeenCalledTimes(1);
  expect(document.fullscreenElement).toBeNull();
});

it.each(["button", "keyboard"])(
  "restores volume slider and playback gain after undoing a keyboard volume commit via %s",
  (mode) => {
    const f = setup(true, false);
    const original = structuredClone(f.read());
    const slider = f.ui.getByRole("slider", { name: "片段音量" });
    fireEvent.keyDown(slider, { key: "Home" });
    expect(f.read().clips[0].volume).toBe(0);
    expect(slider.getAttribute("aria-valuenow")).toBe("0");
    f.client.timelineHistory.mockImplementation(() =>
      f.replaceTimeline(original),
    );
    if (mode === "keyboard")
      fireEvent.keyDown(slider, { key: "z", ctrlKey: true });
    else fireEvent.click(f.ui.getByRole("button", { name: "撤销" }));
    expect(f.ui.getByRole("button", { name: "静音片段" })).toBeTruthy();
    expect(slider.getAttribute("aria-valuenow")).toBe("100");
    expect(
      vi.mocked(TimelinePreview).mock.calls.at(-1)![0].timeline.clips[0].volume,
    ).toBe(1);
  },
);
it("discards a stale volume draft when a collaborator changes the canonical timeline", () => {
  const f = setup(true, false);
  const slider = f.ui.getByRole("slider", { name: "片段音量" });
  fireEvent.keyDown(slider, { key: "Home" });
  const remote = editTimeline(f.read(), {
    type: "clip",
    id: f.read().clips[0].id,
    patch: { volume: 0.7 },
  });
  act(() => f.replaceTimeline(remote));
  expect(slider.getAttribute("aria-valuenow")).toBe("70");
  expect(
    vi.mocked(TimelinePreview).mock.calls.at(-1)![0].timeline.clips[0].volume,
  ).toBe(0.7);
});
it("updates major labels and minor ruler spacing when zoom changes", () => {
  const f = setup();
  const ruler = f.ui.container.querySelector<HTMLElement>(".timeline-ruler")!;
  const zoom = f.ui.getByRole("slider", { name: "时间线缩放" });
  expect(ruler.querySelectorAll("span")[1].textContent).toBe("00:05");
  fireEvent.keyDown(zoom, { key: "End" });
  expect(ruler.querySelectorAll("span")[1].textContent).toBe("00:01");
  expect(ruler.style.backgroundSize).toBe("18px 5px");
  fireEvent.keyDown(zoom, { key: "Home" });
  expect(ruler.querySelectorAll("span")[1].textContent).toBe("00:05");
  expect(ruler.style.backgroundSize).toBe("14px 5px");
});
it("aligns first-video trim headroom across lanes, ruler and playhead and restores on undo", () => {
  const f = setup();
  const original = structuredClone(f.read());
  const trimmed = editTimeline(original, {
    type: "resize",
    id: original.clips[0].id,
    edge: "left",
    delta: 1000,
  });
  act(() => f.replaceTimeline(trimmed));
  const root = f.ui.container;
  expect(
    parseFloat(
      root.querySelector<HTMLElement>(".timeline-lane")!.style.marginLeft,
    ) +
      parseFloat(
        root.querySelector<HTMLElement>(".timeline-track-control")!.style.width,
      ),
  ).toBeCloseTo(73.4);
  expect(
    root.querySelector<HTMLElement>(".timeline-ruler")!.style.marginLeft,
  ).toBe("73.4px");
  expect(
    root.querySelector<HTMLElement>(".timeline-playhead-slider")!.style.left,
  ).toBe("73.4px");
  expect(
    root.querySelector<HTMLElement>(".timeline-tracks-scroll")!.scrollLeft,
  ).toBeCloseTo(21.4);
  act(() => f.replaceTimeline(original));
  expect(
    root.querySelector<HTMLElement>(".timeline-lane")!.style.marginLeft,
  ).toBe("8px");
  expect(
    root.querySelector<HTMLElement>(".timeline-ruler")!.style.marginLeft,
  ).toBe("52px");
  expect(
    root.querySelector<HTMLElement>(".timeline-tracks-scroll")!.scrollLeft,
  ).toBe(0);
});
it("includes viewport scrolling in trim distance and cancels on lost capture", () => {
  const f = setup();
  const trimmed = editTimeline(f.read(), {
    type: "resize",
    id: f.read().clips[0].id,
    edge: "left",
    delta: 1000,
  });
  act(() => f.replaceTimeline(trimmed));
  const scroll = f.ui.container.querySelector<HTMLElement>(
    ".timeline-tracks-scroll",
  )!;
  const handle = f.ui.getByRole("slider", { name: /左边缘裁剪/ });
  fireEvent.pointerDown(handle, { clientX: 150, pointerId: 31 });
  scroll.scrollLeft = 10.7;
  fireEvent.pointerMove(handle, { clientX: 150, pointerId: 31 });
  fireEvent.pointerUp(handle, { clientX: 150, pointerId: 31 });
  expect(f.read().clips[0].source_start_ms).toBe(500);
  expect(f.read().clips[0].duration_ms).toBe(3500);
  const count = f.client.changeTimeline.mock.calls.length;
  fireEvent.pointerDown(handle, { clientX: 150, pointerId: 32 });
  fireEvent.pointerMove(handle, { clientX: 160, pointerId: 32 });
  fireEvent.lostPointerCapture(handle, { pointerId: 32 });
  fireEvent.pointerUp(handle, { clientX: 160, pointerId: 32 });
  expect(f.client.changeTimeline).toHaveBeenCalledTimes(count);
});
