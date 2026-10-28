"use client";
import { useRef, useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { createCanvas } from "@/api/canvases";
import { ProjectPicker } from "@/components/projects/project-picker";
import { isProjectID, type Project } from "@/components/projects/project";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";
import { RequestError } from "@/lib/request";
import { readErrorMessage } from "@/lib/read-error";
export function CreateAssetCanvas({
  kind,
  onClose,
  onLock,
}: {
  kind: "canvas" | "short_drama";
  onClose: () => void;
  onLock: (locked: boolean) => void;
}) {
  const [project, setProject] = useState<Project | null>(null);
  const [name, setName] = useState(
    kind === "short_drama" ? "未命名短剧" : "未命名画布",
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [operation, setOperation] = useState<{
    project: string;
    body: Parameters<typeof createCanvas>[1];
    key: string;
  } | null>(null);
  const submitting = useRef(false);
  const flight = useRef<AbortController | null>(null);
  const router = useRouter();
  useEffect(
    () => () => {
      flight.current?.abort();
      onLock(false);
    },
    [onLock],
  );
  async function create() {
    if (submitting.current || (!operation && (!project || !name.trim())))
      return;
    const op = operation ?? {
      project: project!.id,
      body: { name: name.trim(), kind },
      key: crypto.randomUUID(),
    };
    submitting.current = true;
    setOperation(op);
    onLock(true);
    setSaving(true);
    setError("");
    const controller = new AbortController();
    flight.current = controller;
    try {
      const result = await createCanvas({ pid: op.project }, op.body, {
        headers: { "Idempotency-Key": op.key },
        signal: controller.signal,
      });
      if (
        !isProjectID(result.id) ||
        result.project_id !== op.project ||
        result.kind !== op.body.kind
      )
        throw new RequestError("invalid_receipt");
      if (controller.signal.aborted) return;
      setOperation(null);
      onLock(false);
      onClose();
      router.push(`/canvas?project_id=${op.project}&canvas_id=${result.id}`);
    } catch (reason) {
      if (controller.signal.aborted) return;
      if (
        reason instanceof RequestError &&
        reason.status >= 400 &&
        reason.status < 500
      ) {
        setOperation(null);
        onLock(false);
      }
      setError(readErrorMessage(reason));
    } finally {
      submitting.current = false;
      if (!controller.signal.aborted) setSaving(false);
    }
  }
  const frozen = saving || !!operation;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !frozen) onClose();
      }}
    >
      <DialogContent showCloseButton={!frozen}>
        <DialogTitle>
          {kind === "short_drama" ? "新建短剧画布" : "新建画布"}
        </DialogTitle>
        <DialogDescription>选择所属项目并填写名称。</DialogDescription>
        <form
          className="grid gap-5"
          onSubmit={(event) => {
            event.preventDefault();
            void create();
          }}
        >
          <ProjectPicker
            value={project}
            onChange={setProject}
            disabled={frozen}
          />
          <Field>
            <FieldLabel htmlFor="asset-canvas-name">名称</FieldLabel>
            <Input
              id="asset-canvas-name"
              name="name"
              autoComplete="off"
              value={name}
              maxLength={50}
              disabled={frozen}
              onChange={(event) => setName(event.target.value)}
            />
          </Field>
          {error && (
            <p role="alert" className="text-sm break-words text-destructive">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button
              variant="ghost"
              type="button"
              disabled={frozen}
              onClick={onClose}
            >
              取消
            </Button>
            <Button
              type="submit"
              disabled={saving || (!operation && (!project || !name.trim()))}
            >
              {saving ? "正在创建…" : operation ? "重试原操作" : "创建"}
            </Button>
          </DialogFooter>
        </form>
        {error && <FeedbackToast message={error} />}
      </DialogContent>
    </Dialog>
  );
}
