"use client";
import { useEffect, useRef, useState } from "react";
import {
  createMediaTransfer,
  retryMediaTransfer,
  reconcileMediaTransfer,
  getMediaTransfer,
  getMediaLibraryItem,
  listMediaLibrary,
  getMediaPreview,
} from "@/api/media";
import { getProject } from "@/api/projects";
import { Button } from "@/components/ui/button";
import { useSubjectVoiceJob } from "@/components/subjects/unified-voice-job";
import { UnifiedSubjectVoiceControls } from "@/components/subjects/unified-voice-controls";
import type { SelectedSubjectMedia } from "@/components/subjects/unified-contracts";
import { getScopeGeneration, getSessionGeneration } from "@/lib/request";
import {
  projectSubjectMedia,
  type CanvasSubjectMedia,
} from "./subject-node-state";
import { requireVoiceTransfer } from "./subject-voice-transfer";

type Attempt = { key: string; input: API.TransferInput; job?: API.TransferJob };
export function SubjectNodeVoice({
  actorID,
  scopeID,
  projectID,
  disabled,
  onReady,
}: {
  actorID: string;
  scopeID: string;
  projectID: string;
  disabled: boolean;
  onReady: (name: string, media: CanvasSubjectMedia) => void;
}) {
  const [ready, setReady] = useState<{
    name: string;
    media: SelectedSubjectMedia;
  } | null>(null);
  const [transfer, setTransfer] = useState<API.TransferJob | null>(null);
  const [started, setStarted] = useState(false);
  const control = useRef<{
    key: string;
    action: "retry" | "reconcile";
    revision: number;
  } | null>(null);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const lock = useRef(false);
  const attempt = useRef<Attempt | null>(null),
    flight = useRef<AbortController | null>(null);
  const voice = useSubjectVoiceJob({
    actorID,
    scopeID,
    onReady: (name, media) => {
      attempt.current = null;
      control.current = null;
      setStarted(false);
      setTransfer(null);
      setReady({ name, media });
      setError("");
    },
  });
  useEffect(() => () => flight.current?.abort(), []);
  useEffect(() => {
    if (disabled) flight.current?.abort();
  }, [disabled]);
  async function adopt(action?: "retry" | "reconcile") {
    if (!ready || busy || disabled || lock.current) return;
    lock.current = true;
    const controller = new AbortController();
    flight.current = controller;
    const session = getSessionGeneration(),
      scope = getScopeGeneration();
    const active = () => {
      if (
        controller.signal.aborted ||
        session !== getSessionGeneration() ||
        scope !== getScopeGeneration()
      )
        throw new Error("账号、空间或画布已变化");
    };
    setBusy(true);
    setError("");
    try {
      const options = { signal: controller.signal };
      if (!attempt.current) {
        const [source, target, detail, project] = await Promise.all([
          listMediaLibrary(
            { scope: "personal", page: 1, page_size: 1 },
            options,
          ),
          listMediaLibrary(
            { scope: "project", project_id: projectID, page: 1, page_size: 1 },
            options,
          ),
          getMediaLibraryItem(
            { scope: "personal", item_id: ready.media.library_item_id! },
            options,
          ),
          getProject({ pid: projectID }, options),
        ]);
        active();
        if (
          source.current_actor_id !== actorID ||
          target.current_actor_id !== actorID ||
          source.scope?.kind !== "personal" ||
          target.scope?.project_id !== projectID ||
          detail.id !== ready.media.library_item_id ||
          detail.asset_id !== ready.media.asset_id ||
          detail.media?.revision !== ready.media.asset_revision ||
          detail.kind !== "audio" ||
          project.id !== projectID ||
          !Number.isSafeInteger(project.revision) ||
          !Number.isSafeInteger(source.revision) ||
          !Number.isSafeInteger(target.revision) ||
          !Number.isSafeInteger(detail.revision)
        )
          throw new Error("生成音频或项目版本已变化，请重新读取");
        setStarted(true);
        attempt.current = {
          key: crypto.randomUUID(),
          input: {
            source: { kind: "personal" },
            target: { kind: "project", project_id: projectID },
            items: [{ id: detail.id, revision: detail.revision }],
            expected_source_revision: source.revision,
            expected_target_revision: target.revision,
            expected_project_revision: project.revision,
          },
        };
      }
      const current = attempt.current;
      let value: unknown;
      if (action && current.job && !control.current)
        control.current = {
          key: crypto.randomUUID(),
          action,
          revision: current.job.revision!,
        };
      if (control.current && current.job) {
        const command = control.current;
        const run =
          command.action === "retry"
            ? retryMediaTransfer
            : reconcileMediaTransfer;
        value = await run(
          { job_id: current.job.id! },
          { revision: command.revision },
          { ...options, headers: { "Idempotency-Key": command.key } },
        );
      } else
        value = current.job
          ? await getMediaTransfer({ job_id: current.job.id! }, options)
          : await createMediaTransfer(current.input, {
              ...options,
              headers: { "Idempotency-Key": current.key },
            });

      for (;;) {
        active();
        const job = requireVoiceTransfer(
          value,
          actorID,
          projectID,
          ready.media.library_item_id!,
          current.job,
        );
        current.job = job;
        setTransfer(job);
        if (control.current && job.revision! > control.current.revision)
          control.current = null;
        if (job.status === "succeeded") {
          const preview = await getMediaPreview(
            { pid: projectID, asset_id: job.items![0].target_asset_id! },
            options,
          );
          active();
          if (
            !preview.asset ||
            preview.asset.id !== job.items![0].target_asset_id ||
            preview.asset.kind !== "audio"
          )
            throw new Error("项目音频尚未就绪");
          onReady(ready.name, projectSubjectMedia(preview.asset, projectID));
          setReady(null);
          attempt.current = null;
          setStarted(false);
          setTransfer(null);
          voice.reset();
          break;
        }
        if (
          !["queued", "running"].includes(job.status!) ||
          job.needs_reconciliation ||
          job.execution_unconfirmed
        )
          throw new Error("音频转移尚未完成，请恢复读取；不会重新生成音色。");
        await new Promise<void>((resolve, reject) => {
          const abort = () => {
            clearTimeout(timer);
            reject(new Error("已取消读取"));
          };
          const timer = setTimeout(() => {
            controller.signal.removeEventListener("abort", abort);
            resolve();
          }, 1200);
          controller.signal.addEventListener("abort", abort, { once: true });
        });
        active();
        value = await getMediaTransfer({ job_id: job.id! }, options);
      }
    } catch (cause) {
      if (!controller.signal.aborted)
        setError(
          cause instanceof Error
            ? cause.message
            : "音频加入项目尚未确认，请恢复原请求",
        );
    } finally {
      lock.current = false;
      if (flight.current === controller) {
        flight.current = null;
        setBusy(false);
      }
    }
  }
  return (
    <div
      className="grid gap-2"
      onMouseDown={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
    >
      <Button
        variant="outline"
        disabled={disabled || busy || voice.active || !!ready}
        onClick={() => void voice.begin()}
      >
        智能配音
      </Button>
      <UnifiedSubjectVoiceControls owner={voice} />
      {ready && (
        <>
          <p className="text-sm">
            {ready.name} 已生成。加入当前项目后可保存为此主体音色。
          </p>
          <Button
            variant="outline"
            disabled={disabled || busy}
            onClick={() => void adopt()}
          >
            {busy
              ? "正在保存项目音频…"
              : started
                ? "恢复读取原音频转移"
                : "使用生成音色"}
          </Button>
        </>
      )}
      {transfer &&
        ["failed", "partial_failed", "needs_reconciliation"].includes(
          transfer.status ?? "",
        ) && (
          <Button
            variant="outline"
            disabled={disabled || busy}
            onClick={() =>
              void adopt(
                transfer.status === "needs_reconciliation"
                  ? "reconcile"
                  : "retry",
              )
            }
          >
            {transfer.status === "needs_reconciliation"
              ? "核验音频转移"
              : "重试音频转移"}
          </Button>
        )}
      {(busy || started) && (
        <p className="text-xs text-muted-foreground">
          关闭编辑不会取消已提交的配音或音频转移任务。
        </p>
      )}
      {error && <p role="alert">{error}</p>}
    </div>
  );
}
