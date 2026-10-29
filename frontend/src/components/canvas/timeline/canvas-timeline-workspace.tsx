"use client";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
  type ReactNode,
  type PointerEvent as ReactPointerEvent,
} from "react";
import dynamic from "next/dynamic";
import {
  Plus,
  Play,
  Pause,
  Maximize2,
  Minimize2,
  X,
  Undo2,
  Redo2,
  Trash2,
  Volume2,
  VolumeX,
  CircleMinus,
  CirclePlus,
  PanelLeftClose,
  PanelLeftOpen,
  Upload,
  FolderOpen,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Slider } from "@/components/ui/slider";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useTimelineViewport } from "./use-timeline-viewport";
import { timelineRulerIntervals } from "./timeline-ruler";
import { getMediaPreview } from "@/api/media";
import { type CanvasYjsLiveSession } from "../yjs-live-session";
import {
  editTimeline,
  readTimeline,
  timelineDuration,
  timelineTime,
  timelineRulerTime,
  timelineMoveSnap,
  type TimelineSnap,
  type Timeline,
  type TimelineEdit,
  type TimelineClip,
} from "./timeline-model";
import { TimelineAspectSelect } from "./timeline-aspect-select";
import { TimelinePreview, type TimelineMedia } from "./timeline-preview";
import { useTimelineMedia } from "./use-timeline-media";
import { TimelineExport } from "./timeline-export";
import { timelinePointerDelta, timelineElementScale } from "./timeline-gesture";
import { TimelineThumbnail } from "./timeline-thumbnail";
import { timelineResultGraph, type TimelineResult } from "./timeline-delivery";
import { timelineCanvasAssets } from "./timeline-canvas-assets";
import { TimelineUploadDialog } from "./timeline-upload-dialog";
import "./timeline.css";
const ProjectAssetDialog = dynamic(
  () =>
    import("@/components/media/project-asset-dialog").then(
      (m) => m.ProjectAssetDialog,
    ),
  { loading: () => <p role="status">正在读取资产…</p> },
);
function TimelineCutIcon({ trim }: { trim?: "left" | "right" }) {
  return (
    <svg
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      aria-hidden="true"
    >
      <path
        d="M3 2.5h3v11H3"
        strokeDasharray={trim === "left" ? "1.5 2" : undefined}
      />
      <path
        d="M13 2.5h-3v11h3"
        strokeDasharray={trim === "right" ? "1.5 2" : undefined}
      />
    </svg>
  );
}
function TimelineSnapIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true" fill="none">
      <path
        fill="currentColor"
        fillRule="evenodd"
        d="M2.66 5.33h10.68c.73 0 1.33.6 1.33 1.34v2.66c0 .74-.6 1.34-1.33 1.34H2.66c-.73 0-1.33-.6-1.33-1.34V6.67c0-.74.6-1.34 1.33-1.34ZM7.33 7.33v1.34a.67.67 0 0 0 1.34 0V7.33a.67.67 0 0 0-1.34 0Z"
      />
      <path
        stroke="currentColor"
        strokeWidth="1.33"
        strokeLinecap="round"
        d="M8 2v.67M4.67 2.94l.47.47M10.86 3.41l.47-.47M8 13.33V14M5.14 12.59l-.47.47M10.86 12.59l.47.47"
      />
    </svg>
  );
}
type Props = {
  client: CanvasYjsLiveSession;
  nodeID: string;
  canvasID: string;
  projectID: string;
  compact?: boolean;
  suspended?: boolean;
  onClose?: () => void;
  onExpand?: () => void;
};
export function CanvasTimelineWorkspace(props: Props) {
  const [expanded, setExpanded] = useState(false);
  const fullscreenGuard = useRef({ active: false, exitedAt: -Infinity });
  const onPlayerFullscreenChange = useCallback((active: boolean) => {
    if (fullscreenGuard.current.active && !active)
      fullscreenGuard.current.exitedAt = performance.now();
    fullscreenGuard.current.active = active;
  }, []);
  const key = `${props.projectID}/${props.canvasID}/${props.nodeID}`;
  if (!props.compact) return <TimelineWorkspace key={key} {...props} />;
  return (
    <>
      <TimelineWorkspace
        key={key}
        {...props}
        suspended={expanded}
        onExpand={() => {
          props.onExpand?.();
          if (!props.onExpand) setExpanded(true);
        }}
      />
      <Dialog open={expanded} onOpenChange={setExpanded}>
        <DialogContent
          data-canvas-no-zoom
          data-canvas-shortcuts-ignore
          onEscapeKeyDown={(event) => {
            // Browsers may exit fullscreen before delivering its Escape event.
            // Do not let that same key also dismiss the underlying editor.
            const guard = fullscreenGuard.current;
            if (guard.active) {
              event.preventDefault();
              // Preventing Radix dismissal can also consume the browser's
              // default Escape action, so explicitly leave our player.
              if (document.fullscreenElement)
                void document.exitFullscreen().catch(() => {});
            } else if (performance.now() - guard.exitedAt < 300) {
              event.preventDefault();
            }
          }}
          onMouseDown={(event) => event.stopPropagation()}
          showCloseButton={false}
          className="canvas-timeline-dialog top-0 left-0 max-h-none translate-x-0 translate-y-0"
        >
          <DialogTitle className="sr-only">时间线</DialogTitle>
          <DialogDescription className="sr-only">
            编辑视觉轨和音频轨
          </DialogDescription>
          <TimelineWorkspace
            key={key}
            {...props}
            compact={false}
            onPlayerFullscreenChange={onPlayerFullscreenChange}
            onClose={() => setExpanded(false)}
          />
        </DialogContent>
      </Dialog>
    </>
  );
}
function TimelineWorkspace({
  client,
  nodeID,
  canvasID,
  projectID,
  compact = false,
  suspended = false,
  onClose,
  onExpand,
  onPlayerFullscreenChange,
}: Props & { onPlayerFullscreenChange?: (active: boolean) => void }) {
  const view = useSyncExternalStore(
    client.subscribe,
    client.getSnapshot,
    client.getSnapshot,
  );
  const node = view.document?.nodes.find((item) => item.id === nodeID);
  const source = node?.serviceData?.config?.timeline;
  const { timeline, invalid } = useMemo(() => {
    try {
      return { timeline: readTimeline(source), invalid: "" };
    } catch (cause) {
      return {
        timeline: null,
        invalid: cause instanceof Error ? cause.message : "无法读取时间线",
      };
    }
  }, [source]);
  const readOnly = !view.writable || view.migration;
  const [playbackFailures, setPlaybackFailures] = useState<
    Record<
      string,
      {
        message: string;
        media: TimelineMedia;
      }
    >
  >({});
  const [error, setError] = useState(""),
    [selected, setSelected] = useState<string | null>(null),
    [position, setPlayhead] = useState(0),
    [playing, setPlaying] = useState(false),
    [playerFullscreen, setPlayerFullscreen] = useState(false),
    [muted, setMuted] = useState(true),
    [volume, setVolume] = useState(1),
    [clipVolume, setClipVolume] = useState<{
      id: string;
      value: number;
      baseline: Timeline;
    } | null>(null),
    [showAssets, setShowAssets] = useState(true),
    [assetWidth, setAssetWidth] = useState(360),
    [trackHeight, setTrackHeight] = useState(218),
    [assetSource, setAssetSource] = useState("imported"),
    [filter, setFilter] = useState("all"),
    [picker, setPicker] = useState<"library" | "upload" | null>(null),
    [pickerEpoch, setPickerEpoch] = useState<string | null>(null),
    [busy, setBusy] = useState(false),
    [dragOffset, setDragOffset] = useState<{
      id: string;
      px: number;
      py: number;
    } | null>(null),
    [trimPreview, setTrimPreview] = useState<Timeline | null>(null),
    [trimming, setTrimming] = useState(false),
    [dropAfter, setDropAfter] = useState<string | null>(null),
    [droppedFiles, setDroppedFiles] = useState<
      { id: string; file: File; trackID?: string; epoch: string | null }[]
    >([]),
    [targetTrack, setTargetTrack] = useState<string | undefined>(),
    [retainedEdit, setRetainedEdit] = useState<Timeline | null>(() =>
      client.retainedTimeline(nodeID),
    );
  const scope = `${projectID}/${canvasID}/${nodeID}`,
    identity = useRef(scope);
  useEffect(() => {
    identity.current = scope;
    return () => {
      identity.current = "";
    };
  }, [scope]);
  const requests = useRef(new Set<AbortController>()),
    adding = useRef(false),
    importingCanvas = useRef(false),
    preview = useRef<HTMLDivElement>(null);
  useEffect(() => {
    let element: HTMLDivElement | null = null;
    let owned = false;
    const sync = () => {
      const current = preview.current;
      const active = !!current && document.fullscreenElement === current;
      if (active) element = current;
      if (active === owned) return;
      owned = active;
      setPlayerFullscreen(active);
      onPlayerFullscreenChange?.(active);
    };
    document.addEventListener("fullscreenchange", sync);
    sync();
    return () => {
      document.removeEventListener("fullscreenchange", sync);
      if (owned) {
        onPlayerFullscreenChange?.(false);
        if (document.fullscreenElement === element)
          void document.exitFullscreen?.().catch(() => {});
      }
    };
  }, [onPlayerFullscreenChange]);
  const drag = useRef<{
    id: string;
    startX: number;
    startY: number;
    scale: number;
    zoom: number;
    start: number;
    baseline: Timeline;
    epoch: string | null;
    snap: TimelineSnap | null;
  } | null>(null);
  const seekGesture = useRef<{
    pointerID: number;
    element: HTMLDivElement;
    initial: number;
  } | null>(null);
  const documentEpoch = client.getDocumentEpoch();
  useEffect(() => {
    const cancel = () => {
      drag.current = null;
      setDragOffset(null);
      setDropAfter(null);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || !drag.current) return;
      // Radix dismisses at document capture; consume the active gesture first.
      event.preventDefault();
      event.stopPropagation();
      cancel();
    };
    window.addEventListener("keydown", escape, true);
    window.addEventListener("blur", cancel);
    document.addEventListener("visibilitychange", cancel);
    return () => {
      window.removeEventListener("keydown", escape, true);
      window.removeEventListener("blur", cancel);
      document.removeEventListener("visibilitychange", cancel);
      cancel();
    };
  }, [readOnly, suspended, timeline, documentEpoch]);
  useEffect(() => {
    const cancel = () => {
      const gesture = seekGesture.current;
      seekGesture.current = null;
      if (gesture) setPlayhead(gesture.initial);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || !seekGesture.current) return;
      event.preventDefault();
      event.stopPropagation();
      cancel();
    };
    window.addEventListener("keydown", escape, true);
    window.addEventListener("blur", cancel);
    document.addEventListener("visibilitychange", cancel);
    return () => {
      window.removeEventListener("keydown", escape, true);
      window.removeEventListener("blur", cancel);
      document.removeEventListener("visibilitychange", cancel);
      cancel();
    };
  }, [documentEpoch, suspended, timeline]);
  const canvasAssets = timelineCanvasAssets(view.document?.nodes ?? [], nodeID);
  const nodeAssets = new Map(
    canvasAssets.map((item) => [item.nodeID, item.assetID]),
  );
  const clipIDs = JSON.stringify(
    [
      ...new Set(
        [
          ...(timeline?.imported_assets ?? []),
          ...canvasAssets.map((item) => item.assetID),
          ...(timeline?.clips ?? []).map(
            (c) =>
              c.asset_id ?? (c.node_id ? nodeAssets.get(c.node_id) : undefined),
          ),
        ].filter((id): id is string => !!id),
      ),
    ].sort(),
  );
  const previewMedia = useTimelineMedia(projectID, scope, clipIDs);
  const media: Record<string, TimelineMedia> = { ...previewMedia.media };
  for (const clip of timeline?.clips ?? []) {
    if (clip.asset_id || !clip.node_id) continue;
    const assetID = nodeAssets.get(clip.node_id);
    if (assetID && media[assetID]) media[clip.node_id] = media[assetID];
  }
  const mediaFailures = useMemo(
    () => ({
      ...previewMedia.errors,
      ...Object.fromEntries(
        Object.entries(playbackFailures)
          .filter(([id, failure]) => previewMedia.media[id] === failure.media)
          .map(([id, failure]) => [id, failure.message]),
      ),
    }),
    [previewMedia.errors, previewMedia.media, playbackFailures],
  );
  const previouslySuspended = useRef(suspended);
  useEffect(() => {
    const resumed = previouslySuspended.current && !suspended;
    previouslySuspended.current = suspended;
    if (!resumed) return;
    for (const id of Object.keys(mediaFailures)) previewMedia.retry(id);
  }, [suspended, mediaFailures, previewMedia]);
  useEffect(() => {
    const activeRequests = requests.current;
    return () => {
      for (const request of activeRequests) request.abort();
    };
  }, []);
  const duration = timeline ? timelineDuration(timeline) : 0;
  const {
    zoom,
    changeZoom,
    geometry: trackGeometry,
    scrollRef,
  } = useTimelineViewport(
    trimPreview ? timelineDuration(trimPreview) : duration,
    compact,
    (() => {
      const first = (trimPreview ?? timeline)?.clips.find(
        (clip) => clip.kind !== "audio",
      );
      return first?.kind === "video" ? first.source_start_ms : 0;
    })(),
    trimming,
  );
  const playhead = Math.min(position, duration);
  const playingNow = playing && playhead < duration && !suspended;
  function seekAt(event: ReactPointerEvent<HTMLDivElement>) {
    const element = event.currentTarget;
    const next = timelinePointerDelta(
      event.clientX - element.getBoundingClientRect().left,
      zoom,
      timelineElementScale(element),
    );
    setPlayhead(Math.max(0, Math.min(duration, Math.round(next))));
  }
  const seekHandlers = {
    onPointerDown: (event: ReactPointerEvent<HTMLDivElement>) => {
      event.preventDefault();
      event.stopPropagation();
      if (!duration || event.button !== 0) return;
      if (event.currentTarget.getAttribute("role") === "slider")
        event.currentTarget.focus({ preventScroll: true });
      seekGesture.current = {
        pointerID: event.pointerId,
        element: event.currentTarget,
        initial: playhead,
      };
      event.currentTarget.setPointerCapture(event.pointerId);
      setPlaying(false);
      seekAt(event);
    },
    onPointerMove: (event: ReactPointerEvent<HTMLDivElement>) => {
      if (
        seekGesture.current?.element !== event.currentTarget ||
        seekGesture.current.pointerID !== event.pointerId
      )
        return;
      event.stopPropagation();
      seekAt(event);
    },
    onPointerUp: (event: ReactPointerEvent<HTMLDivElement>) => {
      if (
        seekGesture.current?.element !== event.currentTarget ||
        seekGesture.current.pointerID !== event.pointerId
      )
        return;
      event.stopPropagation();
      seekAt(event);
      seekGesture.current = null;
    },
    onPointerCancel: () => {
      const gesture = seekGesture.current;
      seekGesture.current = null;
      if (gesture) setPlayhead(gesture.initial);
    },
    onLostPointerCapture: () => {
      const gesture = seekGesture.current;
      seekGesture.current = null;
      if (gesture) setPlayhead(gesture.initial);
    },
  };

  useEffect(() => {
    if (!playingNow || !duration) return;
    let frame = 0,
      previous = performance.now();
    const tick = (now: number) => {
      const elapsed = now - previous;
      if (elapsed >= 30) {
        previous = now;
        setPlayhead((value) => Math.min(duration, value + elapsed));
      }
      frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [playingNow, duration]);
  const onPreviewError = useCallback(
    (message: string, source?: TimelineMedia) => {
      const id = source?.asset.id;
      if (!id || !source) {
        setError(message);
        return;
      }
      setPlaybackFailures((current) => ({
        ...current,
        [id]: { message, media: source },
      }));
    },
    [],
  );
  function save(next: Timeline, expected = timeline) {
    if (!expected) return;
    try {
      client.changeTimeline(nodeID, next, expected);
      setRetainedEdit(null);
    } catch (cause) {
      setRetainedEdit(next);
      throw cause;
    }
  }
  const editEpoch = client.getDocumentEpoch();
  function edit(action: TimelineEdit, expectedEpoch?: string | null) {
    if (!timeline || readOnly) return;
    try {
      if (
        expectedEpoch !== undefined &&
        (!expectedEpoch || expectedEpoch !== client.getDocumentEpoch())
      )
        throw new Error("画布已恢复到其他版本，请重新编辑片段");
      save(editTimeline(timeline, action));
      setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "编辑未保存");
    }
  }
  async function add(
    assets: { asset: API.AssetSummary; nodeID?: string }[],
    importOnly = false,
    destination = targetTrack,
    expectedEpoch = picker ? pickerEpoch : undefined,
    keepPickerOpen = false,
  ) {
    if (adding.current || readOnly)
      throw new Error("当前时间线不可写入，请稍后重试");
    adding.current = true;
    setBusy(true);
    const expected = scope;
    try {
      if (
        expectedEpoch !== undefined &&
        (!expectedEpoch || expectedEpoch !== client.getDocumentEpoch())
      )
        throw new Error("画布已恢复到其他版本，请关闭素材选择器后重新导入");
      if (assets.some((item) => item.asset.project_id !== projectID))
        throw new Error("素材不属于当前项目");
      const latest = client.getSnapshot();
      const currentNode = latest.document?.nodes.find(
        (item) => item.id === nodeID,
      );
      if (
        identity.current !== expected ||
        !latest.writable ||
        latest.migration ||
        !currentNode
      )
        throw new Error("画布已切换或不可编辑，请重新导入");
      const current = currentNode.serviceData?.config?.timeline;
      const before = readTimeline(current);
      client.changeTimeline(
        nodeID,
        editTimeline(
          before,
          importOnly
            ? { type: "import", assets }
            : { type: "add", assets, trackID: destination },
        ),
        before,
      );
      if (!keepPickerOpen) {
        setPicker(null);
        setTargetTrack(undefined);
      }
      setError("");
    } catch (cause) {
      if (identity.current === expected)
        setError(cause instanceof Error ? cause.message : "添加素材失败");
      throw cause;
    } finally {
      adding.current = false;
      if (identity.current === expected) setBusy(false);
    }
  }
  async function deliver(result: TimelineResult) {
    const epoch = client.getDocumentEpoch();
    const graph = client.getSnapshot().document;
    if (!graph || !epoch || identity.current !== scope)
      throw new Error("画布已切换，请重新添加导出结果");
    const next = timelineResultGraph(graph, nodeID, result);
    if (!client.canAccept(next))
      throw new Error("画布当前不可编辑，导出结果已保留");
    if (next !== graph) client.change(next, true);
    await client.waitForSaved(() => {
      if (
        client.getDocumentEpoch() !== epoch ||
        identity.current !== scope ||
        !client.getSnapshot().document?.nodes.some((n) => n.id === result.jobID)
      )
        throw new Error("画布交付尚未确认，请重试");
      const current = client.getSnapshot().document;
      if (!current || timelineResultGraph(current, nodeID, result) !== current)
        throw new Error("导出结果已变化，请重新确认画布交付");
    });
  }
  function togglePlayback() {
    if (!duration) return;
    if (playhead >= duration) {
      setPlayhead(0);
      setPlaying(true);
    } else setPlaying(!playingNow);
  }
  function receiveDrop(event: React.DragEvent, trackID?: string) {
    event.preventDefault();
    event.stopPropagation();
    if (readOnly) return;
    const assetID = event.dataTransfer.getData("application/x-timeline-asset");
    if (assetID && media[assetID]) {
      void add([{ asset: media[assetID].asset }], !trackID, trackID).catch(
        () => {},
      );
      return;
    }
    const files = Array.from(event.dataTransfer.files);
    if (files.length) {
      setTargetTrack(trackID);
      setDroppedFiles(
        files.map((file) => ({
          id: crypto.randomUUID(),
          file,
          trackID,
          epoch: client.getDocumentEpoch(),
        })),
      );
    }
  }
  async function addCanvas(assetID: string, sourceNodeID: string) {
    if (adding.current || importingCanvas.current || readOnly) return;
    importingCanvas.current = true;
    const abort = new AbortController();
    requests.current.add(abort);
    const expected = scope,
      expectedEpoch = client.getDocumentEpoch();
    setBusy(true);
    try {
      const value = await getMediaPreview(
        { pid: projectID, asset_id: assetID },
        { signal: abort.signal },
      );
      abort.signal.throwIfAborted();
      if (identity.current !== expected) return;
      if (!expectedEpoch || expectedEpoch !== client.getDocumentEpoch())
        throw new Error("画布已恢复到其他版本，请重新选择素材");
      if (!value.asset || value.asset.id !== assetID)
        throw new Error("素材身份不匹配");
      const currentNode = client
        .getSnapshot()
        .document?.nodes.find((item) => item.id === sourceNodeID);
      if (
        !currentNode ||
        timelineCanvasAssets([currentNode], nodeID)[0]?.assetID !== assetID
      )
        throw new Error("画布素材已变化，请重新选择");
      await add([{ asset: value.asset, nodeID: sourceNodeID }]);
    } catch (cause) {
      if (!abort.signal.aborted && identity.current === expected)
        setError(cause instanceof Error ? cause.message : "素材读取失败");
    } finally {
      importingCanvas.current = false;
      requests.current.delete(abort);
      if (!abort.signal.aborted && identity.current === expected)
        setBusy(false);
    }
  }
  if (!timeline)
    return (
      <div role="alert" className="p-4 text-sm">
        {invalid}
      </div>
    );
  const selectedClip = timeline.clips.find((item) => item.id === selected),
    selectedTrack = timeline.tracks.find(
      (item) => item.id === selectedClip?.track_id,
    );
  const activeClipVolume =
    !readOnly &&
    !selectedTrack?.locked &&
    clipVolume?.baseline === timeline &&
    clipVolume.id === selected
      ? clipVolume
      : null;
  function changeHistory(direction: "undo" | "redo") {
    setClipVolume(null);
    client.timelineHistory(nodeID, direction);
  }
  const cuttable =
    !!selectedClip &&
    !selectedTrack?.locked &&
    playhead - selectedClip.start_ms >= 100 &&
    selectedClip.start_ms + selectedClip.duration_ms - playhead >= 100;
  const importedAssets = timeline.imported_assets.map((assetID) => ({
    assetID,
    title:
      media[assetID]?.asset.file_name ??
      timeline.clips.find((c) => c.asset_id === assetID)?.title ??
      "素材加载中",
    kind:
      media[assetID]?.asset.kind ??
      timeline.clips.find((c) => c.asset_id === assetID)?.kind ??
      "",
  }));
  const visibleAssets = (
    assetSource === "canvas"
      ? canvasAssets
      : importedAssets.map((item) => ({ ...item, nodeID: undefined }))
  ).filter((item) => filter === "all" || item.kind === filter);
  const displayTimeline = trimPreview ?? timeline;
  const trackWidth = trackGeometry.lane;
  const rulerIntervals = timelineRulerIntervals(zoom, compact);
  const toolbar = (
    <div role="toolbar" aria-label="时间线剪辑工具" className="timeline-tools">
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="撤销"
        disabled={readOnly || !client.timelineHistoryState(nodeID).canUndo}
        onClick={() => changeHistory("undo")}
      >
        <Undo2 />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="重做"
        disabled={readOnly || !client.timelineHistoryState(nodeID).canRedo}
        onClick={() => changeHistory("redo")}
      >
        <Redo2 />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="分割"
        title={!cuttable ? "将播放头移动到目标片段内进行剪切。" : "分割"}
        disabled={readOnly || !cuttable}
        onClick={() =>
          selected && edit({ type: "split", id: selected, at: playhead })
        }
      >
        <TimelineCutIcon />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="向左剪裁"
        title={!cuttable ? "将播放头移动到目标片段内进行剪切。" : "向左剪裁"}
        disabled={readOnly || !cuttable}
        onClick={() =>
          selected && edit({ type: "trim-left", id: selected, at: playhead })
        }
      >
        <TimelineCutIcon trim="left" />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="向右剪裁"
        title={!cuttable ? "将播放头移动到目标片段内进行剪切。" : "向右剪裁"}
        disabled={readOnly || !cuttable}
        onClick={() =>
          selected && edit({ type: "trim-right", id: selected, at: playhead })
        }
      >
        <TimelineCutIcon trim="right" />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="删除"
        disabled={readOnly || !selectedClip || selectedTrack?.locked}
        onClick={() => selected && edit({ type: "remove", id: selected })}
      >
        <Trash2 />
      </Button>
      <div className="flex-1" />
      {selectedClip?.kind === "audio" && (
        <div className="timeline-audio-controls">
          <Button
            variant="ghost"
            size="icon-sm"
            disabled={readOnly || selectedTrack?.locked}
            aria-label={selectedClip.volume ? "静音片段" : "开启片段声音"}
            onClick={() =>
              edit({
                type: "clip",
                id: selectedClip.id,
                patch: { volume: selectedClip.volume ? 0 : 1 },
              })
            }
          >
            {selectedClip.volume ? <Volume2 /> : <VolumeX />}
          </Button>
          <Slider
            aria-label="片段音量"
            className="w-[90px]"
            min={0}
            max={100}
            value={[
              activeClipVolume?.id === selectedClip.id
                ? activeClipVolume.value
                : selectedClip.volume * 100,
            ]}
            disabled={readOnly || selectedTrack?.locked}
            onValueChange={([value]) =>
              setClipVolume({ id: selectedClip.id, value, baseline: timeline })
            }
            onValueCommit={([value]) => {
              edit({
                type: "clip",
                id: selectedClip.id,
                patch: { volume: value / 100 },
              });
              setClipVolume(null);
            }}
          />
        </div>
      )}

      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={timeline.snapping ? "关闭自动吸附" : "开启自动吸附"}
        aria-pressed={timeline.snapping}
        disabled={readOnly}
        className={timeline.snapping ? "timeline-snap-active" : ""}
        onClick={() => {
          try {
            save({ ...timeline, snapping: !timeline.snapping });
          } catch (cause) {
            setError(String(cause));
          }
        }}
      >
        <TimelineSnapIcon />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="缩小视图"
        onClick={() => changeZoom(zoom / 1.1)}
      >
        <CircleMinus />
      </Button>
      <Slider
        aria-label="时间线缩放"
        className="timeline-zoom w-20"
        min={14}
        max={180}
        step={0.1}
        value={[zoom]}
        onValueChange={([value]) => changeZoom(value)}
      />
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="放大视图"
        onClick={() => changeZoom(zoom * 1.1)}
      >
        <CirclePlus />
      </Button>
    </div>
  );
  const importer = (trigger: ReactNode, trackID?: string) => (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
      <DropdownMenuContent
        data-canvas-no-zoom
        onMouseDown={(event) => event.stopPropagation()}
      >
        <DropdownMenuItem
          onSelect={() => {
            setTargetTrack(trackID);
            setPickerEpoch(client.getDocumentEpoch());
            setPicker("upload");
          }}
        >
          <Upload />
          本地上传
        </DropdownMenuItem>
        <DropdownMenuItem
          onSelect={() => {
            setTargetTrack(trackID);
            setPickerEpoch(client.getDocumentEpoch());
            setPicker("library");
          }}
        >
          <FolderOpen />
          从资产库添加
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
  return (
    <section
      className={`canvas-timeline ${compact ? "timeline-compact" : ""}`}
      aria-label="时间线工作区"
      data-canvas-shortcuts-ignore
      data-canvas-no-zoom
      onMouseDown={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      onDoubleClick={(event) => event.stopPropagation()}
      onKeyDown={(event) => {
        if (
          (event.target as HTMLElement).closest(
            "input:not([type=range]),textarea,[contenteditable=true]",
          )
        )
          return;
        if (
          (event.metaKey || event.ctrlKey) &&
          event.key.toLowerCase() === "z"
        ) {
          event.preventDefault();
          event.stopPropagation();
          if (!readOnly) changeHistory(event.shiftKey ? "redo" : "undo");
          return;
        }
        if ((event.target as HTMLElement).closest("[role=slider]")) return;
        if (event.key === "Escape") {
          drag.current = null;
          setDragOffset(null);
          setDropAfter(null);
        }
        if (event.code === "Space") {
          event.preventDefault();
          togglePlayback();
        }
        if (event.key === "Home") {
          event.preventDefault();
          setPlayhead(0);
          setPlaying(false);
        }
        if (
          !event.shiftKey &&
          (event.key === "ArrowLeft" || event.key === "ArrowRight")
        ) {
          event.preventDefault();
          setPlaying(false);
          setPlayhead((value) =>
            Math.max(
              0,
              Math.min(
                duration,
                value + (event.key === "ArrowRight" ? 1000 : -1000),
              ),
            ),
          );
        }
      }}
    >
      <header className="timeline-header">
        {compact ? (
          <>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="分割"
              disabled={readOnly || !cuttable}
              onClick={() =>
                selected && edit({ type: "split", id: selected, at: playhead })
              }
            >
              <TimelineCutIcon />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="删除"
              disabled={readOnly || !selectedClip || selectedTrack?.locked}
              onClick={() => selected && edit({ type: "remove", id: selected })}
            >
              <Trash2 />
            </Button>
            <div className="timeline-compact-play">
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={playingNow ? "暂停预览" : "播放时间线预览"}
                disabled={!duration}
                onClick={togglePlayback}
              >
                {playingNow ? (
                  <Pause />
                ) : (
                  <Play fill="currentColor" strokeWidth={0} />
                )}
              </Button>
              <span>
                {timelineRulerTime(playhead)} / {timelineRulerTime(duration)}
              </span>
            </div>
          </>
        ) : (
          <h2 className="ml-1.5 text-sm">时间线</h2>
        )}
        <div className="flex-1" />
        <TimelineExport
          compact={compact}
          suspended={suspended}
          client={client}
          nodeID={nodeID}
          canvasID={canvasID}
          projectID={projectID}
          disabled={!timeline.clips.length || readOnly}
          hasVisual={timeline.clips.some(
            (clip) => clip.kind === "video" || clip.kind === "image",
          )}
          aspectRatio={timeline.aspect_ratio}
          nodeTitle={node?.title}
          onAddToCanvas={deliver}
        />
        {compact ? (
          <Button
            variant="ghost"
            size="sm"
            aria-label="全屏编辑"
            onClick={() => {
              setPlaying(false);
              onExpand?.();
            }}
          >
            <Maximize2 />
            全屏编辑
          </Button>
        ) : (
          onClose && (
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="关闭时间线编辑器"
              onClick={onClose}
            >
              <X />
            </Button>
          )
        )}
      </header>
      {error && (
        <p role="alert" className="px-3 text-xs text-destructive">
          {error}
        </p>
      )}
      {Object.entries(mediaFailures).map(([assetID, message]) => {
        const title =
          media[assetID]?.asset.file_name ??
          timeline.clips.find((clip) => clip.asset_id === assetID)?.title ??
          "素材";
        return (
          <div
            key={assetID}
            role="alert"
            className="flex items-center gap-2 px-3 text-xs text-destructive"
          >
            <span>{message}</span>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setPlaybackFailures((current) => {
                  const next = { ...current };
                  delete next[assetID];
                  return next;
                });
                previewMedia.retry(assetID);
              }}
            >
              重新读取 {title}
            </Button>
          </div>
        );
      })}
      {retainedEdit && (
        <div
          role="status"
          className="flex flex-wrap items-center gap-2 text-xs"
        >
          未保存的编辑已保留，请检查当前时间线。
          <Button
            variant="outline"
            size="sm"
            disabled={readOnly}
            onClick={() => {
              try {
                save(retainedEdit);
                setError("");
              } catch (cause) {
                setError(cause instanceof Error ? cause.message : "编辑未保存");
              }
            }}
          >
            用保留编辑替换当前时间线
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              client.discardRetainedTimeline(nodeID);
              setRetainedEdit(null);
            }}
          >
            放弃保留编辑
          </Button>
        </div>
      )}
      <div
        className="timeline-upper"
        aria-hidden={compact || undefined}
        inert={compact}
      >
        {!compact && showAssets && (
          <aside
            className="timeline-assets"
            style={{ width: assetWidth }}
            onDragOver={(e) => {
              if (!readOnly) e.preventDefault();
            }}
            onDrop={(event) => receiveDrop(event)}
          >
            <Tabs value={assetSource} onValueChange={setAssetSource}>
              <TabsList>
                <TabsTrigger value="imported">已导入资产</TabsTrigger>
                <TabsTrigger value="canvas">画布资产</TabsTrigger>
              </TabsList>
            </Tabs>
            <div className="flex items-center justify-between gap-1">
              <Tabs value={filter} onValueChange={setFilter}>
                <TabsList variant="line">
                  {[
                    ["all", "全部"],
                    ["image", "图片"],
                    ["video", "视频"],
                    ["audio", "音频"],
                  ].map(([value, label]) => (
                    <TabsTrigger
                      key={value}
                      value={value}
                      className="px-1 text-xs"
                    >
                      {label}
                    </TabsTrigger>
                  ))}
                </TabsList>
              </Tabs>
              {importer(
                <Button
                  variant="outline"
                  size="sm"
                  className="h-8 bg-transparent text-[13px]"
                  disabled={readOnly || busy}
                >
                  <Plus />
                  导入
                </Button>,
              )}
            </div>
            <div className="timeline-asset-items">
              {visibleAssets.map((item) => (
                <div
                  key={item.assetID}
                  className="timeline-asset-card"
                  draggable={!readOnly && !!media[item.assetID]}
                  onDragStart={(event) =>
                    event.dataTransfer.setData(
                      "application/x-timeline-asset",
                      item.assetID,
                    )
                  }
                >
                  <TimelineThumbnail
                    media={media[item.assetID]}
                    kind={item.kind}
                  />
                  <span className="sr-only">{item.title}</span>
                  <span className="timeline-asset-duration">
                    {media[item.assetID]?.asset.duration_ms
                      ? timelineRulerTime(
                          media[item.assetID].asset.duration_ms!,
                        )
                      : ""}
                  </span>
                  {previewMedia.errors[item.assetID] ? (
                    <span className="timeline-asset-error">素材读取失败</span>
                  ) : (
                    timeline.clips.some(
                      (c) =>
                        c.asset_id === item.assetID ||
                        (c.node_id === item.nodeID && !!item.nodeID),
                    ) && <span className="timeline-asset-added">已添加</span>
                  )}
                  <Button
                    variant="secondary"
                    size="icon-sm"
                    className="timeline-asset-action"
                    title={item.title}
                    aria-label={`添加 ${item.title} 到时间线`}
                    disabled={readOnly || busy || !media[item.assetID]}
                    onClick={() =>
                      item.nodeID
                        ? void addCanvas(item.assetID, item.nodeID)
                        : void add([
                            { asset: media[item.assetID].asset },
                          ]).catch(() => {})
                    }
                  >
                    <span className="timeline-asset-add">
                      <Plus />
                    </span>
                  </Button>
                </div>
              ))}
              {visibleAssets.length === 0 && (
                <p className="timeline-assets-empty">将文件拖至此处添加</p>
              )}
            </div>
            <Button
              variant="ghost"
              size="icon-sm"
              className="timeline-assets-collapse"
              aria-label="隐藏资产库"
              onClick={() => setShowAssets(false)}
            >
              <PanelLeftClose />
            </Button>
            <div
              role="separator"
              tabIndex={0}
              aria-label="调整资产栏宽度"
              aria-valuenow={assetWidth}
              aria-valuemin={240}
              aria-valuemax={600}
              aria-orientation="vertical"
              className="timeline-assets-resize"
              onKeyDown={(event) => {
                if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
                  event.preventDefault();
                  setAssetWidth((value) =>
                    Math.max(
                      240,
                      Math.min(
                        600,
                        value + (event.key === "ArrowRight" ? 20 : -20),
                      ),
                    ),
                  );
                }
              }}
              onPointerDown={(event) => {
                event.currentTarget.setPointerCapture(event.pointerId);
              }}
              onPointerMove={(event) => {
                if (event.currentTarget.hasPointerCapture(event.pointerId))
                  setAssetWidth((value) =>
                    Math.max(240, Math.min(600, value + event.movementX)),
                  );
              }}
            />
          </aside>
        )}
        <div className="timeline-preview-area" ref={preview}>
          {!compact && !showAssets && (
            <Button
              variant="ghost"
              size="icon-sm"
              className="absolute top-2 left-2 z-20"
              aria-label="显示资产库"
              onClick={() => setShowAssets(true)}
            >
              <PanelLeftOpen />
            </Button>
          )}
          <div className="timeline-preview-stage">
            <TimelinePreview
              timeline={
                activeClipVolume
                  ? {
                      ...timeline,
                      clips: timeline.clips.map((clip) =>
                        clip.id === activeClipVolume.id
                          ? { ...clip, volume: activeClipVolume.value / 100 }
                          : clip,
                      ),
                    }
                  : timeline
              }
              media={media}
              playhead={playhead}
              playing={playingNow}
              muted={muted}
              volume={volume}
              onError={onPreviewError}
            />
          </div>
          <div className="timeline-player" hidden={compact}>
            <div className="timeline-player-center">
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={playingNow ? "暂停预览" : "播放时间线预览"}
                disabled={!duration}
                onClick={togglePlayback}
              >
                {playingNow ? (
                  <Pause />
                ) : (
                  <Play fill="currentColor" strokeWidth={0} />
                )}
              </Button>
              <span className="text-xs tabular-nums">
                {timelineTime(playhead)}{" "}
                <span className="text-foreground/30">
                  / {timelineTime(duration)}
                </span>
              </span>
            </div>
            <div className="timeline-player-options">
              <TimelineAspectSelect
                value={timeline.aspect_ratio}
                readOnly={readOnly}
                onChange={(aspect_ratio) => {
                  try {
                    save({ ...timeline, aspect_ratio });
                  } catch (cause) {
                    setError(String(cause));
                  }
                }}
              />
              <div className="timeline-preview-volume-group">
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={muted ? "开启原声" : "静音预览"}
                  aria-pressed={muted}
                  onClick={() => setMuted(!muted)}
                >
                  {muted ? <VolumeX /> : <Volume2 />}
                </Button>
                {!compact && (
                  <Slider
                    aria-label="预览音量"
                    className="timeline-preview-volume"
                    min={0}
                    max={100}
                    value={[volume * 100]}
                    onValueChange={([value]) => setVolume(value / 100)}
                  />
                )}
              </div>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={playerFullscreen ? "退出全屏播放" : "全屏播放"}
                aria-pressed={playerFullscreen}
                onClick={async () => {
                  try {
                    if (document.fullscreenElement === preview.current)
                      await document.exitFullscreen();
                    else await preview.current?.requestFullscreen();
                  } catch {
                    setError(
                      playerFullscreen
                        ? "浏览器未能退出全屏播放"
                        : "浏览器未允许全屏播放",
                    );
                  }
                }}
              >
                {playerFullscreen ? <Minimize2 /> : <Maximize2 />}
              </Button>
            </div>
          </div>
        </div>
      </div>
      {!compact && (
        <div
          role="separator"
          tabIndex={0}
          aria-label="调整轨道编辑区高度"
          aria-valuenow={trackHeight}
          aria-valuemin={160}
          aria-valuemax={500}
          aria-orientation="horizontal"
          className="timeline-height-resize"
          onKeyDown={(event) => {
            if (event.key === "ArrowUp" || event.key === "ArrowDown") {
              event.preventDefault();
              setTrackHeight((value) =>
                Math.max(
                  160,
                  Math.min(500, value + (event.key === "ArrowUp" ? 20 : -20)),
                ),
              );
            }
          }}
          onPointerDown={(event) =>
            event.currentTarget.setPointerCapture(event.pointerId)
          }
          onPointerMove={(event) => {
            if (event.currentTarget.hasPointerCapture(event.pointerId))
              setTrackHeight((value) =>
                Math.max(160, Math.min(500, value - event.movementY)),
              );
          }}
        />
      )}
      <div
        className="timeline-editor"
        style={{
          height: compact
            ? Math.max(
                118,
                40 +
                  timeline.tracks.reduce(
                    (height, track) =>
                      height + (track.kind === "audio" ? 36 : 60),
                    0,
                  ),
              )
            : trackHeight,
        }}
      >
        {!compact && toolbar}
        <div className="timeline-tracks-scroll" ref={scrollRef}>
          <div
            className="timeline-tracks"
            style={{ width: trackGeometry.content }}
          >
            <div
              className="timeline-ruler"
              {...seekHandlers}
              style={{
                marginLeft: trackGeometry.offset,
                backgroundSize: `${zoom * rulerIntervals.minor}px 5px`,
              }}
            >
              {Array.from(
                { length: Math.ceil(trackWidth / zoom / rulerIntervals.major) },
                (_, index) => (
                  <span
                    key={index}
                    style={{ left: index * rulerIntervals.major * zoom }}
                  >
                    {timelineRulerTime(index * rulerIntervals.major * 1000)}
                  </span>
                ),
              )}
            </div>
            {timeline.tracks.map((track) => (
              <div
                className="timeline-track-row"
                key={track.id}
                data-kind={track.kind}
                data-selected={selectedTrack?.id === track.id}
                aria-label={track.label}
              >
                {dropAfter === track.id && (
                  <div
                    className="timeline-new-track-indicator"
                    aria-hidden="true"
                  />
                )}
                <div
                  className="timeline-track-control"
                  style={{ width: compact ? 66 : 44 }}
                >
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`${track.muted ? "取消静音" : "静音"} ${track.label}`}
                    aria-pressed={track.muted}
                    disabled={readOnly}
                    onClick={() =>
                      edit({
                        type: "track",
                        id: track.id,
                        patch: { muted: !track.muted },
                      })
                    }
                  >
                    {track.muted ? <VolumeX /> : <Volume2 />}
                  </Button>
                </div>
                <div
                  className="timeline-lane"
                  data-track-id={track.id}
                  data-kind={track.kind}
                  style={{
                    width: trackWidth,
                    flexShrink: 0,
                    marginLeft: trackGeometry.offset - (compact ? 66 : 44),
                  }}
                  onDragOver={(event) => {
                    if (!readOnly && !track.locked) event.preventDefault();
                  }}
                  onDrop={(event) => receiveDrop(event, track.id)}
                >
                  {(track.kind === "video" ||
                    !timeline.clips.some((c) => c.track_id === track.id)) &&
                    importer(
                      <Button
                        variant="ghost"
                        className="timeline-track-append"
                        aria-label={`添加素材到 ${track.label}`}
                        disabled={readOnly || busy || track.locked}
                        style={{
                          left: Math.max(
                            0,
                            ...displayTimeline.clips
                              .filter((c) => c.track_id === track.id)
                              .map(
                                (c) =>
                                  ((c.start_ms + c.duration_ms) / 1000) * zoom,
                              ),
                          ),
                        }}
                      >
                        <Plus />
                        {compact && timeline.clips.length === 0 && (
                          <span>添加素材到时间线</span>
                        )}
                      </Button>,
                      track.id,
                    )}
                  {displayTimeline.clips
                    .filter((c) => c.track_id === track.id)
                    .map((clip) => (
                      <div
                        key={clip.id}
                        className="timeline-clip-shell"
                        style={{
                          left: (clip.start_ms / 1000) * zoom,
                          width: Math.max(3, (clip.duration_ms / 1000) * zoom),
                          zIndex: dragOffset?.id === clip.id ? 6 : undefined,
                          transform:
                            dragOffset?.id === clip.id
                              ? `translate(${dragOffset.px}px, ${dragOffset.py}px)`
                              : undefined,
                        }}
                      >
                        <Button
                          variant="ghost"
                          type="button"
                          aria-label={`${clip.title}，${timelineTime(clip.start_ms)}，时长${(clip.duration_ms / 1000).toFixed(2)}秒`}
                          aria-pressed={selected === clip.id}
                          className="timeline-clip"
                          onClick={() => setSelected(clip.id)}
                          onPointerDown={(event) => {
                            setSelected(clip.id);
                            if (readOnly || track.locked) return;
                            event.currentTarget.setPointerCapture(
                              event.pointerId,
                            );
                            drag.current = {
                              id: clip.id,
                              startX: event.clientX,
                              startY: event.clientY,
                              scale: timelineElementScale(event.currentTarget),
                              zoom,
                              start: clip.start_ms,
                              baseline: timeline,
                              epoch: client.getDocumentEpoch(),
                              snap: null,
                            };
                          }}
                          onPointerMove={(event) => {
                            const intent = drag.current;
                            if (!intent || intent.id !== clip.id) return;
                            const raw = Math.max(
                              0,
                              intent.start +
                                timelinePointerDelta(
                                  event.clientX - intent.startX,
                                  intent.zoom,
                                  intent.scale,
                                ),
                            );
                            const snapped =
                              clip.kind === "audio"
                                ? timelineMoveSnap(
                                    intent.baseline,
                                    clip,
                                    raw,
                                    playhead,
                                    8000 / intent.zoom / intent.scale,
                                    intent.snap,
                                    12000 / intent.zoom / intent.scale,
                                  )
                                : { start: raw, snap: null };
                            intent.snap = snapped.snap;
                            setDragOffset({
                              id: clip.id,
                              px:
                                clip.kind === "audio"
                                  ? ((snapped.start - intent.start) / 1000) *
                                    intent.zoom
                                  : (event.clientX - intent.startX) /
                                    intent.scale,
                              py:
                                clip.kind === "audio"
                                  ? (event.clientY - intent.startY) /
                                    intent.scale
                                  : 0,
                            });
                            if (clip.kind === "audio") {
                              const target = audioDropTarget(
                                event.currentTarget,
                                event.clientY,
                                intent.scale,
                              );
                              setDropAfter(
                                target?.newAudioTrack?.afterTrackID ?? null,
                              );
                            }
                          }}
                          onPointerUp={(event) => {
                            setDragOffset(null);
                            setDropAfter(null);
                            const intent = drag.current;
                            drag.current = null;
                            if (!intent || intent.id !== clip.id) return;
                            const delta = timelinePointerDelta(
                              event.clientX - intent.startX,
                              intent.zoom,
                              intent.scale,
                            );
                            const target =
                              clip.kind === "audio"
                                ? audioDropTarget(
                                    event.currentTarget,
                                    event.clientY,
                                    intent.scale,
                                  )
                                : undefined;
                            if (clip.kind === "audio" && !target) return;
                            if (
                              Math.abs(delta) < 1 &&
                              !target?.newAudioTrack &&
                              (!target?.trackID ||
                                target.trackID === clip.track_id)
                            )
                              return;
                            try {
                              if (
                                !intent.epoch ||
                                intent.epoch !== client.getDocumentEpoch()
                              )
                                throw new Error(
                                  "画布已恢复到其他版本，请重新移动片段",
                                );
                              save(
                                editTimeline(intent.baseline, {
                                  type: "move",
                                  id: clip.id,
                                  start: Math.max(0, intent.start + delta),
                                  tolerance: 8000 / intent.zoom / intent.scale,
                                  releaseTolerance:
                                    12000 / intent.zoom / intent.scale,
                                  previousSnap: intent.snap,
                                  playhead,
                                  ...target,
                                }),
                                intent.baseline,
                              );
                            } catch (cause) {
                              setError(
                                cause instanceof Error
                                  ? cause.message
                                  : "移动未完成",
                              );
                            }
                          }}
                          onLostPointerCapture={() => {
                            setDragOffset(null);
                            setDropAfter(null);
                            drag.current = null;
                          }}
                          onPointerCancel={() => {
                            setDragOffset(null);
                            setDropAfter(null);
                            drag.current = null;
                          }}
                          onKeyDown={(event) => {
                            if (
                              event.shiftKey &&
                              (event.key === "ArrowLeft" ||
                                event.key === "ArrowRight")
                            ) {
                              event.preventDefault();
                              edit({
                                type: "reorder",
                                id: clip.id,
                                direction: event.key === "ArrowLeft" ? -1 : 1,
                              });
                            } else if (
                              event.key === "Delete" ||
                              event.key === "Backspace"
                            ) {
                              event.preventDefault();
                              edit({ type: "remove", id: clip.id });
                            }
                          }}
                        >
                          <TimelineThumbnail
                            media={media[clip.asset_id ?? clip.node_id ?? ""]}
                            kind={clip.kind}
                            strip={clip.kind !== "audio"}
                            startMS={clip.source_start_ms}
                            durationMS={clip.duration_ms}
                          />
                          {clip.kind !== "audio" && (
                            <span className="timeline-clip-title">
                              {timelineRulerTime(clip.duration_ms)}
                            </span>
                          )}
                        </Button>
                        {!readOnly &&
                          !track.locked &&
                          selected === clip.id &&
                          (["left", "right"] as const).map((edge) => (
                            <TimelineTrimHandle
                              key={edge}
                              clip={clip}
                              edge={edge}
                              zoom={zoom}
                              onGestureChange={setTrimming}
                              onPreview={(delta) => {
                                if (delta === null) {
                                  setTrimPreview(null);
                                  return;
                                }
                                try {
                                  setTrimPreview(
                                    editTimeline(timeline, {
                                      type: "resize",
                                      id: clip.id,
                                      edge,
                                      delta,
                                    }),
                                  );
                                } catch {
                                  setTrimPreview(null);
                                }
                              }}
                              onResize={(delta) =>
                                edit(
                                  {
                                    type: "resize",
                                    id: clip.id,
                                    edge,
                                    delta,
                                  },
                                  editEpoch,
                                )
                              }
                            />
                          ))}
                      </div>
                    ))}
                </div>
              </div>
            ))}
            <div
              className="timeline-playhead"
              style={{ left: trackGeometry.offset + (playhead / 1000) * zoom }}
            />
            <div
              role="slider"
              aria-label="时间线播放头"
              aria-valuemin={0}
              aria-valuemax={duration}
              aria-valuenow={playhead}
              aria-valuetext={timelineTime(playhead)}
              aria-disabled={!duration}
              tabIndex={duration ? 0 : -1}
              className="timeline-playhead-slider"
              style={{ left: trackGeometry.offset, width: trackWidth }}
              {...seekHandlers}
              onKeyDown={(event) => {
                if (
                  !duration ||
                  !["Home", "End", "ArrowLeft", "ArrowRight"].includes(
                    event.key,
                  )
                )
                  return;
                event.preventDefault();
                event.stopPropagation();
                setPlaying(false);
                setPlayhead(
                  event.key === "Home"
                    ? 0
                    : event.key === "End"
                      ? duration
                      : Math.max(
                          0,
                          Math.min(
                            duration,
                            playhead +
                              (event.key === "ArrowRight" ? 1000 : -1000),
                          ),
                        ),
                );
              }}
            >
              <span
                className="timeline-playhead-grip"
                style={{ left: (playhead / 1000) * zoom }}
              />
            </div>
          </div>
        </div>
      </div>
      {(droppedFiles.length > 0 || picker === "upload") && (
        <TimelineUploadDialog
          key={droppedFiles[0]?.id ?? pickerEpoch ?? "upload"}
          projectID={projectID}
          initialFiles={droppedFiles.map((item) => item.file)}
          disabled={readOnly}
          isCurrent={() => {
            const epoch = droppedFiles[0]?.epoch ?? pickerEpoch;
            const latest = client.getSnapshot();
            return (
              !!epoch &&
              epoch === client.getDocumentEpoch() &&
              identity.current === scope &&
              latest.writable &&
              !latest.migration &&
              !!latest.document?.nodes.some((item) => item.id === nodeID)
            );
          }}
          onClose={() => {
            setDroppedFiles([]);
            setPicker(null);
            setTargetTrack(undefined);
          }}
          onUploaded={async (asset, { signal }) => {
            signal.throwIfAborted();
            const destination = droppedFiles[0]?.trackID ?? targetTrack;
            await add(
              [{ asset }],
              !destination,
              destination,
              droppedFiles[0]?.epoch ?? pickerEpoch,
              true,
            );
          }}
        />
      )}
      {picker === "library" && (
        <ProjectAssetDialog
          dialogTitle="从资产库添加"
          projectID={projectID}
          allowedKinds={
            targetTrack
              ? [
                  ...(timeline.tracks.find((track) => track.id === targetTrack)
                    ?.kind === "audio"
                    ? ["audio"]
                    : ["image", "video"]),
                ]
              : ["image", "video", "audio"]
          }
          initialKind={
            targetTrack
              ? ((timeline.tracks.find((track) => track.id === targetTrack)
                  ?.kind ?? "video") as "image" | "video" | "audio")
              : "video"
          }
          canvasPresentation
          confirmLabel={targetTrack ? "添加到时间线" : "导入"}
          onClose={() => {
            setPicker(null);
            setTargetTrack(undefined);
          }}
          onAdd={(assets) =>
            add(
              assets.map((asset) => ({ asset })),
              !targetTrack,
            )
          }
        />
      )}
    </section>
  );
}

function audioDropTarget(
  element: HTMLElement,
  y: number,
  scale: number,
): { trackID?: string; newAudioTrack?: { afterTrackID: string } } | undefined {
  const lanes = Array.from(
    element
      .closest(".canvas-timeline")
      ?.querySelectorAll<HTMLElement>(".timeline-lane") ?? [],
  );
  const lane = lanes.find((item) => {
    const rect = item.getBoundingClientRect();
    return item.dataset.kind === "audio" && y >= rect.top && y <= rect.bottom;
  });
  if (lane?.dataset.trackId) return { trackID: lane.dataset.trackId };
  const edge = lanes.find((item) => {
    const rect = item.getBoundingClientRect();
    return y > rect.bottom && y <= rect.bottom + 8 * scale;
  });
  if (edge?.dataset.trackId)
    return { newAudioTrack: { afterTrackID: edge.dataset.trackId } };
}

function TimelineTrimHandle({
  clip,
  edge,
  zoom,
  onResize,
  onPreview,
  onGestureChange,
}: {
  clip: TimelineClip;
  edge: "left" | "right";
  zoom: number;
  onResize: (delta: number) => void;
  onPreview: (delta: number | null) => void;
  onGestureChange: (active: boolean) => void;
}) {
  const gesture = useRef<{
    x: number;
    scale: number;
    zoom: number;
    submit: (delta: number) => void;
    preview: (delta: number | null) => void;
    finish: () => void;
    scroll: HTMLElement | null;
    startScroll: number;
    clientX: number;
    frame: number | null;
    edgeDirection: number;
    edgeEntered: number | null;
    lastFrameTime: number | null;
  } | null>(null);
  function trimDelta(intent: NonNullable<typeof gesture.current>) {
    const logicalDelta =
      (intent.clientX - intent.x) / intent.scale +
      (intent.scroll?.scrollLeft ?? 0) -
      intent.startScroll;
    return timelinePointerDelta(logicalDelta, intent.zoom, 1);
  }
  useEffect(() => {
    const cancel = () => {
      gesture.current?.finish();
      gesture.current?.preview(null);
      gesture.current = null;
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || !gesture.current) return;
      event.preventDefault();
      event.stopPropagation();
      cancel();
    };
    window.addEventListener("blur", cancel);
    window.addEventListener("keydown", escape, true);
    document.addEventListener("visibilitychange", cancel);
    return () => {
      window.removeEventListener("blur", cancel);
      window.removeEventListener("keydown", escape, true);
      document.removeEventListener("visibilitychange", cancel);
      cancel();
    };
  }, []);
  return (
    <span
      role="slider"
      tabIndex={0}
      className="timeline-trim-handle"
      data-edge={edge}
      aria-label={`${edge === "left" ? "左" : "右"}边缘裁剪 ${clip.title}`}
      aria-valuemin={0}
      aria-valuemax={clip.source_duration_ms ?? 86400000}
      aria-valuenow={
        edge === "left"
          ? clip.source_start_ms
          : clip.source_start_ms + clip.duration_ms
      }
      onPointerDown={(e) => {
        e.stopPropagation();
        const scroll = e.currentTarget.closest<HTMLElement>(
          ".timeline-tracks-scroll",
        );
        const scale = timelineElementScale(e.currentTarget);
        const finish = () => {
          if (gesture.current?.frame != null)
            cancelAnimationFrame(gesture.current.frame);
          onGestureChange(false);
        };
        gesture.current = {
          x: e.clientX,
          clientX: e.clientX,
          scale,
          zoom,
          submit: onResize,
          preview: onPreview,
          finish,
          scroll,
          startScroll: scroll?.scrollLeft ?? 0,
          frame: null,
          edgeDirection: 0,
          edgeEntered: null,
          lastFrameTime: null,
        };
        onGestureChange(true);
        const tick = (now: number) => {
          const intent = gesture.current;
          if (!intent) return;
          if (
            intent.scroll &&
            Math.abs(intent.clientX - intent.x) > 2 * intent.scale
          ) {
            const rect = intent.scroll.getBoundingClientRect();
            const zone = Math.min(40, rect.width / 2);
            const depth =
              zone > 0 && intent.clientX < rect.left + zone
                ? -Math.min(1, (rect.left + zone - intent.clientX) / zone)
                : zone > 0 && intent.clientX > rect.right - zone
                  ? Math.min(1, (intent.clientX - rect.right + zone) / zone)
                  : 0;
            const direction = Math.sign(depth);
            if (!direction || direction !== intent.edgeDirection) {
              intent.edgeDirection = direction;
              intent.edgeEntered = direction ? now : null;
              intent.lastFrameTime = null;
            }
            if (
              intent.edgeEntered !== null &&
              now - intent.edgeEntered >= 100
            ) {
              const elapsed =
                Math.max(0, now - (intent.lastFrameTime ?? now)) / 1000;
              intent.lastFrameTime = now;
              const previous = intent.scroll.scrollLeft;
              intent.scroll.scrollLeft = Math.max(
                0,
                previous + (depth * 300 * elapsed) / intent.scale,
              );
              if (intent.scroll.scrollLeft !== previous)
                intent.preview(trimDelta(intent));
            }
          }
          intent.frame = requestAnimationFrame(tick);
        };
        gesture.current.frame = requestAnimationFrame(tick);
        e.currentTarget.setPointerCapture(e.pointerId);
      }}
      onPointerMove={(e) => {
        const intent = gesture.current;
        if (intent) {
          intent.clientX = e.clientX;
          intent.preview(trimDelta(intent));
        }
      }}
      onPointerUp={(e) => {
        e.stopPropagation();
        const intent = gesture.current;
        if (!intent) return;
        intent.clientX = e.clientX;
        const delta = trimDelta(intent);
        intent.finish();
        gesture.current = null;
        intent.preview(null);
        intent.submit(delta);
      }}
      onPointerCancel={() => {
        gesture.current?.finish();
        gesture.current?.preview(null);
        gesture.current = null;
      }}
      onLostPointerCapture={() => {
        gesture.current?.finish();
        gesture.current?.preview(null);
        gesture.current = null;
      }}
      onKeyDown={(e) => {
        if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
          e.preventDefault();
          e.stopPropagation();
          onResize(e.key === "ArrowLeft" ? -100 : 100);
        }
      }}
    />
  );
}
