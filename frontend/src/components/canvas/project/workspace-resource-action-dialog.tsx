"use client";

import { useRef, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from "@/components/ui/alert-dialog";
import { Spinner } from "@/components/ui/spinner";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { useWorkspaceCommand } from "../hooks/use-workspace-command";
import {
  readWorkspaceTarget,
  workspaceCommand,
  type WorkspaceCommand,
} from "../lib/workspace-commands";
import {
  resourceKey,
  type WorkspaceResourceTarget,
} from "../lib/workspace-resources";
import { CommandRecovery } from "./project-feedback";
import { ResourceCoverField } from "./resource-cover-field";
import type { CoverReference } from "./resource-cover";

export function WorkspaceResourceActionDialog({
  operation,
  targets,
  onClose,
  onDeleted,
  onChanged,
  returnFocus,
  isCurrentActor,
}: {
  operation: "rename" | "delete";
  targets: WorkspaceResourceTarget[];
  onClose: () => void;
  onDeleted?: (key: string) => void;
  onChanged?: (command: WorkspaceCommand) => void;
  returnFocus?: HTMLElement | null;
  isCurrentActor: () => boolean;
}) {
  const [items, setItems] = useState(() =>
    targets.map((target) => ({
      target,
      command: workspaceCommand(target, operation),
    })),
  );
  const [title, setTitle] = useState(targets[0].title);
  const [cover, setCover] = useState<CoverReference>({
    coverFileId: targets[0].coverFileId ?? "",
    coverFileVersion: targets[0].coverFileVersion ?? "",
  });
  const [coverEdited, setCoverEdited] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [index, setIndex] = useState(0);
  const [deleted, setDeleted] = useState(0);
  const [skipped, setSkipped] = useState(0);
  const [started, setStarted] = useState(false);
  const [reading, setReading] = useState(false);
  const [running, setRunning] = useState(false);
  const lock = useRef(false);
  const command = useWorkspaceCommand(isCurrentActor);
  const busy = running || reading || uploading || command.busy;
  const closeBlocked = busy || command.unknown;
  const complete = index >= items.length;
  const conflict = command.error?.code === 40901;
  const deletePaused = operation === "delete" && !!command.error;
  const finish = (count: number, skippedCount = skipped) => {
    if (operation === "rename") toast.success("已保存");
    else if (skippedCount)
      toast.message(`已删除 ${count} 项，已跳过 ${skippedCount} 项`);
    else toast.success(`已删除 ${count} 项`);
    onClose();
  };
  const accepted = (position: number, confirmed = items[position].command) => {
    onChanged?.(confirmed);
    if (operation === "delete") {
      setDeleted((previous) => previous + 1);
      onDeleted?.(resourceKey(items[position].target));
    }
    setIndex(position + 1);
  };
  const submit = async () => {
    if (
      lock.current ||
      busy ||
      command.unknown ||
      complete ||
      conflict ||
      deletePaused ||
      (operation === "rename" && !title.trim())
    )
      return;
    lock.current = true;
    setRunning(true);
    setStarted(true);
    try {
      for (let position = index; position < items.length; position++) {
        const original =
          operation === "rename"
            ? workspaceCommand(
                items[position].target,
                "rename",
                title.trim(),
                coverEdited ? cover : undefined,
              )
            : items[position].command;
        if (operation === "rename") {
          setItems((previous) =>
            previous.map((item, current) =>
              current === position ? { ...item, command: original } : item,
            ),
          );
        }
        if (!(await command.run(original))) return;
        accepted(position, original);
      }
      finish(deleted + items.length - index);
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
      if (await command.run(undefined, queryOnly)) {
        accepted(index);
        if (index + 1 === items.length) finish(deleted + 1);
      }
    } finally {
      lock.current = false;
      setRunning(false);
    }
  };
  const reread = async () => {
    if (lock.current || command.unknown || complete) return;
    lock.current = true;
    setReading(true);
    try {
      const target = await readWorkspaceTarget(items[index].target);
      setItems((previous) =>
        previous.map((item, i) =>
          i === index
            ? { target, command: workspaceCommand(target, operation) }
            : item,
        ),
      );
      if (!coverEdited)
        setCover({
          coverFileId: target.coverFileId ?? "",
          coverFileVersion: target.coverFileVersion ?? "",
        });
      command.clearError();
    } catch (error) {
      notifyError(error, "内容读取失败");
    } finally {
      lock.current = false;
      setReading(false);
    }
  };
  const skip = () => {
    if (busy || command.unknown || complete) return;
    setSkipped((previous) => previous + 1);
    setIndex(index + 1);
    command.clearError();
    if (index + 1 === items.length) finish(deleted, skipped + 1);
  };
  const restoreFocus = (event: Event) => {
    if (returnFocus?.isConnected) {
      event.preventDefault();
      returnFocus.focus();
    } else {
      const tab = document.querySelector<HTMLElement>(
        '[role="tab"][aria-selected="true"]',
      );
      if (tab) {
        event.preventDefault();
        tab.focus();
      }
    }
  };
  const recovery = (
    <>
      {started &&
        operation === "delete" &&
        (command.error || skipped > 0 || index > 0) && (
          <p role="status" className="text-sm tabular-nums">
            已删除 {deleted} 项，待处理 {items.length - index} 项
            {skipped > 0 ? `，已跳过 ${skipped} 项` : ""}
          </p>
        )}
      {command.error && !complete && (
        <p className="text-sm break-words" role="status">
          {items[index].target.title}：
          {command.unknown ? "操作结果待确认" : command.error.message}
        </p>
      )}
      {conflict && !command.unknown && (
        <Button variant="outline" disabled={busy} onClick={() => void reread()}>
          读取最新内容再确认
        </Button>
      )}
      {command.unknown && (
        <CommandRecovery
          busy={busy}
          onQuery={() => void recover(true)}
          onRetry={() => void recover(false)}
        />
      )}
    </>
  );
  const buttons = (
    <>
      {operation === "delete" ? (
        <AlertDialogCancel
          disabled={closeBlocked}
          onClick={(event) => {
            event.preventDefault();
            onClose();
          }}
        >
          {started ? "停止处理" : "取消"}
        </AlertDialogCancel>
      ) : (
        <Button
          type="button"
          variant="outline"
          disabled={closeBlocked}
          onClick={onClose}
        >
          取消
        </Button>
      )}
      {operation === "delete" &&
        command.error &&
        !command.unknown &&
        items.length > 1 && (
          <Button
            type="button"
            variant="outline"
            disabled={busy}
            onClick={skip}
          >
            跳过此项
          </Button>
        )}
      {!complete &&
        (operation === "delete" ? (
          <AlertDialogAction
            variant="destructive"
            disabled={busy || command.unknown || deletePaused}
            onClick={(event) => {
              event.preventDefault();
              void submit();
            }}
          >
            {busy && <Spinner data-icon="inline-start" />}
            {busy
              ? "正在处理"
              : started
                ? "继续删除"
                : `删除 ${items.length} 项`}
          </AlertDialogAction>
        ) : (
          <Button
            type="submit"
            disabled={busy || command.unknown || conflict || !title.trim()}
          >
            {busy && <Spinner data-icon="inline-start" />}
            {busy ? "正在处理" : "保存"}
          </Button>
        ))}
    </>
  );
  if (operation === "rename")
    return (
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open && !closeBlocked && !lock.current) onClose();
        }}
      >
        <DialogContent
          className="workspace-resource-editor"
          showCloseButton={!closeBlocked}
          onCloseAutoFocus={restoreFocus}
        >
          <DialogHeader>
            <DialogTitle>
              编辑{targets[0].kind === "project" ? "项目" : "画布"}
            </DialogTitle>
            <DialogDescription className="sr-only">
              设置{targets[0].kind === "project" ? "项目" : "画布"}名称和封面
            </DialogDescription>
          </DialogHeader>
          <form
            className="flex min-h-0 flex-col gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              void submit();
            }}
          >
            <FieldGroup className="min-h-0 gap-4 overflow-y-auto">
              <Field data-disabled={busy || command.unknown}>
                <FieldLabel htmlFor="workspace-resource-name">名称</FieldLabel>
                <Input
                  id="workspace-resource-name"
                  autoFocus
                  maxLength={100}
                  required
                  value={title}
                  disabled={busy || command.unknown}
                  onChange={(event) => setTitle(event.target.value)}
                />
              </Field>
              <ResourceCoverField
                title={title}
                kind={targets[0].kind}
                cover={cover}
                disabled={busy || command.unknown || conflict}
                uploading={uploading}
                onBusyChange={setUploading}
                isCurrentActor={isCurrentActor}
                onChange={(value) => {
                  setCover(value);
                  setCoverEdited(true);
                }}
              />
              {recovery}
            </FieldGroup>
            <DialogFooter className="shrink-0">{buttons}</DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    );
  return (
    <AlertDialog
      open
      onOpenChange={(open) => {
        if (!open && !closeBlocked && !lock.current) onClose();
      }}
    >
      <AlertDialogContent onCloseAutoFocus={restoreFocus}>
        <AlertDialogHeader>
          <AlertDialogTitle>删除所选 {items.length} 项？</AlertDialogTitle>
          <AlertDialogDescription>
            仅删除空项目和可删除的画布，无法撤销。
          </AlertDialogDescription>
        </AlertDialogHeader>
        {recovery}
        <AlertDialogFooter>{buttons}</AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
