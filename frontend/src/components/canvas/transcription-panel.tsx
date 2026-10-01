"use client";

import { useEffect, useRef, useState } from "react";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { ZodError } from "zod";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Progress } from "@/components/ui/progress";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { ApiError } from "@/lib/request";
import { CanvasNodeType, type CanvasNodeData } from "./model";
import {
  MEDIA_TRANSCRIPTIONS_KEY,
  controlMediaTranscription,
  createMediaTranscription,
  downloadMediaTranscriptionSubtitles,
  getCanvas,
  getMediaPreview,
  getMediaTranscriptionResult,
  listMediaTranscriptions,
} from "./queries";
import {
  applyTranscript,
  transcriptSchema,
  type Transcript,
  type TranscriptionJob,
  type TranscriptionResult,
  type TranscriptionSource,
} from "./transcription-model";
import { timelineSchema, type TimelineProject } from "./timeline";

type PendingCommand =
  | {
      kind: "create";
      source: TranscriptionSource;
      language: string;
      key: string;
    }
  | {
      kind: "control";
      job: TranscriptionJob;
      action: "cancel" | "retry";
      key: string;
    };
const labels: Record<TranscriptionJob["status"], string> = {
  queued: "等待识别",
  running: "正在识别",
  succeeded: "字幕稿已生成",
  failed: "识别失败",
  cancel_requested: "等待识别结束后取消",
  cancelled: "已取消",
};
const failureLabels: Record<string, string> = {
  no_audio_stream: "此视频没有音轨，无法提取字幕。",
  no_speech: "未识别到语音。",
  invalid_result: "识别结果没有通过时间与文本校验。",
  inference_unknown: "识别请求的结果尚未确认。",
  activity_unknown: "识别工作是否结束尚未确认。",
  dependency_unavailable: "字幕识别服务暂不可用。",
};

export function TranscriptionPanel({
  projectId,
  canvasId,
  nodes,
  draft,
  source,
  disabled,
  onPersist,
  onApplied,
  onBusy,
}: {
  projectId: string;
  canvasId: string;
  nodes: CanvasNodeData[];
  draft: TimelineProject;
  source: () => TranscriptionSource;
  disabled: boolean;
  onPersist: (value: TimelineProject) => Promise<boolean>;
  onApplied: (value: TimelineProject) => void;
  onBusy: (busy: boolean) => void;
}) {
  const resources = nodes.filter(
    (node) =>
      (node.type === CanvasNodeType.Audio ||
        node.type === CanvasNodeType.Video) &&
      node.assetId,
  );
  const [sourceId, setSourceId] = useState(() => resources[0]?.id ?? "all");
  const [language, setLanguage] = useState("auto");
  const [languageCode, setLanguageCode] = useState("");
  const [pending, setPending] = useState(false);
  const [reviewBusy, setReviewBusy] = useState(false);
  const [unknownKind, setUnknownKind] = useState<PendingCommand["kind"] | null>(
    null,
  );
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [selectedJobId, setSelectedJobId] = useState<string>();
  const command = useRef<PendingCommand | null>(null),
    active = useRef(false);
  const cache = useQueryClient();
  const queryKey = [...MEDIA_TRANSCRIPTIONS_KEY, projectId, canvasId, sourceId];
  const jobs = useInfiniteQuery({
    queryKey,
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      listMediaTranscriptions(
        projectId,
        canvasId,
        sourceId === "all" ? undefined : sourceId,
        pageParam,
        signal,
      ),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.pages.some((page) =>
        page.items.some((job) =>
          ["queued", "running", "cancel_requested"].includes(job.status),
        ),
      )
        ? 2000
        : false,
  });
  const values = jobs.data?.pages.flatMap((page) => page.items) ?? [];
  const selected = values.find(
    (job) => job.id === selectedJobId && job.status === "succeeded",
  );
  const result = useQuery({
    queryKey: [
      ...MEDIA_TRANSCRIPTIONS_KEY,
      projectId,
      "result",
      selected?.id,
      selected?.revision,
    ],
    queryFn: ({ signal }) => getMediaTranscriptionResult(selected!, signal),
    enabled: Boolean(selected),
    retry: false,
    staleTime: 0,
  });
  const picked = resources.find((node) => node.id === sourceId);
  const unknown = unknownKind !== null;
  const blocked = pending || unknown || reviewBusy;
  useEffect(() => {
    if (!blocked) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [blocked]);
  async function submit(replay = false) {
    if (active.current || pending || reviewBusy || (!replay && disabled))
      return;
    if (replay && !command.current) return;
    active.current = true;
    setPending(true);
    onBusy(true);
    setError("");
    setNotice("");
    try {
      if (!command.current) {
        if (!picked?.assetId)
          throw new Error("请选择已保存的正式音频或视频素材。");
        const selectedLanguage =
          language === "custom" ? languageCode.trim().toLowerCase() : language;
        if (language === "custom" && !/^[a-z]{2,3}$/.test(selectedLanguage))
          throw new Error("请填写 Whisper 支持的两或三字母语言码。");
        if (!(await onPersist(timelineSchema.parse(draft))))
          throw new Error("时间线尚未确认保存，字幕提取未启动。");
        const saved = source();
        if (saved.canvas_id !== canvasId)
          throw new Error("保存来源与当前画布不一致。");
        command.current = {
          kind: "create",
          source: {
            canvas_id: canvasId,
            node_id: picked.id,
            revision: saved.revision,
          },
          language: selectedLanguage,
          key: crypto.randomUUID(),
        };
      }
      const request = command.current;
      if (request.kind === "create") {
        const job = await createMediaTranscription(
          projectId,
          request.source,
          request.language,
          request.key,
        );
        setNotice(`已创建字幕提取任务，使用画布修订 ${job.source.revision}。`);
      } else {
        await controlMediaTranscription(
          request.job,
          request.action,
          request.key,
        );
        setNotice(
          request.action === "cancel"
            ? "已请求取消。正在进行的识别须结束后才能确认已取消。"
            : "已受理重新识别。",
        );
      }
      command.current = null;
      setUnknownKind(null);
      await cache.invalidateQueries({ queryKey });
    } catch (failure) {
      const uncertain =
        failure instanceof ApiError &&
        (failure.status === 0 || failure.status >= 500) &&
        Boolean(command.current);
      setUnknownKind(uncertain ? command.current!.kind : null);
      if (!uncertain) command.current = null;
      setError(
        uncertain
          ? "请求结果未知，请核验原字幕请求，避免重复识别。"
          : failure instanceof Error
            ? failure.message
            : "字幕提取请求失败。",
      );
    } finally {
      active.current = false;
      setPending(false);
      onBusy(Boolean(command.current));
    }
  }
  function control(job: TranscriptionJob, action: "cancel" | "retry") {
    if (blocked || disabled || active.current) return;
    command.current = {
      kind: "control",
      job: structuredClone(job),
      action,
      key: crypto.randomUUID(),
    };
    void submit(true);
  }
  async function download(job: TranscriptionJob) {
    if (active.current || blocked) return;
    active.current = true;
    setError("");
    try {
      const blob = await downloadMediaTranscriptionSubtitles(job);
      const url = URL.createObjectURL(blob),
        anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = `transcription-${job.id}.srt`;
      anchor.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "字幕稿下载失败。");
    } finally {
      active.current = false;
    }
  }
  return (
    <section
      aria-label="时间线字幕提取"
      className="flex flex-col gap-3 rounded border p-3"
    >
      <div>
        <h3 className="text-sm font-medium">从音视频提取字幕</h3>
        <p className="text-xs text-muted-foreground">
          识别素材原件的完整音轨。字幕稿须复核，再按所选片段剪裁并保存到字幕轨。
        </p>
      </div>
      <div className="flex flex-wrap gap-2">
        <Select
          value={sourceId}
          disabled={blocked || disabled}
          onValueChange={(value) => {
            setSourceId(value);
            setSelectedJobId(undefined);
          }}
        >
          <SelectTrigger aria-label="字幕提取素材" className="w-60">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="all">查看所有转写来源</SelectItem>
              {resources.map((node) => (
                <SelectItem key={node.id} value={node.id}>
                  {node.title}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Select
          value={language}
          disabled={blocked || disabled}
          onValueChange={setLanguage}
        >
          <SelectTrigger aria-label="识别语言" className="w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="auto">自动识别语言</SelectItem>
              <SelectItem value="zh">中文</SelectItem>
              <SelectItem value="en">英语</SelectItem>
              <SelectItem value="ja">日语</SelectItem>
              <SelectItem value="ko">韩语</SelectItem>
              <SelectItem value="custom">其他语言码</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
        {language === "custom" ? (
          <Field className="w-40">
            <FieldLabel htmlFor="transcription-language-code">
              识别语言码
            </FieldLabel>
            <Input
              id="transcription-language-code"
              value={languageCode}
              maxLength={3}
              placeholder="如 de、fr、yue"
              disabled={blocked || disabled}
              onChange={(event) => setLanguageCode(event.target.value)}
            />
          </Field>
        ) : null}
        <Button
          disabled={
            pending || reviewBusy || (!unknown && (disabled || !picked))
          }
          onClick={() => void submit(unknown)}
        >
          {pending
            ? "请求处理中…"
            : unknown
              ? unknownKind === "create"
                ? "核验原字幕创建请求"
                : "核验原字幕控制请求"
              : "保存并提取字幕"}
        </Button>
      </div>
      {unknown ? (
        <p className="text-xs text-muted-foreground">
          核验期间保留原请求，时间线暂时锁定。关闭或刷新可能丢失尚未确认的请求。
        </p>
      ) : null}
      {error || jobs.error || result.error ? (
        <Alert variant="destructive">
          <AlertTitle>字幕操作未完成</AlertTitle>
          <AlertDescription>
            {error ||
              (result.error
                ? "字幕稿读取失败，来源可能已变化，请重新读取。"
                : "字幕任务读取失败，请重试。")}
          </AlertDescription>
        </Alert>
      ) : null}
      {notice ? (
        <p role="status" className="text-sm">
          {notice}
        </p>
      ) : null}
      <Button
        variant="ghost"
        size="sm"
        disabled={jobs.isFetching || blocked}
        onClick={() => void jobs.refetch()}
      >
        读取最新字幕状态
      </Button>
      {jobs.isPending ? <Skeleton className="h-12" /> : null}
      <SpeechJobs
        jobs={values}
        nodes={nodes}
        disabled={disabled || blocked}
        onControl={control}
        onReview={setSelectedJobId}
        onDownload={download}
      />
      {jobs.hasNextPage ? (
        <Button
          variant="outline"
          size="sm"
          disabled={jobs.isFetchingNextPage || blocked}
          onClick={() => void jobs.fetchNextPage()}
        >
          读取更多字幕任务
        </Button>
      ) : null}
      {selected && result.isPending ? <Skeleton className="h-24" /> : null}
      {selected && result.data ? (
        <TranscriptReview
          key={`${selected.id}:${result.data.revision}:${result.data.sha256}`}
          job={selected}
          result={result.data}
          projectId={projectId}
          canvasId={canvasId}
          nodes={nodes}
          timeline={draft}
          disabled={disabled || blocked}
          onPersist={onPersist}
          onApplied={onApplied}
          onBusy={(busy) => {
            setReviewBusy(busy);
            onBusy(busy);
          }}
        />
      ) : null}
    </section>
  );
}

function SpeechJobs({
  jobs,
  nodes,
  disabled,
  onControl,
  onReview,
  onDownload,
}: {
  jobs: TranscriptionJob[];
  nodes: CanvasNodeData[];
  disabled: boolean;
  onControl: (job: TranscriptionJob, action: "cancel" | "retry") => void;
  onReview: (id: string) => void;
  onDownload: (job: TranscriptionJob) => void;
}) {
  const scroll = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line react-hooks/incompatible-library -- Virtual measurements remain owned by TanStack Virtual.
  const virtual = useVirtualizer({
    count: jobs.length,
    getScrollElement: () => scroll.current,
    estimateSize: () => 140,
    overscan: 3,
    enabled: jobs.length > 100,
  });
  const renderJob = (job: TranscriptionJob) => (
    <article
      className="flex flex-col gap-2 rounded border p-3"
      aria-label={`字幕任务 ${job.id}`}
    >
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <Badge variant="secondary">
          {job.stage === "awaiting_reconciliation"
            ? "结果待核验"
            : labels[job.status]}
        </Badge>
        <span>
          {nodes.find((node) => node.id === job.source.node_id)?.title ??
            "原转写素材"}{" "}
          · 第 {job.attempt} 次 · 画布修订 {job.source.revision}
        </span>
      </div>
      {job.stage === "awaiting_reconciliation" ? (
        <p className="text-xs text-muted-foreground">
          识别是否结束尚未确认，不能重新提交或宣称取消完成。
        </p>
      ) : (
        <Progress
          value={job.progress}
          aria-label={`字幕任务阶段 ${job.progress}%`}
        />
      )}
      {job.failure_code ? (
        <p className="text-xs text-destructive">
          {failureLabels[job.failure_code] ??
            "字幕提取未完成，请核对来源与服务状态。"}
        </p>
      ) : null}
      <div className="flex flex-wrap gap-2">
        {job.status === "succeeded" ? (
          <>
            <Button
              size="sm"
              variant="outline"
              disabled={disabled}
              onClick={() => onReview(job.id)}
            >
              查看字幕稿
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={disabled}
              onClick={() => onDownload(job)}
            >
              下载原识别 SRT
            </Button>
          </>
        ) : null}
        {["queued", "running"].includes(job.status) ? (
          <Button
            size="sm"
            variant="outline"
            disabled={disabled}
            onClick={() => onControl(job, "cancel")}
          >
            请求取消识别
          </Button>
        ) : null}
        {["failed", "cancelled"].includes(job.status) &&
        job.stage !== "awaiting_reconciliation" ? (
          <Button
            size="sm"
            variant="outline"
            disabled={disabled}
            onClick={() => onControl(job, "retry")}
          >
            重新识别
          </Button>
        ) : null}
      </div>
    </article>
  );
  return jobs.length > 100 ? (
    <div ref={scroll} className="max-h-96 overflow-y-auto">
      <div className="relative" style={{ height: virtual.getTotalSize() }}>
        {virtual.getVirtualItems().map((item) => (
          <div
            key={jobs[item.index].id}
            data-index={item.index}
            ref={virtual.measureElement}
            className="absolute inset-x-0 top-0 pb-2"
            style={{ transform: `translateY(${item.start}px)` }}
          >
            {renderJob(jobs[item.index])}
          </div>
        ))}
      </div>
    </div>
  ) : (
    <div className="flex flex-col gap-2">
      {jobs.map((job) => (
        <div key={job.id}>{renderJob(job)}</div>
      ))}
    </div>
  );
}

function TranscriptReview({
  job,
  result,
  projectId,
  canvasId,
  nodes,
  timeline,
  disabled,
  onPersist,
  onApplied,
  onBusy,
}: {
  job: TranscriptionJob;
  result: TranscriptionResult;
  projectId: string;
  canvasId: string;
  nodes: CanvasNodeData[];
  timeline: TimelineProject;
  disabled: boolean;
  onPersist: (value: TimelineProject) => Promise<boolean>;
  onApplied: (value: TimelineProject) => void;
  onBusy: (busy: boolean) => void;
}) {
  const [draft, setDraft] = useState<Transcript>(() =>
    structuredClone(result.draft),
  );
  const [clipId, setClipId] = useState("");
  const [trackId, setTrackId] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const active = useRef(false),
    scroll = useRef<HTMLDivElement>(null);
  const clips = timeline.clips.filter(
    (clip) =>
      (clip.kind === "audio" || clip.kind === "video") &&
      ((clip.assetId === result.source_asset_id &&
        (!clip.nodeId || clip.nodeId === job.source.node_id)) ||
        (!clip.assetId && clip.nodeId === job.source.node_id)),
  );
  const tracks = timeline.tracks.filter(
    (track) => track.kind === "subtitle" && !track.locked,
  );
  const original = useQuery({
    queryKey: [
      "canvas",
      "transcription-original",
      projectId,
      result.source_asset_id,
      result.source_asset_revision,
    ],
    queryFn: async ({ signal }) => {
      const preview = await getMediaPreview(
        projectId,
        result.source_asset_id,
        signal,
      );
      if (
        preview.asset.id !== result.source_asset_id ||
        preview.asset.project_id !== projectId ||
        preview.asset.revision !== result.source_asset_revision ||
        !["audio", "video"].includes(preview.asset.kind)
      )
        throw new ApiError(502, "invalid_response");
      return preview;
    },
    retry: false,
    staleTime: 0,
  });
  // eslint-disable-next-line react-hooks/incompatible-library -- Editable cue measurements remain owned by TanStack Virtual.
  const virtual = useVirtualizer({
    count: draft.segments.length,
    getScrollElement: () => scroll.current,
    estimateSize: () => 180,
    overscan: 3,
    enabled: draft.segments.length > 100,
  });
  function edit(index: number, patch: Partial<Transcript["segments"][number]>) {
    setConfirmed(false);
    setError("");
    setDraft((value) => ({
      ...value,
      segments: value.segments.map((segment, i) =>
        i === index ? { ...segment, ...patch } : segment,
      ),
    }));
  }
  async function adopt() {
    if (
      active.current ||
      pending ||
      disabled ||
      !confirmed ||
      !clipId ||
      !trackId
    )
      return;
    active.current = true;
    setPending(true);
    onBusy(true);
    setError("");
    setNotice("");
    try {
      const checked = transcriptSchema.parse(draft);
      const [fresh, preview, document] = await Promise.all([
        getMediaTranscriptionResult(job),
        getMediaPreview(projectId, result.source_asset_id),
        getCanvas(canvasId),
      ]);
      const current = document.nodes.find(
        (node) => node.id === job.source.node_id,
      );
      const known = nodes.find((node) => node.id === job.source.node_id);
      if (
        fresh.sha256 !== result.sha256 ||
        fresh.revision !== result.revision ||
        fresh.source_sha256 !== result.source_sha256 ||
        fresh.source_asset_id !== result.source_asset_id ||
        fresh.source_asset_revision !== result.source_asset_revision ||
        preview.asset.id !== result.source_asset_id ||
        preview.asset.project_id !== projectId ||
        preview.asset.revision !== result.source_asset_revision ||
        document.id !== canvasId ||
        document.projectId !== projectId ||
        current?.assetId !== result.source_asset_id ||
        known?.assetId !== result.source_asset_id ||
        current.type !== preview.asset.kind ||
        ![CanvasNodeType.Audio, CanvasNodeType.Video].includes(current.type)
      )
        throw new Error("转写原件或画布来源已变化，字幕未采用。");
      const next = applyTranscript(
        timeline,
        checked,
        clipId,
        trackId,
        result.source_asset_id,
        job.source.node_id,
      );
      if (!(await onPersist(next)))
        throw new Error("字幕尚未确认保存，请先核验画布保存结果。");
      onApplied(next);
      setConfirmed(false);
      setNotice("已保存复核后的字幕，只替换所选字幕轨。");
    } catch (failure) {
      setError(
        failure instanceof ZodError
          ? "请检查字幕文字与时间，起终点须有效、不得重叠或超过素材时长。"
          : failure instanceof Error
            ? failure.message
            : "字幕采用失败。",
      );
    } finally {
      active.current = false;
      setPending(false);
      onBusy(false);
    }
  }
  function cue(index: number) {
    const segment = draft.segments[index];
    return (
      <div className="flex flex-col gap-2 rounded border p-2">
        <div className="flex flex-wrap gap-2">
          <Field>
            <FieldLabel htmlFor={`speech-${job.id}-${index}-start`}>
              字幕 {index + 1} 起点 ms
            </FieldLabel>
            <Input
              id={`speech-${job.id}-${index}-start`}
              type="number"
              min={0}
              max={draft.duration_ms}
              value={segment.start_ms}
              disabled={pending || disabled}
              onChange={(event) =>
                edit(index, { start_ms: Number(event.target.value) })
              }
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`speech-${job.id}-${index}-end`}>
              字幕 {index + 1} 终点 ms
            </FieldLabel>
            <Input
              id={`speech-${job.id}-${index}-end`}
              type="number"
              min={0}
              max={draft.duration_ms}
              value={segment.end_ms}
              disabled={pending || disabled}
              onChange={(event) =>
                edit(index, { end_ms: Number(event.target.value) })
              }
            />
          </Field>
        </div>
        <Field>
          <FieldLabel htmlFor={`speech-${job.id}-${index}-text`}>
            字幕 {index + 1} 文字
          </FieldLabel>
          <Textarea
            id={`speech-${job.id}-${index}-text`}
            value={segment.text}
            disabled={pending || disabled}
            onChange={(event) => edit(index, { text: event.target.value })}
          />
        </Field>
      </div>
    );
  }
  return (
    <section
      aria-label="字幕稿复核"
      className="flex flex-col gap-3 rounded border p-3"
    >
      <h4 className="text-sm font-medium">复核字幕稿 · {draft.language}</h4>
      {original.isPending ? <Skeleton className="h-12" /> : null}
      {original.error ? (
        <p role="alert" className="text-sm text-destructive">
          转写原件暂不可播放，请核对当前素材状态。
        </p>
      ) : null}
      {original.data?.asset.kind === "audio" ? (
        <audio
          aria-label="转写音频原件"
          controls
          preload="metadata"
          src={original.data.url}
          className="w-full"
        />
      ) : original.data?.asset.kind === "video" ? (
        <video
          aria-label="转写视频原件"
          controls
          preload="metadata"
          src={original.data.url}
          className="max-h-64 w-full"
        />
      ) : null}
      <p className="text-xs text-muted-foreground">
        共 {draft.segments.length}{" "}
        条。请听取原素材，逐条核对文字与时间；采用时只保留所选片段范围内的字幕。
      </p>
      <div className="flex flex-wrap gap-2">
        <Select
          value={clipId}
          disabled={pending || disabled}
          onValueChange={(value) => {
            setClipId(value);
            setConfirmed(false);
          }}
        >
          <SelectTrigger aria-label="采用的音视频片段" className="w-60">
            <SelectValue placeholder="选择摆放片段" />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {clips.map((clip) => (
                <SelectItem key={clip.id} value={clip.id}>
                  {clip.title}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Select
          value={trackId}
          disabled={pending || disabled}
          onValueChange={(value) => {
            setTrackId(value);
            setConfirmed(false);
          }}
        >
          <SelectTrigger aria-label="采用的字幕轨" className="w-60">
            <SelectValue placeholder="选择未锁定字幕轨" />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {tracks.map((track) => (
                <SelectItem key={track.id} value={track.id}>
                  {track.label}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>
      {!clips.length ? (
        <p className="text-xs">
          当前时间线没有对应原件片段，请先加入同一素材。
        </p>
      ) : null}
      {!tracks.length ? (
        <p className="text-xs">请先创建未锁定的字幕轨道。</p>
      ) : null}
      <div ref={scroll} className="max-h-96 overflow-y-auto">
        {draft.segments.length > 100 ? (
          <div className="relative" style={{ height: virtual.getTotalSize() }}>
            {virtual.getVirtualItems().map((item) => (
              <div
                key={item.index}
                data-index={item.index}
                ref={virtual.measureElement}
                className="absolute inset-x-0 top-0 pb-2"
                style={{ transform: `translateY(${item.start}px)` }}
              >
                {cue(item.index)}
              </div>
            ))}
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {draft.segments.map((_, index) => (
              <div key={index}>{cue(index)}</div>
            ))}
          </div>
        )}
      </div>
      <Field orientation="horizontal">
        <Checkbox
          id={`speech-${job.id}-confirmed`}
          checked={confirmed}
          disabled={pending || disabled}
          onCheckedChange={(value) => setConfirmed(value === true)}
        />
        <FieldLabel htmlFor={`speech-${job.id}-confirmed`}>
          已复核字幕文字与时间，并同意替换所选字幕轨
        </FieldLabel>
      </Field>
      {error ? (
        <Alert variant="destructive">
          <AlertTitle>字幕未采用</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      {notice ? (
        <p role="status" className="text-sm">
          {notice}
        </p>
      ) : null}
      <Button
        disabled={pending || disabled || !confirmed || !clipId || !trackId}
        onClick={() => void adopt()}
      >
        {pending ? "保存字幕中…" : "复核并保存字幕"}
      </Button>
    </section>
  );
}
