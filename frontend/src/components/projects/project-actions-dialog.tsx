"use client";
import Link from "next/link";
import { RequestError } from "@/lib/request";
import { useRef, useState } from "react";
import { ensureSessionFresh } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { useProjectPresentation } from "./project-presentation";
import {
  isProjectProofCurrent,
  projectPresentationStore,
} from "./project-presentation-store";
import {
  manageProject,
  projectActions,
  type ProjectAction,
  ProjectManagementError,
  projectManagementMessage,
} from "./project-management";
import type { Project } from "./project";
export function ProjectActionsDialog({
  projects,
  action,
  onClose,
  onResult,
}: {
  projects: Project[];
  action: ProjectAction;
  onClose: () => void;
  onResult: (success: string[], failed: string[]) => void;
}) {
  const { proof, namespace } = useProjectPresentation(projects[0], false);
  const space = useSpace();
  const [pending, setPending] = useState(false),
    [result, setResult] = useState<string | null>(null);
  const started = useRef(false);
  const running = useRef(false);
  const unknownProjects = useRef<Project[]>([]);
  const [canRetry, setCanRetry] = useState(false);
  const outcomes = useRef(
    new Map<
      string,
      { kind: "success" | "rejected" | "unknown"; message?: string }
    >(),
  );
  const label = projectActions[action].label;
  async function submit(retryOriginal = false) {
    if (
      running.current ||
      (started.current && !retryOriginal) ||
      !proof ||
      !namespace ||
      !space.selected?.writable
    )
      return;
    started.current = true;
    running.current = true;
    const targets = retryOriginal ? unknownProjects.current : projects;
    const unresolved: Project[] = [];
    setCanRetry(false);
    setPending(true);
    try {
      const identity = await ensureSessionFresh();
      const guard = () => {
        if (
          identity.actor.id !== proof.actorId ||
          !isProjectProofCurrent(proof)
        )
          throw new Error("身份或空间已变化。");
      };
      guard();
      for (const project of targets) {
        guard();
        try {
          const current = await manageProject(
            project,
            action,
            guard,
            namespace,
            retryOriginal,
          );
          projectPresentationStore.accept(namespace, current);
          outcomes.current.set(project.id, { kind: "success" });
        } catch (reason) {
          const rejected =
            (reason instanceof ProjectManagementError &&
              reason.outcome === "rejected") ||
            (reason instanceof RequestError &&
              reason.status >= 400 &&
              reason.status < 500 &&
              reason.status !== 408);
          if (!rejected) unresolved.push(project);
          outcomes.current.set(project.id, {
            kind: rejected ? "rejected" : "unknown",
            message: `${project.name}：${projectManagementMessage(reason)}`,
          });
        }
      }
      if (!isProjectProofCurrent(proof)) {
        setResult("身份或空间已变化，请关闭后重新读取。");
        return;
      }
      unknownProjects.current = unresolved;
      setCanRetry(unresolved.length > 0);
      const success = projects
        .filter((p) => outcomes.current.get(p.id)?.kind === "success")
        .map((p) => p.id);
      const failed = projects
        .filter((p) => outcomes.current.get(p.id)?.kind !== "success")
        .map((p) => p.id);
      const rejected = [...outcomes.current.values()].filter(
        (o) => o.kind === "rejected",
      ).length;
      const unknown = unresolved.length;
      const failures = [...outcomes.current.values()].flatMap((o) =>
        o.message ? [o.message] : [],
      );
      onResult(success, failed);
      setResult(
        failed.length
          ? `已完成 ${success.length} 项，${rejected} 项被拒绝，${unknown} 项结果未确认。关闭后重新读取列表核对，未完成项保留选择。${failures.join("；")}`
          : `已${label} ${success.length} 个画布。`,
      );
    } catch {
      setResult("无法确认当前登录状态，请关闭后重新读取。");
    } finally {
      running.current = false;
      setPending(false);
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
    >
      <DialogContent>
        <DialogTitle>
          {label} {projects.length} 个画布
        </DialogTitle>
        <DialogDescription>
          {action === "recycle"
            ? "删除后立即移入回收站，保留30天，截止后不可恢复。素材库中的素材保留。"
            : action === "archive"
              ? "归档后画布以只读方式保留，可随时取消归档。"
              : action === "restore"
                ? projects.every((p) => p.status === "archived")
                  ? "恢复到已归档，保留原内容并继续只读。"
                  : projects.every((p) => p.status === "active")
                    ? "恢复到画布，保留原内容和原标识。"
                    : "分别恢复到删除前的画布或已归档列表，保留原内容和原标识。"
                : "取消归档后可在画布列表继续创作。"}
        </DialogDescription>
        <ul className="max-h-48 overflow-auto text-sm text-muted-foreground">
          {projects.map((p) => (
            <li key={p.id} className="truncate py-1">
              {p.name}
            </li>
          ))}
        </ul>
        <p role="status" className="text-sm">
          {pending ? "正在处理，请稍候…" : result}
        </p>
        <DialogFooter>
          <Button variant="outline" disabled={pending} onClick={onClose}>
            {result ? "完成" : "取消"}
          </Button>
          {result && action === "restore" && (
            <>
              {projects.some((p) => p.status === "active") && (
                <Button variant="ghost" asChild>
                  <Link href="/projects?state=active" onClick={onClose}>
                    查看画布
                  </Link>
                </Button>
              )}
              {projects.some((p) => p.status === "archived") && (
                <Button variant="ghost" asChild>
                  <Link href="/projects?state=archived" onClick={onClose}>
                    查看已归档
                  </Link>
                </Button>
              )}
            </>
          )}
          {result && canRetry && (
            <Button
              disabled={pending || !proof || !space.selected?.writable}
              onClick={() => void submit(true)}
            >
              重试原操作
            </Button>
          )}
          {!result && (
            <Button
              disabled={pending || !proof || !space.selected?.writable}
              onClick={() => void submit()}
            >
              {label}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
