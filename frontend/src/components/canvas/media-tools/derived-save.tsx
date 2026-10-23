"use client";
import { useEffect, useId, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { publishDerivedNodes, type DerivedReceipt } from "./publish-derived";
import type { CanvasNodeData } from "../types";

export function DerivedMediaSave({
  nodes,
  projectID,
  onSave,
  onClose,
}: {
  nodes: CanvasNodeData[];
  projectID: string;
  onSave: (nodes: CanvasNodeData[]) => boolean | Promise<boolean>;
  onClose: () => void;
}) {
  const receipts = useRef(new Map<string, DerivedReceipt>());
  const confirmationID = useId();
  const request = useRef<AbortController | null>(null);
  const [pending, setPending] = useState(false),
    [confirmed, setConfirmed] = useState(false),
    [error, setError] = useState("");
  useEffect(() => () => request.current?.abort(), []);
  const save = async () => {
    if (request.current || !confirmed) return;
    const controller = new AbortController();
    request.current = controller;
    setPending(true);
    setError("");
    try {
      const uploaded = await publishDerivedNodes(
        nodes,
        projectID,
        receipts.current,
        controller.signal,
      );
      controller.signal.throwIfAborted();
      if (!(await onSave(uploaded)))
        throw new Error("素材已保存，画布尚未确认。重试会复用已上传素材。");
      if (!controller.signal.aborted) onClose();
    } catch (error) {
      if (!controller.signal.aborted)
        setError(error instanceof Error ? error.message : "保存失败，请重试");
    } finally {
      request.current = null;
      if (!controller.signal.aborted) setPending(false);
    }
  };
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
    >
      <DialogContent
        onEscapeKeyDown={(event) => {
          if (pending) event.preventDefault();
        }}
        onPointerDownOutside={(event) => {
          if (pending) event.preventDefault();
        }}
      >
        <DialogTitle>保存处理结果</DialogTitle>
        <DialogDescription>
          将 {nodes.length}{" "}
          张处理后的图片保存到项目素材库并加入画布。已经上传的素材会保留在项目中。
        </DialogDescription>
        <Field orientation="horizontal">
          <Checkbox
            id={confirmationID}
            checked={confirmed}
            disabled={pending}
            onCheckedChange={(value) => setConfirmed(value === true)}
          />
          <FieldLabel htmlFor={confirmationID}>
            我已确认处理结果的内容与使用权限
          </FieldLabel>
        </Field>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {pending && <p role="status">正在保存素材并等待画布确认…</p>}
        <DialogFooter>
          <Button variant="outline" disabled={pending} onClick={onClose}>
            关闭
          </Button>
          <Button disabled={!confirmed || pending} onClick={() => void save()}>
            {error ? "重试保存" : "保存并加入画布"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
