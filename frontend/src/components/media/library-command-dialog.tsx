"use client";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogHeader,
  DialogFooter,
} from "@/components/ui/dialog";
import { FolderForm, MetadataForm } from "./library-forms";
import { LibrarySelect } from "./library-select";
import {
  folderAncestry,
  libraryMetadataSchema,
  type LibraryCommand,
  type LibraryDetail,
  type LibraryFolder,
  type LibraryMetadata,
  type LibraryScope,
} from "./library-model";
import type { LibraryWriter } from "./use-library-writer";

export type LibraryEditFrame = {
  action: LibraryCommand["action"];
  revision: number;
  folder?: LibraryFolder;
  detail?: LibraryDetail;
  items?: { id: string; revision: number }[];
  metadata?: LibraryMetadata;
  folderDraft?: Extract<LibraryCommand, { action: "create_folder" }>["folder"];
  itemId?: string;
  itemRevision?: number;
  text?: boolean;
};
const titles = {
  create_folder: "新建目录",
  update_folder: "编辑目录",
  delete_folder: "删除目录",
  create_text: "新建文本素材",
  update_item: "编辑素材描述",
  move_items: "批量移动素材",
  recycle_items: "移入回收站",
  restore_items: "恢复素材",
  remove_items: "移出项目素材库",
};
export function LibraryCommandDialog({
  scope,
  folders,
  frame: initialFrame,
  writer,
  onClose,
  onCloseAutoFocus,
}: {
  scope: LibraryScope;
  folders: LibraryFolder[];
  frame?: LibraryEditFrame;
  writer: LibraryWriter;
  onClose: () => void;
  onCloseAutoFocus?: (event: Event) => void;
}) {
  const original = writer.intent?.body;
  const [restoredFrame, setRestoredFrame] = useState<LibraryEditFrame>();
  const frame =
    initialFrame ?? restoredFrame ?? recoverFrame(original, folders);
  const action = frame?.action ?? original?.action;
  const [revision, setRevision] = useState(
    frame?.revision ?? original?.expected_revision ?? 0,
  );
  const [folderRevision, setFolderRevision] = useState(
    frame?.folder?.revision ?? 0,
  );
  const [itemRevision, setItemRevision] = useState(
    frame?.itemRevision ?? frame?.detail?.revision ?? 0,
  );
  const [items, setItems] = useState(
    frame?.items ?? (original && "items" in original ? original.items : []),
  );
  const [dirty, setDirty] = useState(false),
    [discard, setDiscard] = useState(false),
    [target, setTarget] = useState(
      original?.action === "move_items"
        ? (original.target_folder_id ?? "root")
        : "root",
    ),
    [localError, setLocalError] = useState<string>();
  const recovery = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!writer.intent && !writer.storageError) return;
    const focused = document.activeElement;
    if (focused instanceof HTMLElement && focused.matches(":disabled"))
      recovery.current
        ?.querySelector<HTMLButtonElement>("button:not(:disabled)")
        ?.focus();
  }, [writer.intent, writer.storageError, writer.busy]);
  function requestClose() {
    if (writer.locked) return;
    if (dirty) setDiscard(true);
    else onClose();
  }
  const metadata =
    frame?.metadata ??
    (frame?.detail
      ? {
          folder_id: frame.detail.folder_id,
          title: frame.detail.title,
          category: frame.detail.category,
          tags: frame.detail.tags,
          source_label: frame.detail.source_label,
          note: frame.detail.note,
          favorite: frame.detail.favorite,
          ...(frame.detail.kind === "text"
            ? { plain_text: frame.detail.plain_text ?? "" }
            : {}),
        }
      : original && "metadata" in original
        ? original.metadata
        : undefined);
  const submit = (body: LibraryCommand) => void writer.submit(body);
  async function adoptLatest() {
    const latest = writer.latest;
    if (!latest || !writer.rejected) return;
    let entityFolder = frame?.folder;
    if (action === "update_folder" || action === "delete_folder") {
      const id =
        frame?.folder?.id ??
        (original && "folder_id" in original ? original.folder_id : undefined);
      entityFolder = latest.folders.find((folder) => folder.id === id);
      if (!entityFolder) {
        setLocalError("原目录已不可读取，请保留草稿并核对当前目录。");
        return;
      }
    }
    const detailId =
      frame?.itemId ??
      frame?.detail?.id ??
      (original?.action === "update_item" ? original.item_id : undefined);
    const detail = detailId
      ? writer.latestItems.find((item) => item.id === detailId)
      : undefined;
    if (detailId && !detail) {
      setLocalError("原素材已不可读取，请核对当前事实。");
      return;
    }
    if (items.length && writer.latestItems.length !== items.length) {
      setLocalError("批次成员不完整，不能替换原批次。");
      return;
    }
    if (!(await writer.discardRejected())) return;
    setRestoredFrame(frame);
    setRevision(latest.revision);
    if (entityFolder) setFolderRevision(entityFolder.revision);
    if (detail) setItemRevision(detail.revision);
    if (items.length)
      setItems(
        writer.latestItems.map((item) => ({
          id: item.id,
          revision: item.revision,
        })),
      );
    setLocalError(undefined);
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) requestClose();
      }}
    >
      <DialogContent
        data-library-dialog
        onCloseAutoFocus={onCloseAutoFocus}
        showCloseButton={false}
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl"
        onEscapeKeyDown={(event) => {
          if (writer.locked || dirty) {
            event.preventDefault();
            if (!writer.locked) setDiscard(true);
          }
        }}
        onInteractOutside={(event) => {
          event.preventDefault();
          requestClose();
        }}
      >
        <DialogHeader>
          <DialogTitle>
            {action ? titles[action] : "素材库修改恢复"}
          </DialogTitle>
          <DialogDescription>
            素材库版本 {revision}。修改按原范围与版本一次提交。
          </DialogDescription>
        </DialogHeader>
        {action &&
          (action === "create_folder" || action === "update_folder") &&
          frame && (
            <FolderForm
              scope={scope}
              folders={folders}
              current={frame.folder}
              draft={frame.folderDraft}
              locked={writer.locked}
              onDirty={() => setDirty(true)}
              onSubmit={(folder) =>
                submit(
                  action === "create_folder"
                    ? { scope, action, expected_revision: revision, folder }
                    : {
                        scope,
                        action,
                        expected_revision: revision,
                        folder_id: frame.folder!.id,
                        expected_folder_revision: folderRevision,
                        folder,
                      },
                )
              }
            />
          )}
        {(action === "create_text" || action === "update_item") && frame && (
          <MetadataForm
            scope={scope}
            folders={folders}
            metadata={metadata}
            text={
              action === "create_text" ||
              frame.text === true ||
              frame.detail?.kind === "text"
            }
            locked={writer.locked}
            onDirty={() => setDirty(true)}
            onSubmit={(draft) =>
              submit(
                action === "create_text"
                  ? {
                      scope,
                      action,
                      expected_revision: revision,
                      metadata: libraryMetadataSchema
                        .extend({
                          plain_text:
                            libraryMetadataSchema.shape.plain_text.unwrap(),
                        })
                        .parse(draft),
                    }
                  : {
                      scope,
                      action,
                      expected_revision: revision,
                      item_id: frame.itemId ?? frame.detail!.id,
                      expected_item_revision: itemRevision,
                      metadata: draft,
                    },
              )
            }
          />
        )}
        {action === "delete_folder" && frame?.folder && (
          <div className="flex flex-col gap-3">
            <p>
              删除目录「{frame.folder.name}」。
              {scope.kind === "personal"
                ? "目录内素材保留并归入未分类。"
                : "项目目录必须没有素材和子目录，包括回收条目。"}
            </p>
            <Button
              variant="destructive"
              disabled={writer.locked}
              onClick={() =>
                submit({
                  scope,
                  action,
                  expected_revision: revision,
                  folder_id: frame.folder!.id,
                  expected_folder_revision: folderRevision,
                })
              }
            >
              确认删除目录
            </Button>
          </div>
        )}
        {action &&
          (action === "move_items" ||
            action === "recycle_items" ||
            action === "restore_items" ||
            action === "remove_items") &&
          frame && (
            <div className="flex flex-col gap-3">
              <p>
                {action === "remove_items"
                  ? "原媒体和已有创作引用会保留；此操作移除项目素材库的目录关系。"
                  : action === "recycle_items"
                    ? "本批全部条目将进入回收站，媒体原件保留。"
                    : action === "restore_items"
                      ? "恢复库条目，不改变媒体审核或处理事实。"
                      : "本批全部成员将一次移动到目标目录。"}
              </p>
              <ul
                className="max-h-40 overflow-y-auto text-xs break-all"
                aria-label="冻结批次成员"
              >
                {items.map((item) => (
                  <li key={item.id}>
                    {item.id}
                    <span> · 版本 {item.revision}</span>
                  </li>
                ))}
              </ul>
              {action === "move_items" && (
                <LibrarySelect
                  label="目标目录"
                  value={target}
                  disabled={writer.locked}
                  options={[
                    { value: "root", label: "未分类" },
                    ...folders.map((folder) => ({
                      value: folder.id,
                      label: folderAncestry(folders, folder.id)
                        .map((ancestor) => ancestor.name)
                        .join(" / "),
                    })),
                  ]}
                  onChange={(value) => {
                    setTarget(value);
                    setDirty(true);
                  }}
                />
              )}
              <Button
                variant={
                  action === "remove_items" || action === "recycle_items"
                    ? "destructive"
                    : "default"
                }
                disabled={writer.locked || items.length === 0}
                onClick={() =>
                  submit(
                    action === "move_items"
                      ? {
                          scope,
                          action,
                          expected_revision: revision,
                          items,
                          target_folder_id: target === "root" ? null : target,
                        }
                      : { scope, action, expected_revision: revision, items },
                  )
                }
              >
                {action === "move_items"
                  ? "确认移动全部素材"
                  : action === "recycle_items"
                    ? "确认移入回收站"
                    : action === "restore_items"
                      ? "确认恢复全部素材"
                      : "确认移出项目素材库"}
              </Button>
            </div>
          )}
        {writer.error && (
          <Alert variant="destructive">
            <AlertTitle>修改尚未确认</AlertTitle>
            <AlertDescription>{writer.error}</AlertDescription>
          </Alert>
        )}
        <div ref={recovery} className="flex flex-col gap-3">
          {writer.storageError && (
            <Alert>
              <AlertTitle>原意图存储需要恢复</AlertTitle>
              <AlertDescription>
                <p>{writer.storageError}</p>
                <Button
                  variant="outline"
                  disabled={writer.busy}
                  onClick={() => void writer.restoreStorage()}
                >
                  恢复素材库原意图存储
                </Button>
              </AlertDescription>
            </Alert>
          )}
          {writer.intent && (
            <Alert>
              <AlertTitle>
                {writer.rejected
                  ? "服务端确定拒绝原修改"
                  : "原修改的结果尚未确认"}
              </AlertTitle>
              <AlertDescription>
                <p>
                  原键 {writer.intent.key} · 原版本{" "}
                  {writer.intent.body.expected_revision}
                </p>
                <details>
                  <summary>查看原完整正文</summary>
                  <pre className="max-h-64 overflow-auto text-xs break-all whitespace-pre-wrap">
                    {JSON.stringify(writer.intent.body, null, 2)}
                  </pre>
                </details>
                {writer.rejected ? (
                  <>
                    <Button
                      variant="outline"
                      disabled={writer.busy}
                      onClick={() => void writer.readLatest()}
                    >
                      读取最新素材库事实
                    </Button>
                    {writer.latest && (
                      <>
                        <p>
                          当前素材库版本 {writer.latest.revision}
                          ，原键与草稿仍保留。
                        </p>
                        <Button
                          variant="outline"
                          disabled={writer.busy}
                          onClick={() => void adoptLatest()}
                        >
                          保留草稿，采用最新素材库版本
                        </Button>
                        <Button
                          variant="ghost"
                          disabled={writer.busy}
                          onClick={async () => {
                            if (await writer.discardRejected()) onClose();
                          }}
                        >
                          明确放弃原修改草稿
                        </Button>
                      </>
                    )}
                  </>
                ) : (
                  <Button
                    variant="outline"
                    disabled={writer.busy || Boolean(writer.storageError)}
                    onClick={() => void writer.replay()}
                  >
                    人工使用原键核验素材库修改
                  </Button>
                )}
              </AlertDescription>
            </Alert>
          )}
        </div>
        {localError && <p role="alert">{localError}</p>}
        {discard && !writer.locked && (
          <Alert>
            <AlertTitle>保留当前草稿</AlertTitle>
            <AlertDescription>
              <p>关闭会放弃尚未提交的表单草稿。</p>
              <Button variant="outline" onClick={() => setDiscard(false)}>
                继续编辑草稿
              </Button>
              <Button variant="ghost" onClick={onClose}>
                明确放弃草稿并关闭
              </Button>
            </AlertDescription>
          </Alert>
        )}
        <DialogFooter>
          <Button
            variant="ghost"
            disabled={writer.locked}
            onClick={requestClose}
          >
            关闭编辑窗口
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function recoverFrame(
  body: LibraryCommand | undefined,
  folders: LibraryFolder[],
): LibraryEditFrame | undefined {
  if (!body) return undefined;
  const base = { action: body.action, revision: body.expected_revision };
  if (body.action === "create_folder")
    return { ...base, folderDraft: body.folder };
  if (body.action === "update_folder" || body.action === "delete_folder") {
    const current = folders.find((folder) => folder.id === body.folder_id);
    return {
      ...base,
      folder: current
        ? {
            ...current,
            ...(body.action === "update_folder" ? body.folder : {}),
            revision: body.expected_folder_revision,
          }
        : undefined,
      ...(body.action === "update_folder" ? { folderDraft: body.folder } : {}),
    };
  }
  if (body.action === "create_text")
    return { ...base, metadata: body.metadata, text: true };
  if (body.action === "update_item")
    return {
      ...base,
      itemId: body.item_id,
      itemRevision: body.expected_item_revision,
      metadata: body.metadata,
      text: body.metadata.plain_text !== undefined,
    };
  return { ...base, items: body.items };
}
