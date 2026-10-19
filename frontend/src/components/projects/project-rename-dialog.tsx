"use client";

import { useEffect, useId, useRef, useState } from "react";
import { updateProject } from "@/api/projects";
import { ensureSessionFresh } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { RequestError } from "@/lib/request";
import { normalizeProjectName, requireProject, type Project } from "./project";
import { useProjectPresentation } from "./project-presentation";
import {
  isProjectProofCurrent,
  projectPresentationStore,
  publishProjectInvalidation,
  type RenameAttempt,
} from "./project-presentation-store";

export function ProjectRenameDialog({
  project,
  open,
  onOpenChange,
}: {
  project: Project;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return open ? (
    <RenameBody project={project} onOpenChange={onOpenChange} />
  ) : null;
}

function RenameBody({
  project: initial,
  onOpenChange,
}: {
  project: Project;
  onOpenChange: (open: boolean) => void;
}) {
  const {
      project,
      refresh,
      error: readError,
      proof,
      namespace,
    } = useProjectPresentation(initial, false),
    space = useSpace();
  const [name, setName] = useState(initial.name),
    [loading, setLoading] = useState(true),
    [pending, setPending] = useState(false),
    [error, setError] = useState("");
  const [attempt, setAttempt] = useState<RenameAttempt | undefined>(() =>
    namespace
      ? projectPresentationStore.rename(namespace, initial.id)
      : undefined,
  );
  const input = useRef<HTMLInputElement>(null),
    alive = useRef(false),
    composing = useRef(false);
  const fieldId = useId();
  useEffect(() => {
    alive.current = true;
    let active = true;
    refresh()
      .then((current) => {
        if (active) {
          setName(attempt?.body.name ?? current.name);
          setLoading(false);
        }
      })
      .catch(() => {
        if (active) {
          setError("暂时无法读取当前项目名称，请稍后重新打开改名。");
          setLoading(false);
        }
      });
    return () => {
      active = false;
      alive.current = false;
    };
    // The dialog owns its frozen pending attempt for this opening.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refresh]);
  useEffect(() => {
    if (!loading) input.current?.focus();
  }, [loading]);
  const writable =
    project.status === "active" &&
    project.is_delete === false &&
    space.selected?.writable === true &&
    !readError;
  async function submit() {
    if (pending || loading || !proof || !namespace || (!writable && !attempt))
      return;
    setError("");
    const previousAttempt = projectPresentationStore.rename(
      namespace,
      project.id,
    );
    let operation = projectPresentationStore.rename(namespace, project.id);
    try {
      const normalized = operation?.body.name ?? normalizeProjectName(name);
      setPending(true);
      const current = await refresh();
      if (
        !isProjectProofCurrent(proof) ||
        (operation?.phase !== "confirmed" &&
          (current.status !== "active" || current.is_delete !== false))
      )
        throw new RequestError("forbidden", 403);
      if (operation?.phase !== "confirmed") {
        const identity = await ensureSessionFresh();
        if (
          identity.actor.id !== proof.actorId ||
          !isProjectProofCurrent(proof)
        )
          throw new RequestError("session_invalid", 401);
        if (!operation && normalized === current.name) {
          onOpenChange(false);
          return;
        }
        operation = operation ?? {
          key: crypto.randomUUID(),
          body: { name: normalized, expected_revision: current.revision },
          phase: "submitting",
        };
        projectPresentationStore.setRename(namespace, project.id, operation);
        setAttempt(operation);
        const raw = await updateProject({ pid: project.id }, operation.body, {
          headers: { "Idempotency-Key": operation.key },
          sessionRecovery: "never",
        });
        if (!isProjectProofCurrent(proof)) return;
        const receipt = requireProject(raw);
        if (receipt.id !== project.id || receipt.name !== operation.body.name)
          throw new RequestError("invalid_receipt");
        operation = { ...operation, phase: "confirmed" };
        projectPresentationStore.setRename(namespace, project.id, operation);
        if (alive.current) setAttempt(operation);
      }
      try {
        await refresh();
        if (!isProjectProofCurrent(proof)) return;
        projectPresentationStore.dropRename(
          namespace,
          project.id,
          operation.key,
        );
        publishProjectInvalidation(project.id);
        if (alive.current) onOpenChange(false);
      } catch {
        if (alive.current && isProjectProofCurrent(proof))
          setError(
            "名称已保存，暂时无法确认最新显示。请重新读取，不重复提交改名。",
          );
      }
    } catch (reason) {
      if (!alive.current || !isProjectProofCurrent(proof)) return;
      if (operation) {
        if (
          reason instanceof RequestError &&
          reason.status === 422 &&
          !previousAttempt
        ) {
          projectPresentationStore.dropRename(
            namespace,
            project.id,
            operation.key,
          );
          setAttempt(undefined);
          setError("名称未被接受，请修改后重新提交。");
          return;
        }
        if (
          reason instanceof RequestError &&
          reason.status === 409 &&
          reason.code === "revision_conflict"
        ) {
          projectPresentationStore.dropRename(
            namespace,
            project.id,
            operation.key,
          );
          setAttempt(undefined);
          await refresh().catch(() => {});
          if (alive.current)
            setError("项目已被修改。已保留输入，请核对当前名称后再次提交。");
          return;
        }
        if (operation.phase !== "confirmed") {
          operation = { ...operation, phase: "unknown" };
          projectPresentationStore.setRename(namespace, project.id, operation);
          setAttempt(operation);
        }
      }
      setError(
        reason instanceof RequestError
          ? "改名结果尚未确认。请先读取当前项目，再明确确认原改名。"
          : reason instanceof Error
            ? reason.message
            : "暂时无法修改名称。",
      );
    } finally {
      if (alive.current) setPending(false);
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!pending) onOpenChange(open);
      }}
    >
      <DialogContent>
        <DialogTitle>重命名画布</DialogTitle>
        <DialogDescription>
          项目与主画布共用一个名称。去首尾约定空白后支持1～50个Unicode码点。
        </DialogDescription>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (!composing.current) void submit();
          }}
          onKeyDown={(event) => {
            if (
              event.key === "Enter" &&
              (composing.current || event.nativeEvent.isComposing)
            )
              event.preventDefault();
          }}
        >
          <Label htmlFor={fieldId} className="text-sm font-medium">
            画布名称
          </Label>
          <Input
            id={fieldId}
            ref={input}
            value={name}
            disabled={loading || pending || !!attempt || !writable}
            onChange={(event) => setName(event.target.value)}
            onCompositionStart={() => {
              composing.current = true;
            }}
            onCompositionEnd={() => {
              composing.current = false;
            }}
            aria-describedby={`${fieldId}-status`}
          />
          <p
            id={`${fieldId}-status`}
            role="status"
            className="my-3 text-sm text-muted-foreground"
          >
            {loading
              ? "正在读取当前名称…"
              : error ||
                readError ||
                (!writable
                  ? "项目当前只读，无法改名。"
                  : attempt?.phase === "confirmed"
                    ? "原改名已保存，仅需读取最新名称。"
                    : attempt
                      ? "原改名结果未知，保留原名称、版本与幂等键。"
                      : `当前名称：${project.name}`)}
          </p>
          <DialogFooter>
            {!!readError && (
              <Button
                type="button"
                variant="outline"
                disabled={pending || loading}
                onClick={() => {
                  setLoading(true);
                  void refresh()
                    .then((current) => {
                      if (alive.current) {
                        if (!attempt) setName(current.name);
                        setError("");
                      }
                    })
                    .catch(() => {})
                    .finally(() => {
                      if (alive.current) setLoading(false);
                    });
                }}
              >
                重新读取当前名称
              </Button>
            )}
            <Button
              type="button"
              variant="outline"
              disabled={pending}
              onClick={() => onOpenChange(false)}
            >
              取消
            </Button>
            <Button
              type="submit"
              disabled={pending || loading || (!writable && !attempt)}
            >
              {pending
                ? "正在确认…"
                : attempt?.phase === "confirmed"
                  ? "重新读取最新名称"
                  : attempt
                    ? "确认原改名"
                    : "保存名称"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
