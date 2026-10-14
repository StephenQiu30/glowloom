"use client";
import { RichGeneration, type CanvasComposerOwner } from "./rich-generation";
import type { PromptDraft } from "./prompt-document";
import { useEffect, useRef, useState } from "react";
import { History, LoaderCircle, RotateCw } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { ProjectPicker } from "@/components/projects/project-picker";
import { type Project } from "@/components/projects/project";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { RequestError } from "@/lib/request";
import {
  getOperation,
  cancelOperation,
  listProjectOperations,
} from "@/api/operations";
import {
  activeTask,
  flowError,
  requireTask,
  taskLabels,
  type Task,
} from "./contracts";
export type GenerationSeed = {
  composer?: PromptDraft;
  draftID?: string;
  canvasOwner?: CanvasComposerOwner;
  prompt: string;
  capability: "image" | "video";
  source?: API.CanvasSource;
  project?: Project;
  reuse?: Task;
  draft?: {
    aspect_ratio: string;
    candidate_count: number;
    negative_prompt: string;
  };
  pickSubject?: boolean;
  subject?: import("@/components/subjects/contracts").Subject;
  reference?: { assetID: string; kind: "image" | "video" | "audio" };
};
export function GenerationDialog({
  seed,
  onClose,
  onAdopt,
}: {
  seed: GenerationSeed;
  onClose: () => void;
  onAdopt?: (assetID: string, kind: string) => Promise<void> | void;
}) {
  const [project, setProject] = useState<Project | null>(seed.project ?? null);
  const [locked, setLocked] = useState(false);
  useEffect(() => {
    if (!locked) return;
    const guard = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [locked]);
  return (
    <Dialog
      open
      onOpenChange={(v) => {
        if (!v && !locked) onClose();
      }}
    >
      <DialogContent
        showCloseButton={!locked}
        className="generation-dialog sm:max-w-4xl"
        onInteractOutside={(e) => e.preventDefault()}
      >
        <DialogTitle>
          {seed.capability === "video" ? "视频生成" : "图片生成"}
        </DialogTitle>
        <DialogDescription>
          选择模型与参考素材，确认报价后开始生成。任务保留在所选项目中。
        </DialogDescription>
        {!seed.project && (
          <ProjectPicker
            value={project}
            onChange={setProject}
            disabled={locked}
          />
        )}
        {project ? (
          <GenerationForm
            key={project.id}
            project={project}
            seed={seed}
            onAdopt={onAdopt}
            locked={locked}
            onLock={setLocked}
          />
        ) : (
          <div className="empty-state">
            <p>先选择一个项目，继续这次创作。</p>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
function GenerationForm({
  project,
  seed,
  onAdopt,
  onLock,
  locked,
}: {
  project: Project;
  seed: GenerationSeed;
  onAdopt?: (id: string, kind: string) => Promise<void> | void;
  locked: boolean;
  onLock: (v: boolean) => void;
}) {
  const [reuse, setReuse] = useState<Task | undefined>(seed.reuse);
  return (
    <>
      <RichGeneration
        key={reuse?.id ?? seed.draftID ?? "new"}
        projectID={project.id}
        seed={{ ...seed, reuse }}
        draftID={seed.draftID}
        canvas={seed.canvasOwner}
        onAdopt={onAdopt}
        onLock={onLock}
      />
      <TaskHistory
        projectID={project.id}
        capability={seed.capability + ".generate"}
        onAdopt={onAdopt}
        onReuse={setReuse}
        locked={locked}
      />
    </>
  );
}
function TaskHistory({
  projectID,
  capability,
  onAdopt,
  onReuse,
  locked,
}: {
  projectID: string;
  capability: string;
  onAdopt?: (id: string, kind: string) => Promise<void> | void;
  onReuse: (t: Task) => void;
  locked: boolean;
}) {
  const [open, setOpen] = useState(false),
    [items, setItems] = useState<API.TaskSummary[]>([]),
    [cursor, setCursor] = useState<string | undefined>(),
    [error, setError] = useState(""),
    [pending, setPending] = useState(false),
    [selected, setSelected] = useState<string | null>(null);
  async function load(next?: string) {
    if (pending) return;
    setPending(true);
    setError("");
    try {
      const r = await listProjectOperations({
        pid: projectID,
        capability,
        limit: 30,
        cursor: next,
      });
      if (!Array.isArray(r.items)) throw new RequestError("invalid_receipt");
      r.items.forEach((i) => requireTask(i, projectID));
      setItems((v) => (next ? [...v, ...r.items!] : r.items!));
      setCursor(r.next_cursor);
      setOpen(true);
    } catch (e) {
      setError(flowError(e));
    } finally {
      setPending(false);
    }
  }
  return (
    <section className="task-history">
      <Button variant="ghost" onClick={() => void load()} disabled={pending}>
        <History />
        {pending ? "正在读取…" : "查看生成历史"}
      </Button>
      {error && <p role="alert">{error}</p>}
      {open && (
        <>
          <div className="task-list">
            {items.map((t) => (
              <button key={t.id} onClick={() => setSelected(t.id!)}>
                <span>
                  {t.model_name} · {taskLabels[t.status!]}
                </span>
                <small>
                  {new Date(t.create_time!).toLocaleString("zh-CN")}
                </small>
              </button>
            ))}
          </div>
          {items.length === 0 && (
            <p>还没有生成任务。提交后的任务会保留在这里。</p>
          )}
          {cursor && (
            <Button
              variant="ghost"
              disabled={pending}
              onClick={() => void load(cursor)}
            >
              更多任务
            </Button>
          )}
        </>
      )}
      {selected && (
        <TaskDetail
          key={selected}
          id={selected}
          projectID={projectID}
          onAdopt={onAdopt}
          onReuse={
            locked
              ? undefined
              : (t) => {
                  onReuse(t);
                  setSelected(null);
                }
          }
        />
      )}
    </section>
  );
}
export function TaskDetail({
  projectID,
  id,
  onAdopt,
  onReuse,
}: {
  projectID: string;
  id: string;
  onAdopt?: (id: string, kind: string) => Promise<void> | void;
  onReuse?: (task: Task) => void;
}) {
  const [task, setTask] = useState<Task | null>(null),
    [error, setError] = useState(""),
    [retry, setRetry] = useState(0),
    [confirmCancel, setConfirmCancel] = useState(false),
    [pending, setPending] = useState(false);
  const cancelKey = useRef<string | null>(null);
  const [adopting, setAdopting] = useState(false);
  async function adopt(asset: string, kind: string) {
    if (adopting) return;
    setAdopting(true);
    setError("");
    try {
      await onAdopt?.(asset, kind);
    } catch (e) {
      setError(e instanceof Error ? e.message : "采用结果未确认，请重试。");
    } finally {
      setAdopting(false);
    }
  }
  useEffect(() => {
    const c = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function read() {
      try {
        const t = requireTask(
          await getOperation(
            { id, project_id: projectID },
            { signal: c.signal },
          ),
          projectID,
        );
        if (c.signal.aborted) return;
        if (t.id !== id) throw new RequestError("invalid_receipt");
        setTask(t);
        setError("");
        if (activeTask(t.status)) timer = setTimeout(() => void read(), 3000);
      } catch (e) {
        if (!c.signal.aborted) setError(flowError(e));
      }
    }
    void read();
    return () => {
      c.abort();
      clearTimeout(timer);
    };
  }, [id, projectID, retry]);
  async function cancel() {
    if (pending) return;
    setPending(true);
    cancelKey.current ??= crypto.randomUUID();
    try {
      const receipt = await cancelOperation(
        { id },
        { project_id: projectID },
        { headers: { "Idempotency-Key": cancelKey.current } },
      );
      if (
        receipt.accepted !== true ||
        receipt.target_id !== id ||
        receipt.target_type !== "operation" ||
        receipt.action !== "cancel"
      )
        throw new RequestError("invalid_receipt");
      setConfirmCancel(false);
      setRetry((n) => n + 1);
    } catch (e) {
      setError(flowError(e));
    } finally {
      setPending(false);
    }
  }
  return (
    <section className="task-detail" aria-live="polite">
      <div className="flex items-center gap-3">
        <h3>{task ? taskLabels[task.status] : "读取任务…"}</h3>
        {task && activeTask(task.status) && (
          <LoaderCircle className="animate-spin" size={16} />
        )}
        <Button
          variant="ghost"
          className="ml-auto"
          aria-label="刷新任务"
          onClick={() => setRetry((n) => n + 1)}
        >
          <RotateCw />
        </Button>
      </div>
      {error && (
        <p role="alert" className="flow-error">
          {error}
        </p>
      )}
      {task?.failure_code && (
        <p>
          生成未能完成，请核对输入或稍后重试。
          <small className="block text-muted-foreground">
            错误编号：{task.failure_code}
          </small>
        </p>
      )}
      {task?.status === "completed" &&
        !task.outputs?.some(
          (o) => o.moderation_status === "passed" && o.media_asset_id,
        ) && (
          <p>任务已结束，当前没有通过审核的可用结果。请调整描述后重新生成。</p>
        )}
      {task && ["unknown", "reconciling", "manual"].includes(task.status) && (
        <p>结果正在核对，请保留此任务，避免重复提交。</p>
      )}
      {task?.status === "cancelled" && (
        <p>任务已取消，你可以调整输入重新生成。</p>
      )}
      {task?.outputs && (
        <div className="generation-results">
          {task.outputs
            .filter((o) => o.media_asset_id && o.moderation_status === "passed")
            .map((o) => (
              <div key={o.id}>
                <div className="generation-result-preview">
                  <ProjectMediaPreview
                    projectID={projectID}
                    id={o.media_asset_id!}
                    kind={o.kind}
                    large
                  />
                </div>
                {onAdopt && (
                  <Button
                    disabled={adopting}
                    onClick={() => void adopt(o.media_asset_id!, o.kind!)}
                  >
                    {adopting ? "正在保存结果…" : "采用结果到画布"}
                  </Button>
                )}
              </div>
            ))}
        </div>
      )}
      {task && activeTask(task.status) && task.status !== "cancelling" && (
        <Button variant="outline" onClick={() => setConfirmCancel(true)}>
          取消任务
        </Button>
      )}
      {task &&
        ["failed", "cancelled", "expired", "completed"].includes(task.status) &&
        onReuse && (
          <Button variant="outline" onClick={() => onReuse(task)}>
            复用参数重新报价
          </Button>
        )}
      {confirmCancel && (
        <div className="quote-card">
          <h4>取消这个任务？</h4>
          <p>已开始的任务可能仍会完成，取消与费用结果以任务状态为准。</p>
          <div className="flex gap-3">
            <Button
              variant="outline"
              disabled={pending}
              onClick={() => setConfirmCancel(false)}
            >
              继续等待
            </Button>
            <Button disabled={pending} onClick={() => void cancel()}>
              {pending ? "正在请求…" : "确认取消"}
            </Button>
          </div>
        </div>
      )}
    </section>
  );
}
