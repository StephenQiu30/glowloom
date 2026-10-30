"use client";

import { useRef } from "react";
import { X } from "lucide-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";

/** One confirmation for single and batch canvas commands; targets stay private to the caller. */
export function CanvasDeleteConfirmation({
  description,
  locked,
  pending,
  retry,
  notice,
  onClose,
  onConfirm,
  onCloseAutoFocus,
}: {
  description: string;
  locked: boolean;
  pending: boolean;
  retry: boolean;
  notice: string;
  onClose: () => void;
  onConfirm: () => void;
  onCloseAutoFocus: (event: Event) => void;
}) {
  const cancel = useRef<HTMLButtonElement>(null);
  return (
    <AlertDialog
      open
      onOpenChange={(open) => {
        if (!open && !locked) onClose();
      }}
    >
      <AlertDialogContent
        className="canvas-delete-dialog"
        overlayClassName="canvas-delete-overlay"
        aria-busy={pending}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          cancel.current?.focus();
        }}
        onCloseAutoFocus={onCloseAutoFocus}
        onEscapeKeyDown={(event) => {
          if (locked) event.preventDefault();
        }}
      >
        <AlertDialogHeader>
          <AlertDialogTitle>确认删除</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        {notice && (
          <Alert variant="destructive">
            <AlertTitle>删除尚未完成</AlertTitle>
            <AlertDescription className="break-words">
              {notice}
            </AlertDescription>
          </Alert>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel ref={cancel} disabled={locked}>
            取消
          </AlertDialogCancel>
          <AlertDialogAction
            disabled={pending}
            onClick={(event) => {
              event.preventDefault();
              onConfirm();
            }}
          >
            {pending && <Spinner data-icon="inline-start" aria-hidden="true" />}
            {pending ? "删除中…" : retry ? "重试原操作" : "删除"}
          </AlertDialogAction>
        </AlertDialogFooter>
        {!locked && (
          <Button
            className="canvas-delete-close"
            variant="ghost"
            size="icon"
            onClick={onClose}
            aria-label="关闭对话框"
          >
            <X size={20} className="size-5" aria-hidden="true" />
          </Button>
        )}
      </AlertDialogContent>
    </AlertDialog>
  );
}
