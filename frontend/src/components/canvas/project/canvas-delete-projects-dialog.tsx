"use client";
import { getCanvasDocument } from "@/components/canvas/runtime/api/canvasDocumentBffController";
import { canvasLibraryItem } from "../hooks/use-projects";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useProjectCommand } from "@/components/canvas/hooks/use-projects";
import {
  commandFor,
  type ProjectCommandTarget,
} from "@/components/canvas/hooks/use-projects";
import { CommandRecovery } from "./project-feedback";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";

export function CanvasDeleteProjectsDialog({
  project,
  onClose,
  onDeleted,
}: {
  project: ProjectCommandTarget;
  onClose: () => void;
  onDeleted?: () => void;
}) {
  const [base, setBase] = useState(project);
  const [reading, setReading] = useState(false);
  const readLock = useRef(false);
  const command = useProjectCommand();
  const locked = reading || command.busy || command.unknown;
  const confirm = async (retry = false, queryOnly = false) => {
    const result = await command.run(
      retry ? undefined : commandFor("delete", base),
      queryOnly,
    );
    if (result) {
      onClose();
      onDeleted?.();
    }
  };
  const reread = async () => {
    if (readLock.current) return;
    readLock.current = true;
    setReading(true);
    try {
      setBase(
        canvasLibraryItem(
          unwrap(await getCanvasDocument({ canvasId: project.id })),
        ),
      );
      command.clearError();
    } catch (error) {
      notifyError(error, "画布读取失败");
    } finally {
      readLock.current = false;
      setReading(false);
    }
  };
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !locked) onClose();
      }}
    >
      <DialogContent className="project-action-dialog project-delete-dialog">
        <DialogHeader>
          <DialogTitle>删除「{base.title}」？</DialogTitle>
          <DialogDescription>
            画布将从独立画布列表移除，删除后无法打开。
          </DialogDescription>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">
          该操作不会自动删除参考素材中独立保存的素材。
        </p>
        {command.error?.code === 40901 && (
          <Button
            variant="outline"
            disabled={locked}
            onClick={() => void reread()}
          >
            {reading ? "正在读取" : "读取最新版本再确认"}
          </Button>
        )}
        {command.unknown && (
          <CommandRecovery
            busy={command.busy}
            onQuery={() => void confirm(true, true)}
            onRetry={() => void confirm(true)}
          />
        )}
        <DialogFooter>
          <Button variant="outline" disabled={locked} onClick={onClose}>
            取消
          </Button>
          <Button
            variant="destructive"
            disabled={locked || command.error?.code === 40901}
            onClick={() => void confirm()}
          >
            {command.busy ? "正在删除" : "确认删除"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
