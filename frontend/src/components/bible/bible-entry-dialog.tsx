"use client";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { BibleFields, type BibleInput } from "./bible-fields";
import { kindLabels, type BibleCommand, type BibleKind } from "./bible-model";
import type { BibleWriter } from "./use-bible-writer";

export function BibleWriteDialog({
  title,
  writer,
  dirty,
  children,
  onClose,
  onCloseAutoFocus,
  onPrepareLatest,
}: {
  title: string;
  writer: BibleWriter;
  dirty: boolean;
  children: ReactNode;
  onClose: () => void;
  onCloseAutoFocus?: (event: Event) => void;
  onPrepareLatest?: () => Promise<void>;
}) {
  const [discard, setDiscard] = useState(false),
    recovery = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);
  useEffect(() => {
    const focused = document.activeElement;
    if (
      (writer.intent || writer.storageError) &&
      focused instanceof HTMLElement &&
      focused.matches(":disabled")
    )
      recovery.current
        ?.querySelector<HTMLButtonElement>("button:not(:disabled)")
        ?.focus();
  }, [writer.intent, writer.storageError, writer.busy]);
  function close() {
    if (writer.locked) return;
    if (dirty) setDiscard(true);
    else onClose();
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      <DialogContent
        data-bible-dialog
        showCloseButton={false}
        onCloseAutoFocus={onCloseAutoFocus}
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-3xl"
        onEscapeKeyDown={(event) => {
          if (writer.locked || dirty) {
            event.preventDefault();
            if (!writer.locked) setDiscard(true);
          }
        }}
        onInteractOutside={(event) => {
          event.preventDefault();
          close();
        }}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            按当前项目与完整冻结版本提交。修改成功后读取当前事实，旧永久回执不会替换新版本。
          </DialogDescription>
        </DialogHeader>
        {children}
        {writer.error && <p role="alert">{writer.error}</p>}
        <div ref={recovery} className="space-y-3">
          {writer.storageError && (
            <div role="alert">
              <p>{writer.storageError}</p>
              <Button
                variant="outline"
                disabled={writer.busy}
                onClick={() => void writer.restoreStorage()}
              >
                恢复设定原意图存储
              </Button>
            </div>
          )}
          {writer.intent && (
            <div className="space-y-2" role="status">
              <p>
                {writer.rejected
                  ? "服务端确定拒绝原修改。"
                  : "原修改结果尚未确认。"}{" "}
                原键 {writer.intent.key} · 原版本{" "}
                {writer.intent.command.body.expected_revision}
              </p>
              <details>
                <summary>查看原完整输入</summary>
                <pre className="max-h-64 overflow-auto text-xs break-all whitespace-pre-wrap">
                  {JSON.stringify(writer.intent.command, null, 2)}
                </pre>
              </details>
              {writer.rejected ? (
                <>
                  <Button
                    variant="outline"
                    disabled={writer.busy}
                    onClick={() => void writer.readLatest()}
                  >
                    读取最新设定事实
                  </Button>
                  {writer.latest && (
                    <div>
                      <p>
                        最新读取已完成
                        {writer.latest.detail
                          ? `：当前版本 ${writer.latest.detail.head.revision}、SHA ${writer.latest.detail.current.content_sha256}`
                          : "。"}
                      </p>
                      {onPrepareLatest && (
                        <Button
                          variant="outline"
                          disabled={writer.busy}
                          onClick={() => void onPrepareLatest()}
                        >
                          明确保留草稿并准备新的修改意图
                        </Button>
                      )}
                    </div>
                  )}
                </>
              ) : (
                <Button
                  variant="outline"
                  disabled={writer.busy || Boolean(writer.storageError)}
                  onClick={() => void writer.replay()}
                >
                  人工使用原键和原输入核验
                </Button>
              )}
            </div>
          )}
        </div>
        {discard && (
          <div role="alert" className="space-y-2">
            <p>关闭会放弃尚未提交的表单草稿。</p>
            <Button variant="outline" onClick={() => setDiscard(false)}>
              继续编辑草稿
            </Button>
            <Button variant="outline" onClick={onClose}>
              明确放弃草稿并关闭
            </Button>
          </div>
        )}
        <Button variant="ghost" disabled={writer.locked} onClick={close}>
          关闭设定编辑
        </Button>
      </DialogContent>
    </Dialog>
  );
}
export type BibleEditFrame = {
  action: "create" | "update" | "split";
  kind: BibleKind;
  id?: string;
  revision: number;
  initial?: BibleInput;
};
function recoveryFrame(command?: BibleCommand): BibleEditFrame | undefined {
  if (!command) return;
  if (command.action === "create" || command.action === "update")
    return {
      action: command.action,
      kind: command.kind,
      id: command.id,
      revision: command.body.expected_revision,
      initial: command.body[command.kind],
    };
  if (command.action === "split")
    return {
      action: "split",
      kind: "character",
      id: command.id,
      revision: command.body.expected_revision,
      initial: command.body.character,
    };
}
export function BibleEntryDialog({
  frame: initialFrame,
  writer,
  onClose,
  onCloseAutoFocus,
}: {
  frame?: BibleEditFrame;
  writer: BibleWriter;
  onClose: () => void;
  onCloseAutoFocus?: (event: Event) => void;
}) {
  const [restoredFrame, setRestoredFrame] = useState<BibleEditFrame>();
  const frame =
      initialFrame ?? restoredFrame ?? recoveryFrame(writer.intent?.command),
    [dirty, setDirty] = useState(false),
    [revision, setRevision] = useState(frame?.revision ?? 0);
  async function prepare() {
    const latest = writer.latest;
    if (
      !latest ||
      !frame ||
      (frame.id &&
        (!latest.detail ||
          latest.detail.head.id !== frame.id ||
          latest.detail.head.deleted ||
          latest.detail.head.redirect_id))
    )
      return;
    if (await writer.discardRejected()) {
      setRestoredFrame(frame);
      setRevision(latest.detail?.head.revision ?? 0);
    }
  }
  return (
    <BibleWriteDialog
      title={
        frame
          ? `${frame.action === "split" ? "拆分独立角色" : frame.action === "create" ? "新建" : "编辑"}${kindLabels[frame.kind]}`
          : "恢复原设定修改"
      }
      writer={writer}
      dirty={dirty}
      onClose={onClose}
      onCloseAutoFocus={onCloseAutoFocus}
      onPrepareLatest={prepare}
    >
      {frame && (
        <>
          <p className="text-xs break-all">
            原身份 {frame.id ?? "尚未创建"} · 冻结版本 {revision}
          </p>
          <BibleFields
            kind={frame.kind}
            initial={frame.initial}
            locked={writer.locked}
            onDirty={() => setDirty(true)}
            onSubmit={async (input) => {
              if (frame.action === "split") {
                if (!frame.id || !("definition" in input)) return;
                await writer.submit({
                  action: "split",
                  kind: "character",
                  id: frame.id,
                  body: { expected_revision: revision, character: input },
                });
              } else
                await writer.submit({
                  action: frame.action,
                  kind: frame.kind,
                  ...(frame.id ? { id: frame.id } : {}),
                  body: { expected_revision: revision, [frame.kind]: input },
                });
            }}
          />
        </>
      )}
    </BibleWriteDialog>
  );
}
