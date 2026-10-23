"use client";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { createProjectCopy, getProjectCopy } from "@/api/projectCopies";
import { getProject } from "@/api/projects";
import { RequestError } from "@/lib/request";
import { useProjectPresentation } from "./project-presentation";
import {
  isProjectProofCurrent,
  publishProjectInvalidation,
} from "./project-presentation-store";
import { canvasHref, requireProject, type Project } from "./project";
import {
  copyIntent,
  requireCopy,
  restoreCopyIntent,
  type CopyIntent,
  type CopyJob,
} from "./project-copy";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";

export function ProjectCopyDialog({
  project,
  onClose,
}: {
  project: Project;
  onClose: () => void;
}) {
  const { proof, namespace } = useProjectPresentation(project, false);
  const storageKey = namespace
    ? `glowloom:copy:${namespace}:${project.id}`
    : null;
  const [name, setName] = useState(
    `${Array.from(project.name).slice(0, 45).join("")} 副本`,
  );
  const [intent, setIntent] = useState<CopyIntent | null>(null);
  const [job, setJob] = useState<CopyJob | null>(null);
  const [target, setTarget] = useState<Project | null>(null);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [readAttempt, setReadAttempt] = useState(0);
  const lock = useRef(false);
  const flight = useRef<AbortController | null>(null);
  useEffect(() => {
    if (!storageKey) return;
    let active = true;
    queueMicrotask(() => {
      if (!active) return;
      try {
        const raw = localStorage.getItem(storageKey);
        if (raw) {
          const saved = restoreCopyIntent(raw, project.id);
          setIntent(saved);
          setName(saved.body.target_name ?? "");
        }
      } catch {
        setError("无法读取复制恢复记录，请检查浏览器存储。");
        return;
      }
      setLoaded(true);
    });
    return () => {
      active = false;
      flight.current?.abort();
    };
  }, [storageKey, project.id]);
  const jobID = intent?.jobID;
  useEffect(() => {
    if (!jobID || !proof) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function read() {
      try {
        if (!isProjectProofCurrent(proof!))
          throw new Error("账号或空间已变化，请关闭后重新打开");
        const result = requireCopy(
          await getProjectCopy({ id: jobID! }, { signal: controller.signal }),
          project.id,
          jobID,
        );
        if (controller.signal.aborted || !isProjectProofCurrent(proof!)) return;
        setJob(result);
        setError("");
        if (result.status === "succeeded") {
          const copied = requireProject(
            await getProject(
              { pid: result.target_project_id },
              { signal: controller.signal },
            ),
            true,
          );
          if (controller.signal.aborted || !isProjectProofCurrent(proof!))
            return;
          if (
            copied.id !== result.target_project_id ||
            copied.status !== "active" ||
            copied.is_delete
          )
            throw new Error("副本暂时无法打开");
          setTarget(copied);
          publishProjectInvalidation(copied.id);
        } else if (
          ["queued", "running", "cancel_requested"].includes(result.status)
        )
          timer = setTimeout(() => void read(), 2500);
      } catch (reason) {
        if (!controller.signal.aborted)
          setError(
            reason instanceof Error ? reason.message : "复制状态读取失败",
          );
      }
    }
    void read();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [jobID, project.id, proof, readAttempt]);
  async function submit() {
    if (
      lock.current ||
      !loaded ||
      !proof ||
      !storageKey ||
      !isProjectProofCurrent(proof)
    )
      return;
    lock.current = true;
    setPending(true);
    setError("");
    const controller = new AbortController();
    flight.current = controller;
    try {
      const original = intent ?? copyIntent(project.id, project.revision, name);
      // Persist before sending so an ambiguous response never becomes a second copy.
      localStorage.setItem(storageKey, JSON.stringify(original));
      setIntent(original);
      const result = requireCopy(
        await createProjectCopy({ pid: project.id }, original.body, {
          signal: controller.signal,
          headers: { "Idempotency-Key": original.key },
        }),
        project.id,
        original.jobID,
      );
      if (controller.signal.aborted || !isProjectProofCurrent(proof)) return;
      if (
        result.source_revision !== original.body.expected_revision ||
        result.target_name !== original.body.target_name
      )
        throw new Error("复制回执与原操作不一致，请重新确认原操作");
      const saved = { ...original, jobID: result.id };
      setIntent(saved);
      setJob(result);
      localStorage.setItem(storageKey, JSON.stringify(saved));
    } catch (reason) {
      if (!controller.signal.aborted) {
        const rejected =
          reason instanceof RequestError &&
          reason.status >= 400 &&
          reason.status < 500 &&
          reason.status !== 408;
        if (rejected) {
          try {
            localStorage.removeItem(storageKey);
            setIntent(null);
          } catch {
            setError("无法更新复制恢复记录，请检查浏览器存储。");
            return;
          }
        }
        setError(
          reason instanceof RequestError
            ? reason.status >= 500
              ? "复制服务暂不可用，原操作已保留。"
              : reason.status === 409
                ? "项目已更新，请关闭后重新读取。"
                : reason.status === 404
                  ? "项目不存在或当前账号无权复制。"
                  : "复制未完成，请检查输入后重试。"
            : reason instanceof Error
              ? reason.message
              : "复制结果未确认，请重新确认原操作",
        );
      }
    } finally {
      lock.current = false;
      if (!controller.signal.aborted) setPending(false);
    }
  }
  const labels = {
    queued: "正在等待复制",
    running: "正在复制",
    failed: "复制失败",
    cancel_requested: "正在取消",
    cancelled: "已取消",
    succeeded: "复制完成",
  };
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
    >
      <DialogContent>
        <DialogTitle>复制画布</DialogTitle>
        <DialogDescription>
          复制画布、项目素材与主体，保留原项目。
        </DialogDescription>
        {!job && (
          <div className="grid gap-2">
            <Label htmlFor="copy-name">副本名称</Label>
            <Input
              id="copy-name"
              value={name}
              disabled={pending || !!intent}
              onChange={(event) => setName(event.target.value)}
            />
          </div>
        )}
        {job && (
          <p role="status">
            {labels[job.status]}
            {job.failure_code ? ` · ${job.failure_code}` : ""}
          </p>
        )}
        {error && (
          <p role="alert" className="text-destructive">
            {error}
          </p>
        )}
        <DialogFooter>
          <Button variant="ghost" disabled={pending} onClick={onClose}>
            关闭
          </Button>
          {!jobID && (
            <Button
              disabled={
                !loaded ||
                pending ||
                !name.trim() ||
                Array.from(name.trim()).length > 50
              }
              onClick={() => void submit()}
            >
              {pending ? "正在提交…" : intent ? "重新确认原操作" : "复制画布"}
            </Button>
          )}
          {jobID && !target && (
            <Button
              variant="secondary"
              onClick={() => setReadAttempt((n) => n + 1)}
            >
              刷新状态
            </Button>
          )}
          {target && (
            <Button asChild>
              <Link
                href={canvasHref(target)}
                onClick={() => {
                  if (storageKey) localStorage.removeItem(storageKey);
                  onClose();
                }}
              >
                打开副本
              </Link>
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
