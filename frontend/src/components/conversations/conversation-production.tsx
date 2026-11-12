"use client";

import { useEffect, useRef, useState } from "react";
import useSWR from "swr";
import { saveAs } from "file-saver";
import {
  getMediaExport,
  previewMediaExport,
  reviewMediaExport,
  downloadMediaExport,
} from "@/api/mediaExports";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { isProjectID } from "@/components/projects/project";
import { money } from "@/components/generation/contracts";
import { readErrorMessage } from "@/lib/read-error";
import {
  MediaExportStatus,
  artifactKind,
  requireTimelineExport,
  requireTimelinePreview,
  verifyTimelineDownload,
  verifyArtifactDownload,
} from "@/components/canvas/timeline/export-contract";

function record(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}
type Delivery = {
  delivery_id: string;
  canvas_id: string;
  timeline_node_id: string;
  video_export_id: string;
  project_export_id: string;
};
function delivery(value: unknown): value is Delivery {
  return (
    record(value) &&
    [
      value.delivery_id,
      value.canvas_id,
      value.timeline_node_id,
      value.video_export_id,
      value.project_export_id,
    ].every(isProjectID)
  );
}

export function ConversationProduction({
  facts,
  projectID,
  rootRunID,
  writable,
  onRefresh,
}: {
  facts: unknown;
  projectID: string;
  rootRunID: string;
  writable: boolean;
  onRefresh: () => void;
}) {
  if (!record(facts)) return null;
  const cost = record(facts.cost) ? facts.cost : undefined;
  const validCost =
    cost?.currency === "CNY" &&
    cost.unit === "micros" &&
    [
      cost.authorized_limit,
      cost.pending_commitment,
      cost.settled_charge,
      cost.remaining,
    ].every(
      (value) =>
        typeof value === "number" && Number.isSafeInteger(value) && value >= 0,
    );
  const deliveries = Array.isArray(facts.deliveries)
    ? facts.deliveries.filter(delivery)
    : [];
  return (
    <div className="space-y-3">
      {validCost && (
        <p className="text-xs text-muted-foreground">
          本轮上限 {money(Number(cost!.authorized_limit))} · 已结算{" "}
          {money(Number(cost!.settled_charge))} · 待结算{" "}
          {money(Number(cost!.pending_commitment))} · 剩余{" "}
          {money(Number(cost!.remaining))}
        </p>
      )}
      {deliveries.map((item) => (
        <section
          key={item.delivery_id}
          aria-label="单镜交付"
          className="space-y-3 rounded-xl border p-4"
        >
          <h3 className="font-medium">单镜交付</h3>
          {[item.video_export_id, item.project_export_id].map((id) => (
            <DeliveryFile
              key={`${rootRunID}/${id}`}
              id={id}
              writable={writable}
              projectID={projectID}
              canvasID={item.canvas_id}
              nodeID={item.timeline_node_id}
              onRefresh={onRefresh}
            />
          ))}
        </section>
      ))}
    </div>
  );
}

function DeliveryFile({
  id,
  projectID,
  canvasID,
  nodeID,
  writable,
  onRefresh,
}: {
  id: string;
  projectID: string;
  canvasID: string;
  nodeID: string;
  writable: boolean;
  onRefresh: () => void;
}) {
  const abort = useRef(new AbortController());
  useEffect(() => {
    const controller = new AbortController();
    abort.current = controller;
    return () => controller.abort();
  }, []);
  const query = useSWR(
    ["agent-delivery-export", projectID, id],
    async () =>
      requireTimelineExport(
        await getMediaExport(
          { job_id: id, project_id: projectID },
          { signal: abort.current.signal },
        ),
        projectID,
        canvasID,
        nodeID,
      ),
    {
      refreshInterval: (job) =>
        job &&
        [
          MediaExportStatus.Queued,
          MediaExportStatus.Running,
          MediaExportStatus.CancelRequested,
          MediaExportStatus.ReviewRequired,
        ].includes(job.status as typeof MediaExportStatus.Queued)
          ? 2000
          : 0,
    },
  );
  const [preview, setPreview] = useState<API.ExportPreview>();
  const [viewed, setViewed] = useState<string>();
  const [checked, setChecked] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const flight = useRef(false);
  const review = useRef<{ key: string; input: API.ReviewInput } | null>(null);
  const job = query.data;
  const version = job ? `${job.revision}/${job.sha256}` : "";
  const currentPreview =
    job &&
    preview &&
    preview.revision === job.revision &&
    preview.sha256 === job.sha256
      ? preview
      : undefined;
  async function perform(work: () => Promise<void>) {
    if (flight.current) return;
    flight.current = true;
    setBusy(true);
    setError("");
    try {
      await work();
    } catch (cause) {
      if (!abort.current.signal.aborted) setError(readErrorMessage(cause));
    } finally {
      flight.current = false;
      if (!abort.current.signal.aborted) setBusy(false);
    }
  }
  async function inspect() {
    if (!job) return;
    const value = requireTimelinePreview(
      await previewMediaExport(
        { job_id: id, project_id: projectID },
        { signal: abort.current.signal },
      ),
      job,
    );
    if (abort.current.signal.aborted) return;
    if (
      review.current &&
      (review.current.input.revision !== job.revision ||
        review.current.input.sha256 !== job.sha256)
    )
      review.current = null;
    setPreview(value);
    setChecked(false);
    setViewed(undefined);
    if (artifactKind(job.output_kind)) {
      // eslint-disable-next-line no-restricted-syntax -- Read verified bytes from the already granted export URL; all business commands use generated APIs.
      const response = await fetch(value.url!, {
        credentials: "omit",
        signal: abort.current.signal,
      });
      if (!response.ok) throw new Error("工程预览下载失败");
      if (!response.body) throw new Error("工程预览内容为空");
      const reader = response.body.getReader(),
        chunks: BlobPart[] = [];
      let size = 0;
      try {
        for (;;) {
          const { value: chunk, done } = await reader.read();
          if (done) break;
          size += chunk.byteLength;
          if (size > value.artifact!.byte_size!) {
            await reader.cancel();
            throw new Error("工程预览大小不匹配");
          }
          chunks.push(new Uint8Array(chunk).buffer);
        }
      } finally {
        reader.releaseLock();
      }
      const blob = new Blob(chunks, {
        type: response.headers.get("Content-Type")?.split(";")[0] ?? "",
      });
      await verifyArtifactDownload(blob, value.artifact!);
      if (abort.current.signal.aborted) return;
      saveAs(blob, value.artifact!.file_name);
      setViewed(version);
    }
  }
  async function confirm() {
    if (
      !writable ||
      !job ||
      !checked ||
      viewed !== version ||
      job.status !== MediaExportStatus.ReviewRequired
    )
      return;
    review.current ??= {
      key: crypto.randomUUID(),
      input: {
        project_id: projectID,
        revision: job.revision,
        sha256: job.sha256!,
        local_review_confirmed: true,
      },
    };
    const intent = review.current;
    const updated = requireTimelineExport(
      await reviewMediaExport({ job_id: id }, intent.input, {
        signal: abort.current.signal,
        headers: { "Idempotency-Key": intent.key },
      }),
      projectID,
      canvasID,
      nodeID,
      job,
    );
    if (abort.current.signal.aborted) return;
    await query.mutate(updated, { revalidate: false });
    review.current = null;
    onRefresh();
  }
  async function download() {
    if (!job || job.status !== MediaExportStatus.Succeeded) return;
    const value = requireTimelinePreview(
      await previewMediaExport(
        { job_id: id, project_id: projectID },
        { signal: abort.current.signal },
      ),
      job,
    );
    const blob = await downloadMediaExport(
      { job_id: id, project_id: projectID },
      { signal: abort.current.signal, responseType: "blob" },
    );
    if (artifactKind(job.output_kind))
      await verifyArtifactDownload(blob, value.artifact!);
    else await verifyTimelineDownload(blob, job.sha256!);
    if (!abort.current.signal.aborted)
      saveAs(blob, value.artifact?.file_name ?? `shot-${id}.mp4`);
  }
  const title = artifactKind(job?.output_kind) ? "剪辑工程" : "成片";
  return (
    <div className="space-y-2">
      <p className="text-sm">
        {title} ·{" "}
        {job?.status === MediaExportStatus.Succeeded
          ? "已确认"
          : job?.status === MediaExportStatus.ReviewRequired
            ? "待人工检查"
            : job?.status === MediaExportStatus.Failed ||
                job?.status === MediaExportStatus.Cancelled
              ? "制作未完成，请核对原任务"
              : "制作中"}
      </p>
      {currentPreview && !artifactKind(job?.output_kind) && (
        <video
          key={version}
          aria-label="成片预览"
          controls
          preload="metadata"
          src={currentPreview.url}
          className="max-h-80 max-w-full rounded-lg"
          onLoadedData={() => setViewed(version)}
          onError={() => {
            setViewed(undefined);
            setError("成片预览不可用，请重新打开预览");
          }}
        />
      )}
      <div className="flex flex-wrap items-center gap-2">
        {job?.status === MediaExportStatus.ReviewRequired && (
          <>
            <Button
              variant="outline"
              size="sm"
              disabled={busy}
              onClick={() => void perform(inspect)}
            >
              {artifactKind(job.output_kind) ? "下载工程检查" : "预览成片"}
            </Button>
            <Label className="flex items-center gap-2 text-sm">
              <Checkbox
                aria-label={`已检查${title}`}
                checked={checked && viewed === version}
                disabled={!writable || busy || viewed !== version}
                onCheckedChange={(value) => setChecked(value === true)}
              />
              已检查{title}
            </Label>
            <Button
              size="sm"
              disabled={!writable || busy || !checked || viewed !== version}
              onClick={() => void perform(confirm)}
            >
              确认{title}
            </Button>
          </>
        )}
        {job?.status === MediaExportStatus.Succeeded && (
          <Button
            variant="outline"
            size="sm"
            disabled={busy}
            onClick={() => void perform(download)}
          >
            下载{title}
          </Button>
        )}
        {(query.error ||
          error ||
          job?.status === MediaExportStatus.Failed ||
          job?.status === MediaExportStatus.Cancelled) && (
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            onClick={() => {
              setError("");
              void query.mutate();
              onRefresh();
            }}
          >
            核对原交付
          </Button>
        )}
      </div>
      {(query.error || error) && (
        <p role="alert" className="text-sm">
          {error || readErrorMessage(query.error)}
        </p>
      )}
    </div>
  );
}
