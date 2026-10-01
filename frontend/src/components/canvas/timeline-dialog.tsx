"use client";

import { useEffect, useRef, useState } from "react";
import {
  Download,
  Pause,
  Play,
  Plus,
  Scissors,
  Trash2,
  Upload,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { CanvasNodeType, type CanvasNodeData } from "./model";
import { getMediaPreview } from "./queries";
import {
  parseSrt,
  resegmentSrtEntries,
  serializeSrtEntries,
} from "./subtitles";
import {
  canPlaceAt,
  computeSnap,
  findNearestAvailablePlacement,
  splitTimelineClip,
  timelineDuration,
  timelineSchema,
  type TimelineClip,
  type TimelineProject,
  type TimelineTrack,
} from "./timeline";
import { TimelinePreview } from "./timeline-preview";
import { TimelineCropControl } from "./timeline-crop-control";
import { MediaExportPanel } from "./media-export-panel";
import { TranscriptionPanel } from "./transcription-panel";
import type { ExportSource } from "./media-export-model";
import type { MediaAsset } from "./queries";

const labels = {
  video: "视频",
  audio: "音频",
  image: "图片",
  text: "文字",
  subtitle: "字幕",
};
type Props = {
  node: CanvasNodeData;
  nodes: CanvasNodeData[];
  projectId: string;
  readOnly: boolean;
  onClose: () => void;
  onSave: (value: TimelineProject) => Promise<boolean>;
  canvasId: string;
  source: () => ExportSource;
  onExportResult: (asset: MediaAsset) => Promise<boolean>;
};
function downloadText(content: string, name: string) {
  const url = URL.createObjectURL(
    new Blob([content], { type: "application/x-subrip;charset=utf-8" }),
  );
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = name;
  anchor.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export function TimelineDialog({
  node,
  nodes,
  projectId,
  readOnly,
  onClose,
  onSave,
  canvasId,
  source,
  onExportResult,
}: Props) {
  const [draft, setDraft] = useState(() => timelineSchema.parse(node.timeline));
  const [timeMs, setTimeMs] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [selectedId, setSelectedId] = useState<string>();
  const [sourceId, setSourceId] = useState("");
  const [adding, setAdding] = useState(false);
  const [saving, setSaving] = useState(false);
  const [exportBusy, setExportBusy] = useState(false);
  const [transcriptionBusy, setTranscriptionBusy] = useState(false);
  const [error, setError] = useState("");
  const [scale, setScale] = useState(0.04);
  const [newKind, setNewKind] = useState<TimelineTrack["kind"]>("video");
  const input = useRef<HTMLInputElement>(null);
  const activeAbort = useRef<AbortController | undefined>(undefined);
  const drag = useRef<{
    id: string;
    pointer: number;
    original: number;
    x: number;
    next: number;
    element: HTMLElement;
    frame?: number;
  } | null>(null);
  const duration = timelineDuration(draft);
  const visibleDuration = Math.max(duration, 15000);
  const selected = draft.clips.find((clip) => clip.id === selectedId);
  const selectedTrack = draft.tracks.find(
    (track) => track.id === selected?.trackId,
  );
  const disabled =
    readOnly || saving || adding || exportBusy || transcriptionBusy;
  const available = nodes.filter((item) =>
    [
      CanvasNodeType.Image,
      CanvasNodeType.Audio,
      CanvasNodeType.Video,
      CanvasNodeType.Text,
    ].includes(item.type),
  );
  useEffect(
    () => () => {
      activeAbort.current?.abort();
      if (drag.current?.frame) cancelAnimationFrame(drag.current.frame);
    },
    [],
  );
  useEffect(() => {
    if (!playing) return;
    const start = performance.now() - timeMs;
    const timer = setInterval(() => {
      const next = performance.now() - start;
      if (next >= duration) {
        setTimeMs(duration);
        setPlaying(false);
      } else setTimeMs(next);
    }, 80);
    return () => clearInterval(timer);
    // Playback anchors when the user presses play; seeking pauses playback.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [playing, duration]);
  function patchClip(patch: Partial<TimelineClip>) {
    if (!selected || selectedTrack?.locked || disabled) return;
    setDraft((value) => ({
      ...value,
      clips: value.clips.map((clip) =>
        clip.id === selected.id ? { ...clip, ...patch } : clip,
      ),
    }));
  }
  async function addSource() {
    const source = available.find((item) => item.id === sourceId);
    if (!source || disabled || draft.clips.length >= 1000) return;
    setPlaying(false);
    setAdding(true);
    setError("");
    const controller = new AbortController();
    activeAbort.current = controller;
    try {
      const kind = source.type as TimelineClip["kind"];
      const asset = source.assetId
        ? (await getMediaPreview(projectId, source.assetId, controller.signal))
            .asset
        : null;
      if (controller.signal.aborted) return;
      if (
        asset &&
        (asset.id !== source.assetId ||
          asset.project_id !== projectId ||
          asset.kind !== kind)
      )
        throw new Error("素材身份不匹配。");
      const sourceDurationMs =
        kind === "video" || kind === "audio" ? asset?.duration_ms : null;
      if (
        (kind === "video" || kind === "audio") &&
        (!sourceDurationMs || sourceDurationMs < 100)
      )
        throw new Error("素材缺少有效时长，无法加入时间线。");
      const durationMs = sourceDurationMs
        ? Math.min(sourceDurationMs, 10000)
        : 5000;
      const track = draft.tracks.find(
        (item) => item.kind === kind && !item.locked,
      );
      if (!track) throw new Error("请先添加对应类型的未锁定轨道。");
      const placement = findNearestAvailablePlacement({
        targetStartMs: Math.round(timeMs),
        durationMs,
        trackId: track.id,
        clips: draft.clips,
      });
      const clip: TimelineClip = {
        id: crypto.randomUUID(),
        trackId: track.id,
        kind,
        nodeId: source.id,
        assetId: source.assetId ?? null,
        title: source.title,
        startMs: placement.startMs,
        durationMs,
        sourceStartMs: 0,
        sourceDurationMs: sourceDurationMs ?? null,
        volume: 1,
        fadeInMs: 0,
        fadeOutMs: 0,
        text: source.metadata?.content ?? "",
      };
      const next = timelineSchema.parse({
        ...draft,
        clips: [...draft.clips, clip],
      });
      setDraft(next);
      setSelectedId(clip.id);
    } catch (failure) {
      if (!controller.signal.aborted)
        setError(failure instanceof Error ? failure.message : "素材加入失败。");
    } finally {
      if (!controller.signal.aborted) setAdding(false);
    }
  }
  async function importSubtitles(file?: File) {
    if (!file || disabled) return;
    try {
      if (file.size > 1_000_000) throw new Error("SRT文件最大1MB。");
      const entries = parseSrt(await file.text());
      if (!entries.length) throw new Error("文件没有字幕。");
      const track: TimelineTrack = {
        id: crypto.randomUUID(),
        kind: "subtitle",
        label: file.name.slice(0, 128),
        locked: false,
        visible: true,
        muted: false,
      };
      const clips = entries.map((entry): TimelineClip => ({
        id: crypto.randomUUID(),
        trackId: track.id,
        kind: "subtitle",
        nodeId: null,
        assetId: null,
        title: `字幕 ${entry.index}`,
        startMs: entry.startMs,
        durationMs: entry.endMs - entry.startMs,
        sourceStartMs: 0,
        sourceDurationMs: null,
        volume: 1,
        fadeInMs: 0,
        fadeOutMs: 0,
        text: entry.text,
      }));
      setDraft(
        timelineSchema.parse({
          ...draft,
          tracks: [...draft.tracks, track],
          clips: [...draft.clips, ...clips],
        }),
      );
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "字幕导入失败。");
    }
  }
  function save() {
    const parsed = timelineSchema.safeParse(draft);
    if (!parsed.success) {
      setError(parsed.error.issues[0].message);
      return;
    }
    setSaving(true);
    setPlaying(false);
    setError("");
    void onSave(parsed.data)
      .then((saved) => {
        if (saved) onClose();
        else setError("时间线尚未确认保存，请修复画布错误后重试。");
      })
      .catch(() => setError("时间线保存失败。"))
      .finally(() => setSaving(false));
  }
  function endDrag(cancel = false) {
    const current = drag.current;
    if (!current) return;
    if (current.frame) cancelAnimationFrame(current.frame);
    current.element.style.left = `${current.original * scale}px`;
    drag.current = null;
    if (cancel) return;
    const clip = draft.clips.find((value) => value.id === current.id)!;
    const snapped = computeSnap({
      candidateMs: Math.max(0, Math.round(current.next)),
      playheadMs: timeMs,
      clips: draft.clips,
      excludeClipId: clip.id,
      pxPerMs: scale,
      thresholdPx: 8,
      enabled: draft.snapping,
    }).snappedMs;
    if (
      !canPlaceAt({
        trackId: clip.trackId,
        startMs: snapped,
        durationMs: clip.durationMs,
        excludeClipId: clip.id,
        clips: draft.clips,
      }).ok
    ) {
      setError("移动后与同轨片段重叠，请选择空隙或另一轨道。");
      return;
    }
    setError("");
    setDraft((value) => ({
      ...value,
      clips: value.clips.map((item) =>
        item.id === clip.id ? { ...item, startMs: snapped } : item,
      ),
    }));
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !saving && !exportBusy && !transcriptionBusy) onClose();
      }}
    >
      <DialogContent
        className="flex max-h-[94dvh] w-[98vw] flex-col overflow-y-auto sm:max-w-[1400px] [&>*]:shrink-0"
        onEscapeKeyDown={(event) => {
          if (saving || exportBusy || transcriptionBusy) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>{node.title} · 时间线</DialogTitle>
          <DialogDescription>
            剪辑、音轨、字幕与文字使用项目素材。修改保存为画布工具状态。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-5 lg:grid-cols-[1fr_340px]">
          <TimelinePreview
            timeline={draft}
            nodes={nodes}
            projectId={projectId}
            timeMs={timeMs}
            playing={playing}
          />
          <div className="space-y-3">
            <div className="flex gap-2">
              <Field className="flex-1">
                <FieldLabel>画面比例</FieldLabel>
                <Select
                  value={draft.aspectRatio}
                  disabled={disabled}
                  onValueChange={(
                    aspectRatio: TimelineProject["aspectRatio"],
                  ) => setDraft((value) => ({ ...value, aspectRatio }))}
                >
                  <SelectTrigger aria-label="时间线画面比例">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {["16:9", "9:16", "1:1"].map((ratio) => (
                      <SelectItem key={ratio} value={ratio}>
                        {ratio}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field className="flex-1">
                <FieldLabel>帧率</FieldLabel>
                <Select
                  value={String(draft.fps)}
                  disabled={disabled}
                  onValueChange={(fps) =>
                    setDraft((value) => ({
                      ...value,
                      fps: Number(fps) as TimelineProject["fps"],
                    }))
                  }
                >
                  <SelectTrigger aria-label="时间线帧率">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {[24, 25, 30, 60].map((fps) => (
                      <SelectItem key={fps} value={String(fps)}>
                        {fps} fps
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </div>
            <Field>
              <FieldLabel>画布素材</FieldLabel>
              <Select
                value={sourceId}
                disabled={disabled}
                onValueChange={setSourceId}
              >
                <SelectTrigger aria-label="加入时间线的素材">
                  <SelectValue placeholder="选择素材" />
                </SelectTrigger>
                <SelectContent>
                  {available.map((item) => (
                    <SelectItem key={item.id} value={item.id}>
                      {item.title} · {labels[item.type as keyof typeof labels]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Button
              className="w-full"
              variant="outline"
              disabled={disabled || !sourceId || draft.clips.length >= 1000}
              onClick={() => {
                void addSource();
              }}
            >
              <Plus />
              {adding ? "读取素材…" : "加入时间线"}
            </Button>
            <div className="flex gap-2">
              <Button
                variant="outline"
                disabled={disabled || draft.tracks.length >= 32}
                onClick={() => input.current?.click()}
              >
                <Upload />
                导入 SRT
              </Button>
              <Button
                variant="outline"
                disabled={!draft.clips.some((clip) => clip.kind === "subtitle")}
                onClick={() =>
                  downloadText(
                    serializeSrtEntries(
                      draft.clips
                        .filter((clip) => clip.kind === "subtitle")
                        .sort((left, right) => left.startMs - right.startMs)
                        .map((clip, index) => ({
                          index: index + 1,
                          startMs: clip.startMs,
                          endMs: clip.startMs + clip.durationMs,
                          text: clip.text,
                        })),
                    ),
                    `${node.title}.srt`,
                  )
                }
              >
                <Download />
                导出 SRT
              </Button>
            </div>
            <input
              ref={input}
              type="file"
              accept=".srt,application/x-subrip"
              hidden
              onChange={(event) => {
                void importSubtitles(event.target.files?.[0]);
                event.target.value = "";
              }}
            />
            <div className="flex items-center gap-2">
              <Checkbox
                id="timeline-snapping"
                checked={draft.snapping}
                disabled={disabled}
                onCheckedChange={(checked) =>
                  setDraft((value) => ({
                    ...value,
                    snapping: checked === true,
                  }))
                }
              />
              <label htmlFor="timeline-snapping" className="text-sm">
                吸附片段边缘和播放头
              </label>
            </div>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <Button
            size="icon"
            variant="outline"
            aria-label={playing ? "暂停时间线" : "播放时间线"}
            disabled={!duration}
            onClick={() => {
              if (timeMs >= duration) setTimeMs(0);
              setPlaying(!playing);
            }}
          >
            {playing ? <Pause /> : <Play />}
          </Button>
          <span className="text-xs tabular-nums">
            {(timeMs / 1000).toFixed(2)} / {(duration / 1000).toFixed(2)} s
          </span>
          <input
            aria-label="时间线播放头"
            type="range"
            min={0}
            max={Math.max(1, duration)}
            step={1}
            value={timeMs}
            className="min-w-40 flex-1"
            onChange={(event) => {
              setPlaying(false);
              setTimeMs(Number(event.target.value));
            }}
          />
          <label className="flex items-center gap-2 text-xs">
            缩放
            <input
              aria-label="时间线缩放"
              type="range"
              min={0.005}
              max={0.2}
              step={0.005}
              value={scale}
              onChange={(event) => setScale(Number(event.target.value))}
            />
          </label>
        </div>
        <div className="max-h-64 shrink-0 overflow-auto rounded border">
          <div style={{ minWidth: 220 + visibleDuration * scale }}>
            <div className="sticky top-0 z-10 flex h-7 bg-muted text-xs">
              <div className="sticky left-0 w-52 shrink-0 bg-muted px-3">
                轨道
              </div>
              <div className="relative flex-1">
                {Array.from(
                  { length: Math.min(100, Math.ceil(visibleDuration / 5000)) },
                  (_, index) => (
                    <span
                      key={index}
                      className="absolute border-l pl-1"
                      style={{ left: index * 5000 * scale }}
                    >
                      {index * 5}s
                    </span>
                  ),
                )}
              </div>
            </div>
            {draft.tracks.map((track) => (
              <div key={track.id} className="flex h-16 border-t">
                <div className="sticky left-0 z-10 flex w-52 shrink-0 items-center gap-2 border-r bg-background px-3">
                  <span className="w-16 truncate text-xs" title={track.label}>
                    {track.label}
                  </span>
                  <Checkbox
                    aria-label={`${track.label}可见`}
                    disabled={disabled}
                    checked={track.visible}
                    onCheckedChange={(visible) =>
                      setDraft((value) => ({
                        ...value,
                        tracks: value.tracks.map((item) =>
                          item.id === track.id
                            ? { ...item, visible: visible === true }
                            : item,
                        ),
                      }))
                    }
                  />
                  <Checkbox
                    aria-label={`${track.label}静音`}
                    disabled={disabled}
                    checked={track.muted}
                    onCheckedChange={(muted) =>
                      setDraft((value) => ({
                        ...value,
                        tracks: value.tracks.map((item) =>
                          item.id === track.id
                            ? { ...item, muted: muted === true }
                            : item,
                        ),
                      }))
                    }
                  />
                  <Checkbox
                    aria-label={`${track.label}锁定`}
                    disabled={disabled}
                    checked={track.locked}
                    onCheckedChange={(locked) =>
                      setDraft((value) => ({
                        ...value,
                        tracks: value.tracks.map((item) =>
                          item.id === track.id
                            ? { ...item, locked: locked === true }
                            : item,
                        ),
                      }))
                    }
                  />
                  <Button
                    size="icon"
                    variant="ghost"
                    aria-label={`删除${track.label}轨道`}
                    disabled={disabled || track.locked}
                    onClick={() =>
                      setDraft((value) => ({
                        ...value,
                        tracks: value.tracks.filter(
                          (item) => item.id !== track.id,
                        ),
                        clips: value.clips.filter(
                          (clip) => clip.trackId !== track.id,
                        ),
                      }))
                    }
                  >
                    <Trash2 />
                  </Button>
                </div>
                <div
                  className="relative flex-1 bg-muted/20"
                  onClick={(event) => {
                    if (event.target === event.currentTarget) {
                      setPlaying(false);
                      setTimeMs(
                        Math.max(
                          0,
                          (event.clientX -
                            event.currentTarget.getBoundingClientRect().left) /
                            scale,
                        ),
                      );
                    }
                  }}
                >
                  <div
                    className="pointer-events-none absolute inset-y-0 z-10 w-px bg-destructive"
                    style={{ left: timeMs * scale }}
                  />
                  {draft.clips
                    .filter((clip) => clip.trackId === track.id)
                    .map((clip) => (
                      <button
                        key={clip.id}
                        className={`absolute top-2 h-12 touch-none truncate rounded border px-2 text-left text-xs ${selectedId === clip.id ? "border-foreground bg-foreground text-background" : "bg-background"}`}
                        style={{
                          left: clip.startMs * scale,
                          width: Math.max(18, clip.durationMs * scale),
                        }}
                        title={`${clip.title}: ${(clip.startMs / 1000).toFixed(2)}s`}
                        onClick={() => setSelectedId(clip.id)}
                        onPointerDown={(event) => {
                          setSelectedId(clip.id);
                          if (disabled || track.locked) return;
                          setPlaying(false);
                          event.currentTarget.setPointerCapture(
                            event.pointerId,
                          );
                          drag.current = {
                            id: clip.id,
                            pointer: event.pointerId,
                            original: clip.startMs,
                            x: event.clientX,
                            next: clip.startMs,
                            element: event.currentTarget,
                          };
                        }}
                        onPointerMove={(event) => {
                          const current = drag.current;
                          if (!current || current.pointer !== event.pointerId)
                            return;
                          current.next = Math.max(
                            0,
                            current.original +
                              (event.clientX - current.x) / scale,
                          );
                          if (!current.frame)
                            current.frame = requestAnimationFrame(() => {
                              if (drag.current) {
                                drag.current.element.style.left = `${drag.current.next * scale}px`;
                                drag.current.frame = undefined;
                              }
                            });
                        }}
                        onPointerUp={() => endDrag()}
                        onPointerCancel={() => endDrag(true)}
                      >
                        {clip.title || labels[clip.kind]}
                      </button>
                    ))}
                </div>
              </div>
            ))}
          </div>
        </div>
        <div className="flex gap-2">
          <Select
            value={newKind}
            disabled={disabled}
            onValueChange={(kind: TimelineTrack["kind"]) => setNewKind(kind)}
          >
            <SelectTrigger className="w-28" aria-label="新增轨道类型">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {Object.entries(labels).map(([kind, label]) => (
                <SelectItem key={kind} value={kind}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            variant="outline"
            disabled={disabled || draft.tracks.length >= 32}
            onClick={() =>
              setDraft((value) => ({
                ...value,
                tracks: [
                  ...value.tracks,
                  {
                    id: crypto.randomUUID(),
                    kind: newKind,
                    label: `${labels[newKind]} ${value.tracks.filter((track) => track.kind === newKind).length + 1}`,
                    locked: false,
                    muted: false,
                    visible: true,
                  },
                ],
              }))
            }
          >
            <Plus />
            轨道
          </Button>
        </div>
        {selected ? (
          <div className="grid gap-3 rounded border p-3 sm:grid-cols-4">
            <Field>
              <FieldLabel htmlFor="timeline-clip-title">片段名称</FieldLabel>
              <Input
                id="timeline-clip-title"
                value={selected.title}
                maxLength={128}
                disabled={disabled || selectedTrack?.locked}
                onChange={(event) => patchClip({ title: event.target.value })}
              />
            </Field>
            <Field>
              <FieldLabel>目标轨道</FieldLabel>
              <Select
                value={selected.trackId}
                disabled={disabled || selectedTrack?.locked}
                onValueChange={(trackId) => patchClip({ trackId })}
              >
                <SelectTrigger aria-label="片段轨道">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {draft.tracks
                    .filter(
                      (track) => track.kind === selected.kind && !track.locked,
                    )
                    .map((track) => (
                      <SelectItem key={track.id} value={track.id}>
                        {track.label}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            </Field>
            {(
              [
                ["startMs", "时间线起点 (ms)"],
                ["durationMs", "片段时长 (ms)"],
                ["sourceStartMs", "素材裁剪起点 (ms)"],
                ["volume", "音量 (0～2)"],
                ["fadeInMs", "淡入 (ms)"],
                ["fadeOutMs", "淡出 (ms)"],
              ] as const
            ).map(([field, label]) => (
              <Field key={field}>
                <FieldLabel htmlFor={`timeline-${field}`}>{label}</FieldLabel>
                <Input
                  id={`timeline-${field}`}
                  type="number"
                  min={0}
                  step={field === "volume" ? 0.1 : 1}
                  value={selected[field]}
                  disabled={disabled || selectedTrack?.locked}
                  onChange={(event) =>
                    patchClip({ [field]: Number(event.target.value) })
                  }
                />
              </Field>
            ))}
            {selected.kind === "text" || selected.kind === "subtitle" ? (
              <Field className="sm:col-span-3">
                <FieldLabel htmlFor="timeline-clip-text">文字</FieldLabel>
                <Textarea
                  id="timeline-clip-text"
                  value={selected.text}
                  maxLength={10000}
                  disabled={disabled || selectedTrack?.locked}
                  onChange={(event) => patchClip({ text: event.target.value })}
                />
              </Field>
            ) : null}
            {selected.kind === "video" || selected.kind === "image" ? (
              <TimelineCropControl
                key={selected.id}
                clip={selected}
                projectId={projectId}
                assetId={
                  selected.assetId ??
                  nodes.find((item) => item.id === selected.nodeId)?.assetId
                }
                disabled={disabled || Boolean(selectedTrack?.locked)}
                onChange={(crop) => patchClip({ crop })}
              />
            ) : null}
            <div className="flex items-end gap-2">
              <Button
                variant="outline"
                disabled={disabled || selectedTrack?.locked}
                onClick={() => {
                  const split = splitTimelineClip(selected, timeMs);
                  if (!split) {
                    setError("播放头需距片段两端至少100ms。");
                    return;
                  }
                  setDraft((value) => ({
                    ...value,
                    clips: value.clips.flatMap((clip) =>
                      clip.id === selected.id ? split : [clip],
                    ),
                  }));
                  setSelectedId(split[1].id);
                }}
              >
                <Scissors />
                分割
              </Button>
              <Button
                variant="outline"
                disabled={disabled || selectedTrack?.locked}
                onClick={() => {
                  setDraft((value) => ({
                    ...value,
                    clips: value.clips.filter(
                      (clip) => clip.id !== selected.id,
                    ),
                  }));
                  setSelectedId(undefined);
                }}
              >
                <Trash2 />
              </Button>
            </div>
          </div>
        ) : null}
        <div className="flex flex-wrap items-end gap-3">
          <Field className="w-28">
            <FieldLabel htmlFor="subtitle-font-size">字幕字号</FieldLabel>
            <Input
              id="subtitle-font-size"
              type="number"
              min={12}
              max={96}
              value={draft.subtitleStyle.fontSize}
              disabled={disabled}
              onChange={(event) =>
                setDraft((value) => ({
                  ...value,
                  subtitleStyle: {
                    ...value.subtitleStyle,
                    fontSize: Number(event.target.value),
                  },
                }))
              }
            />
          </Field>
          <Field className="w-20">
            <FieldLabel htmlFor="subtitle-color">颜色</FieldLabel>
            <Input
              id="subtitle-color"
              type="color"
              value={draft.subtitleStyle.color}
              disabled={disabled}
              onChange={(event) =>
                setDraft((value) => ({
                  ...value,
                  subtitleStyle: {
                    ...value.subtitleStyle,
                    color: event.target.value,
                  },
                }))
              }
            />
          </Field>
          <Field className="w-28">
            <FieldLabel>字幕位置</FieldLabel>
            <Select
              value={draft.subtitleStyle.position}
              disabled={disabled}
              onValueChange={(
                position: TimelineProject["subtitleStyle"]["position"],
              ) =>
                setDraft((value) => ({
                  ...value,
                  subtitleStyle: { ...value.subtitleStyle, position },
                }))
              }
            >
              <SelectTrigger aria-label="字幕位置">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="top">顶部</SelectItem>
                <SelectItem value="center">中间</SelectItem>
                <SelectItem value="bottom">底部</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <Button
            variant="outline"
            disabled={
              disabled ||
              !selected ||
              selected.kind !== "subtitle" ||
              selectedTrack?.locked
            }
            onClick={() => {
              if (!selected) return;
              const split = resegmentSrtEntries(
                [
                  {
                    index: 1,
                    startMs: selected.startMs,
                    endMs: selected.startMs + selected.durationMs,
                    text: selected.text,
                  },
                ],
                35,
              ).map((entry) => ({
                ...selected,
                id: crypto.randomUUID(),
                startMs: entry.startMs,
                durationMs: entry.endMs - entry.startMs,
                text: entry.text,
              }));
              try {
                setDraft(
                  timelineSchema.parse({
                    ...draft,
                    clips: draft.clips.flatMap((clip) =>
                      clip.id === selected.id ? split : [clip],
                    ),
                  }),
                );
              } catch {
                setError("片段时长太短，无法安全切分长字幕。");
              }
            }}
          >
            按标点切分长字幕
          </Button>
        </div>
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        <TranscriptionPanel
          projectId={projectId}
          canvasId={canvasId}
          nodes={nodes}
          draft={draft}
          source={source}
          disabled={readOnly || saving || adding || exportBusy}
          onPersist={onSave}
          onApplied={setDraft}
          onBusy={setTranscriptionBusy}
        />
        <MediaExportPanel
          projectId={projectId}
          canvasId={canvasId}
          nodeId={node.id}
          draft={draft}
          source={source}
          disabled={disabled}
          onPersist={onSave}
          onImport={onExportResult}
          onBusy={setExportBusy}
        />
        <DialogFooter>
          <Button
            variant="outline"
            disabled={saving || exportBusy || transcriptionBusy}
            onClick={onClose}
          >
            关闭
          </Button>
          <Button disabled={disabled} onClick={save}>
            {saving ? "保存中…" : "保存时间线"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
