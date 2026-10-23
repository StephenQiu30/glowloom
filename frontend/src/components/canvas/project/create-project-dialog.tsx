"use client";
import { useEffect, useId, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Spinner } from "@/components/ui/spinner";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useProjectCommand } from "@/components/canvas/hooks/use-projects";
import { commandFor } from "@/components/canvas/hooks/use-projects";
import { CommandRecovery } from "./project-feedback";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";

export function CreateProjectDialog({
  open,
  onOpenChange,
  returnFocus,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  returnFocus?: HTMLElement | null;
}) {
  const router = useRouter();
  const titleId = useId();
  const titleInputRef = useRef<HTMLInputElement>(null);
  const [title, setTitle] = useState("");
  const [validation, setValidation] = useState<Error | null>(null);
  const command = useProjectCommand();
  const invalidTitle = command.error?.code === 40000;
  useEffect(() => {
    if (invalidTitle) titleInputRef.current?.focus();
  }, [invalidTitle]);
  const complete = async (retry = false, queryOnly = false) => {
    setValidation(null);
    try {
      const result = await command.run(
        retry ? undefined : commandFor("create", undefined, title),
        queryOnly,
      );
      if (result) {
        onOpenChange(false);
        setTitle("");
        router.push(`/canvases/${result.id}`);
      }
    } catch (error) {
      notifyError(error, "画布操作未完成");
      titleInputRef.current?.focus();
      setValidation(
        error instanceof Error ? error : new Error("请输入有效的画布名称"),
      );
    }
  };
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!command.busy && !command.unknown) {
          command.clearError();
          setValidation(null);
          onOpenChange(next);
        }
      }}
    >
      <DialogContent
        className="project-action-dialog"
        onCloseAutoFocus={(event) => {
          if (returnFocus?.isConnected) {
            event.preventDefault();
            returnFocus.focus();
          }
        }}
      >
        <DialogHeader>
          <DialogTitle>新建画布</DialogTitle>
          <DialogDescription>
            输入画布名称，开始编辑文本、图片和视频节点。
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void complete();
          }}
          className="grid gap-4"
        >
          <FieldGroup>
            <Field
              data-invalid={Boolean(validation || invalidTitle)}
              data-disabled={command.busy || command.unknown}
            >
              <FieldLabel htmlFor={titleId}>画布名称</FieldLabel>
              <Input
                id={titleId}
                ref={titleInputRef}
                name="projectTitle"
                aria-invalid={Boolean(validation || invalidTitle)}
                autoFocus
                placeholder="为画布取一个名称"
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                disabled={command.busy || command.unknown}
              />
            </Field>
          </FieldGroup>
          {command.unknown && (
            <CommandRecovery
              busy={command.busy}
              onQuery={() => void complete(true, true)}
              onRetry={() => void complete(true)}
            />
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={command.busy || command.unknown}
              onClick={() => onOpenChange(false)}
            >
              取消
            </Button>
            <Button type="submit" disabled={command.busy || command.unknown}>
              {command.busy && <Spinner data-icon="inline-start" />}
              {command.busy ? "正在创建" : "创建并打开"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
