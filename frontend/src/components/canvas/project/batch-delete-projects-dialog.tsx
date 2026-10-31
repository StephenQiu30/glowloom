"use client";

import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { getCanvasDocument } from "@/components/canvas/runtime/api/canvas-document-bff";
import { canvasLibraryItem } from "../hooks/use-projects";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  commandFor,
  useProjectCommand,
  type ProjectCommandTarget,
} from "../hooks/use-projects";
import { CommandRecovery } from "./project-feedback";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";

export function BatchDeleteProjectsDialog({
  projects,
  onClose,
  onDeleted,
}: {
  projects: ProjectCommandTarget[];
  onClose: () => void;
  onDeleted?: (id: string) => void;
}) {
  const [items, setItems] = useState(() =>
    projects.map((project) => ({
      project,
      command: commandFor("delete", project),
    })),
  );
  const [index, setIndex] = useState(0);
  const [deleted, setDeleted] = useState<string[]>([]);
  const [skipped, setSkipped] = useState<string[]>([]);
  const [started, setStarted] = useState(false);
  const [running, setRunning] = useState(false);
  const [reading, setReading] = useState(false);
  const lock = useRef(false);
  const command = useProjectCommand();
  const complete = index >= items.length;
  const busy = running || reading || command.busy;
  const protectedOperation = running || command.unknown;
  useEffect(() => {
    if (!protectedOperation) return;
    const protect = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", protect);
    return () => window.removeEventListener("beforeunload", protect);
  }, [protectedOperation]);

  const recordDeleted = (position: number) => {
    const id = String(items[position].project.id);
    setDeleted((previous) => [...previous, id]);
    setIndex(position + 1);
    onDeleted?.(id);
  };
  const finish = (count: number, skippedCount = skipped.length) => {
    if (skippedCount)
      toast.message(`已删除 ${count} 个，已跳过 ${skippedCount} 个`);
    else toast.success(`已删除 ${count} 个画布`);
    onClose();
  };
  const runRemaining = async () => {
    if (lock.current || command.unknown || complete) return;
    lock.current = true;
    setStarted(true);
    setRunning(true);
    try {
      for (let position = index; position < items.length; position++) {
        const result = await command.run(items[position].command);
        if (!result) return;
        recordDeleted(position);
      }
      finish(deleted.length + items.length - index);
    } finally {
      lock.current = false;
      setRunning(false);
    }
  };
  const recover = async (queryOnly: boolean) => {
    if (lock.current || complete) return;
    lock.current = true;
    setRunning(true);
    try {
      const result = await command.run(undefined, queryOnly);
      if (result) {
        recordDeleted(index);
        if (index + 1 === items.length) finish(deleted.length + 1);
      }
      // Resume the remaining queue in a new user event after recovery state settles.
    } finally {
      lock.current = false;
      setRunning(false);
    }
  };
  const reread = async () => {
    if (lock.current || complete) return;
    lock.current = true;
    setReading(true);
    try {
      const latest = canvasLibraryItem(
        unwrap(await getCanvasDocument({ canvasId: items[index].project.id })),
      );
      const project = {
        id: latest.id,
        title: latest.title,
        revision: latest.revision,
        projectBindingRevision: latest.projectBindingRevision,
      };
      setItems((previous) =>
        previous.map((item, position) =>
          position === index
            ? { project, command: commandFor("delete", project) }
            : item,
        ),
      );
      command.clearError();
    } catch (error) {
      notifyError(error, "画布读取失败");
    } finally {
      lock.current = false;
      setReading(false);
    }
  };
  const skip = () => {
    if (busy || command.unknown || complete) return;
    setSkipped((previous) => [...previous, String(items[index].project.id)]);
    setIndex(index + 1);
    command.clearError();
    if (index + 1 === items.length) finish(deleted.length, skipped.length + 1);
  };
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !lock.current && !busy && !command.unknown) onClose();
      }}
    >
      <DialogContent
        className="project-action-dialog max-h-dialog overflow-y-auto"
        showCloseButton={!busy && !command.unknown}
      >
        <DialogHeader>
          <DialogTitle>批量删除画布？</DialogTitle>
          <DialogDescription>
            将删除所选的 {items.length} 个画布，删除后无法打开。
          </DialogDescription>
        </DialogHeader>
        {started && (
          <p className="text-sm tabular-nums" role="status">
            已删除 {deleted.length} 个，已跳过 {skipped.length} 个，待处理{" "}
            {items.length - index} 个
          </p>
        )}
        {command.error && !complete && (
          <p className="text-sm break-words">
            当前画布：{items[index].project.title}
          </p>
        )}
        {!command.unknown && command.error?.code === 40901 && (
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => void reread()}
          >
            读取最新版本再确认
          </Button>
        )}
        {command.unknown && (
          <CommandRecovery
            busy={busy}
            onQuery={() => void recover(true)}
            onRetry={() => void recover(false)}
          />
        )}
        <DialogFooter className="flex-wrap">
          <Button
            variant="outline"
            disabled={busy || command.unknown}
            onClick={onClose}
          >
            {complete ? "完成" : started ? "停止处理" : "取消"}
          </Button>
          {command.error && !command.unknown && !complete && (
            <Button variant="outline" disabled={busy} onClick={skip}>
              跳过此项
            </Button>
          )}
          {!complete && (
            <Button
              variant="destructive"
              disabled={
                busy || command.unknown || command.error?.code === 40901
              }
              onClick={() => void runRemaining()}
            >
              {busy
                ? "正在处理"
                : started
                  ? "继续删除剩余画布"
                  : "确认批量删除"}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
