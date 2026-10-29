"use client";
import { useCallback, useEffect, useRef, useState, useId } from "react";
import {
  createMediaExport,
  getMediaExport,
  listMediaExports,
  cancelMediaExport,
  retryMediaExport,
  previewMediaExport,
  downloadMediaExport,
} from "@/api/mediaExports";
import { RequestError } from "@/lib/request";
import { readErrorMessage } from "@/lib/read-error";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { createPortal } from "react-dom";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
  PopoverDescription,
} from "@/components/ui/popover";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import { Download, ChevronDown, LoaderCircle, Pencil, X } from "lucide-react";
import type { CanvasYjsLiveSession } from "../yjs-live-session";
import { requirePreview } from "@/components/media/library";
import { isProjectID } from "@/components/projects/project";

type ExportKind = NonNullable<API.MediatoolCreateInput["output_kind"]>;
const artifactKind = (kind?: string) =>
  ["media_bundle", "fcpxml", "xmeml"].includes(kind ?? "");
const artifactVersion = (kind?: string) =>
  ({ media_bundle: "1", fcpxml: "1.10", xmeml: "5" })[
    kind as "media_bundle" | "fcpxml" | "xmeml"
  ];
const active = (status?: API.MediatoolStatus) =>
  ["queued", "running", "cancel_requested"].includes(status ?? "");
function exportErrorMessage(cause: unknown, fallback: string): string {
  if (cause instanceof RequestError) {
    if (cause.status === 409) return "时间线或导出状态已更新，请重试。";
    if ([400, 422].includes(cause.status))
      return "导出设置或素材不可用，请检查后重试。";
    if (
      [401, 403, 404, 429].includes(cause.status) ||
      cause.status >= 500 ||
      [
        "dependency_unavailable",
        "network_error",
        "network_unavailable",
      ].includes(cause.code)
    )
      return readErrorMessage(cause);
    return fallback;
  }
  // Local validation and delivery failures already contain actionable Chinese copy.
  return cause instanceof Error && /\p{Script=Han}/u.test(cause.message)
    ? cause.message
    : fallback;
}

export function requireTimelineExport(
  value: API.ExportJobResponse,
  projectID: string,
  canvasID: string,
  nodeID: string,
  previous?: API.ExportJobResponse,
): API.ExportJobResponse {
  if (
    (previous &&
      (value.id !== previous.id ||
        value.revision! < previous.revision! ||
        value.source?.revision !== previous.source?.revision ||
        value.output_kind !== previous.output_kind)) ||
    !isProjectID(value.id) ||
    value.project_id !== projectID ||
    value.source?.canvas_id !== canvasID ||
    value.source?.node_id !== nodeID ||
    !Number.isSafeInteger(value.revision) ||
    value.revision! < 1 ||
    !Number.isSafeInteger(value.source?.revision) ||
    value.source!.revision! < 1 ||
    !["video", "audio", "media_bundle", "fcpxml", "xmeml"].includes(
      value.output_kind ?? "",
    ) ||
    (["review_required", "succeeded"].includes(value.status ?? "") &&
      (!(artifactKind(value.output_kind)
        ? isProjectID(value.artifact_id) && !value.asset_id
        : isProjectID(value.asset_id) && !value.artifact_id) ||
        !/^[a-f0-9]{64}$/i.test(value.sha256 ?? ""))) ||
    ![
      "queued",
      "running",
      "cancel_requested",
      "review_required",
      "succeeded",
      "failed",
      "cancelled",
    ].includes(value.status ?? "")
  )
    throw new Error("导出任务身份不匹配");
  return value;
}
export function requireTimelinePreview(
  value: API.ExportPreview,
  job: API.ExportJobResponse,
) {
  if (
    value.job_id !== job.id ||
    value.revision !== job.revision ||
    value.sha256 !== job.sha256
  )
    throw new Error("导出预览版本不匹配");
  if (artifactKind(job.output_kind)) {
    const artifact = value.artifact;
    if (
      !artifact ||
      artifact.id !== job.artifact_id ||
      artifact.job_id !== job.id ||
      artifact.project_id !== job.project_id ||
      artifact.attempt !== job.attempt ||
      artifact.format !== job.output_kind ||
      artifact.format_version !== artifactVersion(job.output_kind) ||
      artifact.sha256 !== job.sha256 ||
      artifact.mime_type !== "application/zip" ||
      artifact.file_name !== `timeline-${job.id}.zip` ||
      !Number.isSafeInteger(artifact.byte_size) ||
      artifact.byte_size! < 1 ||
      artifact.byte_size! > 500 * 1024 * 1024 ||
      !Number.isFinite(Date.parse(artifact.created_at ?? ""))
    )
      throw new Error("工程产物身份不匹配");
    if (
      !value.url ||
      !Number.isFinite(Date.parse(value.expires_at ?? "")) ||
      Date.parse(value.expires_at!) <= Date.now()
    )
      throw new Error("工程下载地址已失效");
    const url = new URL(value.url);
    if (
      !["http:", "https:"].includes(url.protocol) ||
      url.username ||
      url.password
    )
      throw new Error("工程下载地址无效");
  } else {
    if (
      value.asset?.id !== job.asset_id ||
      value.asset?.project_id !== job.project_id ||
      value.asset?.kind !== job.output_kind
    )
      throw new Error("导出预览版本不匹配");
    requirePreview(value, job.asset_id!);
  }
  return value;
}
export async function verifyTimelineDownload(blob: Blob, sha256: string) {
  if (!(blob instanceof Blob) || !blob.size) throw new Error("导出文件为空");
  const digest = await crypto.subtle.digest(
    "SHA-256",
    await blob.arrayBuffer(),
  );
  const actual = Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
  if (actual !== sha256.toLowerCase())
    throw new Error("导出文件校验不匹配，请重新下载");
}
export async function verifyArtifactDownload(
  blob: Blob,
  artifact: API.Artifact,
) {
  if (blob.size !== artifact.byte_size || blob.type !== "application/zip")
    throw new Error("工程文件类型或大小不匹配");
  await verifyTimelineDownload(blob, artifact.sha256!);
  const signature = new Uint8Array(await blob.slice(0, 4).arrayBuffer());
  if (signature.join(",") !== "80,75,3,4")
    throw new Error("工程文件不是有效的ZIP包");
}
export type TimelineExportDelivery = {
  jobID: string;
  assetID: string;
  fileName: string;
};
type ExportProps = {
  client: CanvasYjsLiveSession;
  nodeID: string;
  canvasID: string;
  projectID: string;
  disabled: boolean;
  hasVisual: boolean;
  aspectRatio: string;
  nodeTitle?: string;
  compact?: boolean;
  onAddToCanvas?: (result: TimelineExportDelivery) => Promise<void>;
};
export function TimelineExport(props: ExportProps) {
  return (
    <TimelineExportSession
      key={`${props.projectID}/${props.canvasID}/${props.nodeID}`}
      {...props}
    />
  );
}
function TimelineExportSession({
  client,
  nodeID,
  canvasID,
  projectID,
  disabled,
  hasVisual,
  nodeTitle = "时间线",
  compact = false,
  onAddToCanvas,
}: ExportProps) {
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState(false);
  const [fileName, setFileName] = useState(nodeTitle);
  const [destination, setDestination] = useState<"canvas" | "local">("canvas");
  const [resolution, setResolution] = useState(720);
  const [job, setJob] = useState<API.ExportJobResponse | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [delivery, setDelivery] = useState<
    "idle" | "working" | "canvas" | "local" | "failed"
  >("idle");
  const alive = useRef(true),
    inFlight = useRef(false),
    autoDeliver = useRef(false),
    jobRef = useRef<API.ExportJobResponse | null>(null);
  const intent = useRef<{
    key: string;
    input: API.MediatoolCreateInput;
  } | null>(null);
  const delivered = useRef(new Set<string>());
  const fieldID = useId();
  const running = active(job?.status);
  const accept = useCallback(
    (value: API.ExportJobResponse, previous?: API.ExportJobResponse) => {
      const accepted = requireTimelineExport(
        value,
        projectID,
        canvasID,
        nodeID,
        previous,
      );
      jobRef.current = accepted;
      setJob(accepted);
      return accepted;
    },
    [projectID, canvasID, nodeID],
  );
  useEffect(() => {
    alive.current = true;
    const abort = new AbortController();
    void listMediaExports(
      { pid: projectID, canvas_id: canvasID, node_id: nodeID, limit: 1 },
      { signal: abort.signal },
    )
      .then((page) => {
        if (abort.signal.aborted || jobRef.current) return;
        const latest = page.items?.find(
          (item) =>
            item.source?.canvas_id === canvasID &&
            item.source?.node_id === nodeID,
        );
        if (latest) accept(latest);
      })
      .catch((cause) => {
        if (!abort.signal.aborted)
          setError(exportErrorMessage(cause, "无法读取导出状态"));
      });
    return () => {
      alive.current = false;
      abort.abort();
    };
  }, [projectID, canvasID, nodeID, accept]);
  const deliver = useCallback(
    async (current: API.ExportJobResponse, repeat = false) => {
      if (current.status !== "succeeded" || !current.id) return;
      const receiptKey = `${current.id}/${current.attempt ?? 1}`;
      if (delivered.current.has(receiptKey) && !repeat) return;
      setDelivery("working");
      setError("");
      try {
        const name = current.settings?.file_name || nodeTitle;
        const location = artifactKind(current.output_kind)
          ? "local"
          : current.settings?.destination || "local";
        if (location === "canvas") {
          if (!onAddToCanvas || !current.asset_id)
            throw new Error("无法添加到画布，请重试");
          await onAddToCanvas({
            jobID: current.id,
            assetID: current.asset_id,
            fileName: name,
          });
        } else {
          const params = { job_id: current.id, project_id: projectID };
          const blob = await downloadMediaExport(params, {
            responseType: "blob",
          });
          if (!alive.current) return;
          if (artifactKind(current.output_kind)) {
            const receipt = requireTimelinePreview(
              await previewMediaExport(params),
              current,
            );
            await verifyArtifactDownload(blob, receipt.artifact!);
          } else await verifyTimelineDownload(blob, current.sha256!);
          if (!alive.current) return;
          const { saveAs } = await import("file-saver");
          saveAs(
            blob,
            `${name}.${artifactKind(current.output_kind) ? "zip" : current.output_kind === "audio" ? "m4a" : "mp4"}`,
          );
        }
        if (alive.current) {
          delivered.current.add(receiptKey);
          setDelivery(location);
        }
      } catch (cause) {
        if (alive.current) {
          setError(exportErrorMessage(cause, "导出交付失败，请重试"));
          setDelivery("failed");
        }
      }
    },
    [nodeTitle, onAddToCanvas, projectID],
  );
  const deliverRef = useRef(deliver);
  useEffect(() => {
    deliverRef.current = deliver;
  }, [deliver]);
  useEffect(() => {
    if (!running || !job?.id) return;
    const abort = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    let failures = 0;
    const poll = async () => {
      try {
        const previous = jobRef.current;
        const value = await getMediaExport(
          { job_id: job.id!, project_id: projectID },
          { signal: abort.signal },
        );
        if (abort.signal.aborted) return;
        const current = accept(value, previous ?? undefined);
        failures = 0;
        setError("");
        if (current.status === "succeeded" && autoDeliver.current) {
          autoDeliver.current = false;
          await deliverRef.current(current);
        }
        if (!active(current.status)) return;
      } catch (cause) {
        if (abort.signal.aborted) return;
        failures++;
        setError(exportErrorMessage(cause, "无法更新导出状态，正在重试"));
      }
      if (!abort.signal.aborted)
        timer = setTimeout(
          () => void poll(),
          Math.min(1500 * 2 ** failures, 10000),
        );
    };
    timer = setTimeout(() => void poll(), 1500);
    return () => {
      abort.abort();
      clearTimeout(timer);
    };
  }, [running, job?.id, projectID, accept]);
  async function perform(action: () => Promise<void>) {
    if (inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setError("");
    try {
      await action();
    } catch (cause) {
      if (alive.current) setError(exportErrorMessage(cause, "导出操作未完成"));
    } finally {
      inFlight.current = false;
      if (alive.current) setBusy(false);
    }
  }
  async function start(kind: ExportKind = "video") {
    if (kind !== "video") {
      setEditing(false);
      setOpen(false);
    }
    await perform(async () => {
      if (
        disabled ||
        (kind === "video" && !hasVisual) ||
        active(jobRef.current?.status)
      )
        throw new Error("当前时间线不可导出");
      if (intent.current && intent.current.input.output_kind !== kind)
        throw new Error("上次导出结果尚未确认，请重试原导出");
      if (!intent.current) {
        const name = fileName.trim();
        if (
          !name ||
          name.length > 120 ||
          new TextEncoder().encode(name).byteLength > 251 ||
          /[\\/:*?"<>|\x00-\x1f]/.test(name)
        )
          throw new Error("请输入有效的文件名");
        intent.current = {
          key: crypto.randomUUID(),
          input: {
            ...(await client.prepareTimeline(nodeID)),
            output_kind: kind,
            settings: {
              file_name: name,
              destination: artifactKind(kind) ? "local" : destination,
              resolution: resolution as API.ExportSettings["resolution"],
            },
          },
        };
      }
      if (!alive.current) return;
      const value = await createMediaExport(
        { pid: projectID },
        intent.current.input,
        { headers: { "Idempotency-Key": intent.current.key } },
      ).catch((cause: unknown) => {
        // A rejected command has no receipt; retry the latest saved source.
        // Unknown transport outcomes keep their original identity for replay.
        if (
          cause instanceof RequestError &&
          [400, 403, 404, 409, 422].includes(cause.status)
        )
          intent.current = null;
        throw cause;
      });
      if (!alive.current) return;
      const current = requireTimelineExport(value, projectID, canvasID, nodeID);
      if (
        current.source?.revision !== intent.current.input.revision ||
        current.output_kind !== kind
      )
        throw new Error("导出任务与冻结版本不匹配");
      accept(current);
      intent.current = null;
      setEditing(false);
      setOpen(false);
      setDelivery("idle");
      autoDeliver.current = true;
      if (current.status === "succeeded") {
        autoDeliver.current = false;
        await deliver(current);
      }
    });
  }
  async function control(action: "cancel" | "retry" | "refresh") {
    await perform(async () => {
      const previous = jobRef.current;
      if (!previous?.id) return;
      const params = { job_id: previous.id, project_id: projectID };
      if (action !== "refresh")
        await (action === "cancel" ? cancelMediaExport : retryMediaExport)(
          params,
          { project_id: projectID, revision: previous.revision },
          { headers: { "Idempotency-Key": crypto.randomUUID() } },
        );
      const value = await getMediaExport(params);
      if (!alive.current) return;
      const current = accept(value, previous);
      if (action === "retry") {
        setDelivery("idle");
        autoDeliver.current = true;
      }
      if (current.status === "succeeded" && autoDeliver.current) {
        autoDeliver.current = false;
        await deliver(current);
      }
    });
  }
  const status =
    delivery === "working"
      ? "正在交付…"
      : delivery === "canvas"
        ? "已添加到画布"
        : delivery === "local"
          ? "已开始下载"
          : {
              queued: "排队中…",
              running: "导出中…",
              cancel_requested: "正在取消…",
              cancelled: "已取消",
              failed: "导出失败",
              review_required: "请重新导出此旧任务",
              succeeded: "导出完成",
            }[job?.status ?? "queued"];
  const workingText =
    job?.settings?.destination === "local" || artifactKind(job?.output_kind)
      ? "下载中..."
      : "导出中...";
  const notice = !editing && (job || error || busy);
  return (
    <>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverAnchor asChild>
          <div
            className={
              compact
                ? "inline-flex items-center"
                : "inline-flex items-center rounded-lg bg-white/[0.06]"
            }
          >
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size={compact ? "icon-sm" : "sm"}
                    className={`${compact ? "" : "h-9"} ${hasVisual ? "" : "opacity-50"}`}
                    aria-label="导出"
                    aria-disabled={!hasVisual || undefined}
                    disabled={
                      disabled || busy || running || delivery === "working"
                    }
                    onClick={() => {
                      if (!hasVisual) return;
                      setFileName(nodeTitle);
                      setEditing(true);
                      setError("");
                      setOpen(!open);
                    }}
                    aria-expanded={open}
                    aria-haspopup="dialog"
                  >
                    <Download />
                    {!compact && "导出"}
                  </Button>
                </TooltipTrigger>
                {!hasVisual && (
                  <TooltipContent>当前时间线暂不支持此操作</TooltipContent>
                )}
              </Tooltip>
            </TooltipProvider>
            {!compact && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    className="h-9"
                    aria-label="更多导出方式"
                    disabled={
                      disabled || busy || running || delivery === "working"
                    }
                  >
                    <ChevronDown />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-56">
                  <DropdownMenuItem onSelect={() => void start("xmeml")}>
                    导出为 XML
                  </DropdownMenuItem>
                  <DropdownMenuItem onSelect={() => void start("media_bundle")}>
                    批量导出时间线素材
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem disabled title="剪映专业版交付协议核验中">
                    导出到剪映
                  </DropdownMenuItem>
                  <DropdownMenuItem onSelect={() => void start("xmeml")}>
                    导出到 DaVinci Resolve
                  </DropdownMenuItem>
                  <DropdownMenuItem onSelect={() => void start("xmeml")}>
                    导出到 Premiere
                  </DropdownMenuItem>
                  <DropdownMenuItem onSelect={() => void start("fcpxml")}>
                    导出到 Final Cut Pro
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </div>
        </PopoverAnchor>
        <PopoverContent
          align="end"
          sideOffset={8}
          data-canvas-shortcuts-ignore
          aria-labelledby={`${fieldID}-title`}
          className="w-80 gap-0 rounded-2xl border-0 bg-popover p-4 text-[13px] text-white ring-0"
          onPointerDown={(event) => event.stopPropagation()}
        >
          <PopoverHeader className="gap-1.5">
            <PopoverTitle id={`${fieldID}-title`}>导出设置</PopoverTitle>
            <PopoverDescription className="text-xs text-white/60">
              选择导出位置、分辨率和格式
            </PopoverDescription>
          </PopoverHeader>
          <div className="flex h-16 items-center justify-between gap-5">
            <Label
              className="shrink-0 font-normal text-white/80"
              htmlFor={`${fieldID}-name`}
            >
              文件名
            </Label>
            <div className="flex min-w-0 items-center gap-1">
              <Input
                id={`${fieldID}-name`}
                value={fileName}
                maxLength={120}
                onChange={(event) => setFileName(event.target.value)}
                className="h-8 min-w-0 border-transparent bg-transparent px-0 text-right text-xs shadow-none dark:bg-transparent"
              />
              <Pencil
                className="size-3 shrink-0 text-white/70"
                aria-hidden="true"
              />
            </div>
          </div>
          <div className="flex h-14 items-center justify-between border-b border-white/[0.06] pb-1">
            <Label
              className="font-normal text-white/80"
              htmlFor={`${fieldID}-destination`}
            >
              导出位置
            </Label>
            <Select
              value={destination}
              onValueChange={(value) =>
                setDestination(value as "canvas" | "local")
              }
            >
              <SelectTrigger
                id={`${fieldID}-destination`}
                className="h-8 w-auto gap-1 border-0 bg-transparent p-0 text-xs shadow-none dark:bg-transparent"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="canvas">添加到画布</SelectItem>
                <SelectItem value="local">下载到本地</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex h-16 items-center justify-between">
            <Label
              className="font-normal text-white/80"
              htmlFor={`${fieldID}-resolution`}
            >
              分辨率
            </Label>
            <Select
              value={String(resolution)}
              onValueChange={(value) => setResolution(Number(value))}
            >
              <SelectTrigger
                id={`${fieldID}-resolution`}
                className="h-8 w-auto gap-1 border-0 bg-transparent p-0 text-xs shadow-none dark:bg-transparent"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {[
                  [480, "480P"],
                  [720, "720P"],
                  [1080, "1080P"],
                  [1440, "2K"],
                  [2160, "4K"],
                ].map(([value, label]) => (
                  <SelectItem key={value} value={String(value)}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {error && (
            <p role="alert" className="mb-3 text-xs text-destructive">
              {error}
            </p>
          )}
          <div className="flex justify-end gap-3 pt-1">
            <Button
              variant="secondary"
              className="h-9 w-20 rounded-lg bg-white/[0.07] text-white hover:bg-white/10"
              onClick={() => setOpen(false)}
            >
              取消
            </Button>
            <Button
              className="h-9 w-20 rounded-lg bg-white text-black hover:bg-white/90"
              disabled={busy || running || !hasVisual || !fileName.trim()}
              onClick={() => void start("video")}
            >
              确认
            </Button>
          </div>
        </PopoverContent>
      </Popover>
      {notice &&
        createPortal(
          <section
            aria-label="时间线导出通知"
            data-canvas-shortcuts-ignore
            className="fixed top-8 left-1/2 z-[100] flex max-w-[calc(100vw-32px)] -translate-x-1/2 items-center gap-3 rounded-xl bg-secondary px-4 py-2.5 text-xs text-white shadow-lg"
            onPointerDown={(event) => event.stopPropagation()}
          >
            {(running || busy || delivery === "working") && (
              <LoaderCircle
                className="size-3 animate-spin"
                aria-hidden="true"
              />
            )}
            <span role="status">
              {running
                ? job?.status === "cancel_requested"
                  ? "正在取消..."
                  : workingText
                : busy
                  ? "正在准备..."
                  : status}
            </span>
            {error && (
              <span role="alert" className="max-w-64 text-destructive">
                {error}
              </span>
            )}
            {running && (
              <Button
                variant="ghost"
                size="sm"
                aria-label="取消导出"
                disabled={busy || job?.status === "cancel_requested"}
                onClick={() => void control("cancel")}
              >
                取消
              </Button>
            )}
            {["failed", "cancelled"].includes(job?.status ?? "") && (
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                onClick={() => void control("retry")}
              >
                重试导出
              </Button>
            )}
            {job?.status === "succeeded" && delivery !== "working" && (
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                onClick={() => {
                  if (job) void deliver(job, true);
                }}
              >
                {delivery === "failed"
                  ? "重试交付"
                  : job.settings?.destination === "canvas"
                    ? "添加到画布"
                    : "下载到本地"}
              </Button>
            )}
            {job?.status === "review_required" && (
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  setEditing(true);
                  setOpen(true);
                }}
              >
                重新导出
              </Button>
            )}
            {error && running && (
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                onClick={() => void control("refresh")}
              >
                刷新状态
              </Button>
            )}
            {!running && !busy && delivery !== "working" && (
              <Button
                size="icon-xs"
                variant="ghost"
                aria-label="关闭导出通知"
                onClick={() => setEditing(true)}
              >
                <X />
              </Button>
            )}
          </section>,
          document.body,
        )}
    </>
  );
}
