"use client";

import { useRef, useState } from "react";
import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ApiError } from "@/lib/request";
import { downloadMediaExport } from "@/api/mediaExports";
import {
  MEDIA_EXPORTS_KEY,
  controlTimelineExport,
  createTimelineExport,
  downloadTimelineSubtitles,
  listTimelineExports,
} from "./media-export-queries";
import type { ExportJob, ExportSource } from "./media-export-model";
import { MediaExportPreviewDialog } from "./media-export-preview-dialog";
import { getMediaPreview, type MediaAsset } from "./queries";
import { timelineSchema, type TimelineProject } from "./timeline";

const statusLabel: Record<ExportJob["status"], string> = {
  queued: "等待导出",
  running: "正在导出",
  review_required: "等待检查成片",
  succeeded: "导出已审核",
  failed: "导出失败",
  cancel_requested: "正在取消",
  cancelled: "已取消",
};
export function MediaExportPanel({
  projectId,
  canvasId,
  nodeId,
  draft,
  source,
  disabled,
  onPersist,
  onImport,
  onBusy,
}: {
  projectId: string;
  canvasId: string;
  nodeId: string;
  draft: TimelineProject;
  source: () => ExportSource;
  disabled: boolean;
  onPersist: (value: TimelineProject) => Promise<boolean>;
  onImport: (asset: MediaAsset) => Promise<boolean>;
  onBusy: (busy: boolean) => void;
}) {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [selected, setSelected] = useState<ExportJob>();
  const [unknownCreate, setUnknownCreate] = useState(false);
  const [outputKind, setOutputKind] =
    useState<ExportJob["output_kind"]>("video");
  const active = useRef(false);
  const attempt = useRef<{
    source: ExportSource;
    key: string;
    outputKind: ExportJob["output_kind"];
  } | null>(null);
  const commands = useRef(new Map<string, string>());
  const cache = useQueryClient();
  const queryKey = [...MEDIA_EXPORTS_KEY, projectId, canvasId, nodeId];
  const jobs = useInfiniteQuery({
    queryKey,
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      listTimelineExports(projectId, canvasId, nodeId, pageParam, signal),
    getNextPageParam: (data) => data.next_cursor ?? undefined,
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
  async function create() {
    if (active.current || pending || (disabled && !unknownCreate)) return;
    active.current = true;
    setPending(true);
    onBusy(true);
    setError("");
    setNotice("");
    try {
      if (!attempt.current) {
        const next = timelineSchema.parse(draft);
        if (!next.clips.length) throw new Error("请先加入实际素材或文字片段。");
        if (!(await onPersist(next)))
          throw new Error("时间线尚未确认保存，导出未启动。");
        attempt.current = {
          source: source(),
          key: crypto.randomUUID(),
          outputKind,
        };
      }
      const result = await createTimelineExport(
        projectId,
        attempt.current.source,
        attempt.current.key,
        attempt.current.outputKind,
      );
      setNotice(`已创建导出任务，使用画布修订 ${result.source.revision}。`);
      attempt.current = null;
      setUnknownCreate(false);
      await cache.invalidateQueries({ queryKey });
    } catch (failure) {
      const unknown =
        failure instanceof ApiError &&
        (failure.status === 0 || failure.status >= 500) &&
        Boolean(attempt.current);
      setUnknownCreate(unknown);
      if (!unknown) attempt.current = null;
      setError(
        unknown
          ? "导出创建结果未知。请按原幂等请求核验，避免重复导出。"
          : failure instanceof Error
            ? failure.message
            : "导出创建失败。",
      );
    } finally {
      active.current = false;
      setPending(false);
      onBusy(false);
    }
  }
  async function control(job: ExportJob, action: "cancel" | "retry") {
    if (active.current || pending || disabled) return;
    active.current = true;
    setPending(true);
    onBusy(true);
    setError("");
    const identity = `${job.id}:${job.revision}:${action}`;
    if (!commands.current.has(identity))
      commands.current.set(identity, crypto.randomUUID());
    try {
      await controlTimelineExport(job, action, commands.current.get(identity)!);
      await cache.invalidateQueries({ queryKey });
    } catch (failure) {
      setError(
        failure instanceof Error
          ? failure.message
          : "导出请求未确认，请重试原请求。",
      );
    } finally {
      active.current = false;
      setPending(false);
      onBusy(false);
    }
  }
  async function addResult(job: ExportJob) {
    if (
      active.current ||
      job.status !== "succeeded" ||
      !job.asset_id ||
      pending ||
      disabled
    )
      return;
    active.current = true;
    setPending(true);
    onBusy(true);
    setError("");
    try {
      const preview = await getMediaPreview(projectId, job.asset_id);
      if (
        preview.asset.id !== job.asset_id ||
        preview.asset.project_id !== projectId ||
        preview.asset.kind !== job.output_kind
      )
        throw new Error("导出素材身份不匹配。");
      if (!(await onImport(preview.asset)))
        throw new Error("输出尚未确认加入画布，请修复保存错误后重试。");
      setNotice(
        job.output_kind === "audio"
          ? "已将审核后的真实音频加入画布。"
          : "已将审核后的真实成片加入画布。",
      );
    } catch (failure) {
      setError(
        failure instanceof Error ? failure.message : "成片加入画布失败。",
      );
    } finally {
      active.current = false;
      setPending(false);
      onBusy(false);
    }
  }
  async function subtitles(job: ExportJob) {
    if (active.current || pending) return;
    active.current = true;
    setPending(true);
    setError("");
    try {
      const blob = await downloadTimelineSubtitles(job);
      const url = URL.createObjectURL(blob),
        anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = `timeline-${job.id}.srt`;
      anchor.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "字幕下载失败。");
    } finally {
      active.current = false;
      setPending(false);
    }
  }
  async function downloadResult(job: ExportJob) {
    if (
      active.current ||
      pending ||
      job.status !== "succeeded" ||
      !job.asset_id ||
      !job.sha256 ||
      job.project_id !== projectId
    )
      return;
    active.current = true;
    setPending(true);
    setError("");
    try {
      const blob: unknown = await downloadMediaExport(
        { job_id: job.id, project_id: projectId },
        { responseType: "blob", timeout: 0 },
      );
      if (
        !(blob instanceof Blob) ||
        blob.size === 0 ||
        blob.type !== (job.output_kind === "audio" ? "audio/mp4" : "video/mp4")
      )
        throw new ApiError(502, "invalid_response");
      const url = URL.createObjectURL(blob);
      try {
        const anchor = document.createElement("a");
        anchor.href = url;
        anchor.download = `timeline-${job.id}.${job.output_kind === "audio" ? "m4a" : "mp4"}`;
        anchor.click();
      } finally {
        setTimeout(() => URL.revokeObjectURL(url), 1000);
      }
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "成片下载失败。");
    } finally {
      active.current = false;
      setPending(false);
    }
  }
  return (
    <section
      aria-label="时间线正式导出"
      className="space-y-3 rounded border p-3"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 className="text-sm font-medium">导出视频与音频</h3>
          <p className="text-xs text-muted-foreground">
            保存当前时间线，导出 MP4 或提取真实音轨为
            M4A。完成后检查实际输出，再加入素材或下载。
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Select
            value={outputKind}
            disabled={pending || disabled || unknownCreate}
            onValueChange={(value) =>
              setOutputKind(value as ExportJob["output_kind"])
            }
          >
            <SelectTrigger aria-label="导出类型" className="w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="video">视频 MP4</SelectItem>
              <SelectItem value="audio">音频 M4A</SelectItem>
            </SelectContent>
          </Select>
          <Button
            disabled={
              pending ||
              (disabled && !unknownCreate) ||
              (!unknownCreate && !draft.clips.length)
            }
            onClick={() => {
              void create();
            }}
          >
            {pending
              ? "请求处理中…"
              : unknownCreate
                ? "核验原导出创建请求"
                : outputKind === "audio"
                  ? "保存并提取 M4A"
                  : "保存并导出 MP4"}
          </Button>
        </div>
      </div>
      {error || jobs.error ? (
        <p role="alert" className="text-sm text-destructive">
          {error || "导出任务读取失败，请重新读取。"}
        </p>
      ) : null}
      {notice ? (
        <p role="status" className="text-sm">
          {notice}
        </p>
      ) : null}
      <Button
        size="sm"
        variant="ghost"
        disabled={jobs.isFetching || pending}
        onClick={() => {
          void jobs.refetch();
        }}
      >
        读取最新导出状态
      </Button>
      {jobs.isPending ? <p className="text-xs">正在读取导出记录…</p> : null}
      {jobs.data?.pages
        .flatMap((page) => page.items)
        .map((job) => (
          <article key={job.id} className="space-y-2 rounded border p-3">
            <div className="flex flex-wrap justify-between gap-2 text-xs">
              <span>
                {job.output_kind === "audio" ? "音频" : "视频"} ·{" "}
                {job.output_kind === "audio" && job.status === "review_required"
                  ? "等待检查音频"
                  : statusLabel[job.status]}{" "}
                · 第 {job.attempt} 次 · 画布修订 {job.source.revision}
              </span>
              <time dateTime={job.created_at}>
                {new Date(job.created_at).toLocaleString("zh-CN")}
              </time>
            </div>
            <Progress
              value={job.progress}
              aria-label={`导出任务进度 ${job.progress}%`}
            />
            <p className="text-xs text-muted-foreground">
              {job.progress}%
              {job.failure_code
                ? ` · ${job.failure_code === "no_audio_stream" ? "此素材没有可提取音轨" : job.failure_code}`
                : ""}
            </p>
            <div className="flex flex-wrap gap-2">
              {["review_required", "succeeded"].includes(job.status) ? (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={pending}
                  onClick={() => setSelected(job)}
                >
                  {job.status === "review_required"
                    ? job.output_kind === "audio"
                      ? "检查并审核音频"
                      : "检查并审核成片"
                    : job.output_kind === "audio"
                      ? "预览音频"
                      : "预览成片"}
                </Button>
              ) : null}
              {job.status === "succeeded" ? (
                <>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={pending || disabled}
                    onClick={() => {
                      void addResult(job);
                    }}
                  >
                    {job.output_kind === "audio"
                      ? "音频加入画布"
                      : "成片加入画布"}
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={pending}
                    onClick={() => {
                      void downloadResult(job);
                    }}
                  >
                    {job.output_kind === "audio" ? "下载 M4A" : "下载 MP4"}
                  </Button>
                </>
              ) : null}
              {["queued", "running", "review_required"].includes(job.status) ? (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={pending || disabled}
                  onClick={() => {
                    void control(job, "cancel");
                  }}
                >
                  取消导出
                </Button>
              ) : null}
              {["failed", "cancelled"].includes(job.status) ? (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={pending || disabled}
                  onClick={() => {
                    void control(job, "retry");
                  }}
                >
                  重试冻结时间线
                </Button>
              ) : null}
              <Button
                size="sm"
                variant="ghost"
                disabled={pending}
                onClick={() => {
                  void subtitles(job);
                }}
              >
                下载冻结字幕 SRT
              </Button>
            </div>
          </article>
        ))}
      {jobs.hasNextPage ? (
        <Button
          size="sm"
          variant="outline"
          disabled={jobs.isFetchingNextPage || pending}
          onClick={() => {
            void jobs.fetchNextPage();
          }}
        >
          加载更早导出记录
        </Button>
      ) : null}
      {selected ? (
        <MediaExportPreviewDialog
          key={`${selected.id}:${selected.revision}`}
          job={selected}
          readOnly={disabled}
          onClose={() => setSelected(undefined)}
          onReviewed={() => setSelected(undefined)}
          onBusy={onBusy}
        />
      ) : null}
    </section>
  );
}
