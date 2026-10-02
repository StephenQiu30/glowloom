"use client";
import { useEffect, useId, useRef, useState } from "react";
import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  documentMediaKey,
  downloadDocument,
  listDocuments,
  validateDocumentFile,
  type DocumentAsset,
} from "./document-media";
import { useDocumentUpload } from "./use-document-upload";
import type { ScriptScope } from "./source-intent";

export function DocumentLibrary({
  scope,
  disabled,
  onSelect,
  onImport,
  onBlockedChange,
}: {
  scope: ScriptScope;
  disabled: boolean;
  onSelect?: (asset: DocumentAsset) => void;
  onImport?: (assets: DocumentAsset[]) => void;
  onBlockedChange?: (blocked: boolean) => void;
}) {
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string>();
  const [downloading, setDownloading] = useState<string>();
  const [local, setLocal] = useState<File | null>(null);
  const [selected, setSelected] = useState<DocumentAsset[]>([]);
  const [reviewed, setReviewed] = useState(false);
  const id = useId();
  const recovery = useRef<HTMLButtonElement>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const upload = useDocumentUpload(scope, async (asset) => {
    await client.invalidateQueries({ queryKey: documentMediaKey(scope) });
    onSelect?.(asset);
    if (onImport)
      setSelected((previous) =>
        previous.some((item) => item.id === asset.id) || previous.length >= 200
          ? previous
          : [...previous, asset],
      );
  });
  const documents = useInfiniteQuery({
    queryKey: documentMediaKey(scope),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) => listDocuments(scope, pageParam, signal),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    staleTime: 0,
  });
  const items = documents.data?.pages.flatMap((page) => page.items) ?? [];
  const duplicates =
    new Set(items.map((asset) => asset.id)).size !== items.length;
  const pending = Boolean(upload.intent || upload.storageError);
  useEffect(() => {
    onBlockedChange?.(upload.locked);
    return () => onBlockedChange?.(false);
  }, [upload.locked, onBlockedChange]);
  useEffect(() => {
    if (!pending || upload.busy) return;
    const focused = document.activeElement;
    if (focused instanceof HTMLButtonElement && focused.matches(":disabled")) {
      if (upload.storageError || local) recovery.current?.focus();
      else fileInput.current?.focus();
    }
  }, [pending, upload.busy, upload.storageError, local]);
  async function download(asset: DocumentAsset) {
    if (downloading) return;
    setDownloading(asset.id);
    setError(undefined);
    try {
      const blob = await downloadDocument(scope, asset);
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = asset.file_name;
      document.body.append(link);
      link.click();
      link.remove();
      // 下载点击需浏览器取得 blob；取消私有临时 URL，不保存到任何持久状态。
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "原件下载未完成。");
    } finally {
      setDownloading(undefined);
    }
  }
  function close() {
    if (!upload.busy && !pending) {
      setOpen(false);
      setLocal(null);
      setReviewed(false);
    }
  }
  const show = open || pending;
  return (
    <section
      className="space-y-3 rounded-lg border p-4"
      aria-label="项目文档原件"
    >
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="font-semibold">TXT / DOCX 原件</h2>
        <Button
          type="button"
          variant="outline"
          disabled={disabled || upload.locked}
          onClick={() => {
            setOpen(true);
            setError(undefined);
          }}
        >
          上传文档原件
        </Button>
        <Button
          type="button"
          variant="ghost"
          onClick={() => void documents.refetch()}
        >
          重新读取文档
        </Button>
      </div>
      <p className="text-sm text-muted-foreground">
        原件作为项目文档保留，上传不修改章节。可下载确切原件；来源正文抽取与版本发布由文件导入任务完成。
      </p>
      {documents.isPending && <p role="status">正在读取已授权文档…</p>}
      {(documents.error || duplicates) && (
        <p role="alert">
          {duplicates
            ? "文档分页出现重复身份，请重新读取。"
            : documents.error?.message}
        </p>
      )}
      {!documents.isPending && !documents.isError && !items.length && (
        <p>项目尚无可用文档原件。</p>
      )}
      {!duplicates && (
        <ul className="max-h-80 space-y-2 overflow-y-auto">
          {items.map((asset) => (
            <li
              key={asset.id}
              className="flex min-w-0 flex-wrap items-center gap-2 rounded-lg border p-3"
            >
              <span className="min-w-0 flex-1 break-words">
                {asset.file_name} · {asset.byte_size.toLocaleString("zh-CN")}{" "}
                字节 · 修订 {asset.revision}
              </span>
              {onImport && (
                <Checkbox
                  aria-label={`导入选择 ${asset.file_name} ${asset.id}`}
                  checked={selected.some((item) => item.id === asset.id)}
                  disabled={
                    disabled ||
                    upload.locked ||
                    (!selected.some((item) => item.id === asset.id) &&
                      selected.length >= 200)
                  }
                  onCheckedChange={(checked) =>
                    setSelected((previous) =>
                      checked === true
                        ? previous.some((item) => item.id === asset.id)
                          ? previous
                          : [...previous, asset]
                        : previous.filter((item) => item.id !== asset.id),
                    )
                  }
                />
              )}
              {onSelect && (
                <Button
                  type="button"
                  variant="outline"
                  disabled={disabled || upload.locked}
                  onClick={() => onSelect(asset)}
                >
                  选择原件 {asset.file_name}
                </Button>
              )}
              <Button
                type="button"
                variant="outline"
                disabled={Boolean(downloading)}
                onClick={() => void download(asset)}
              >
                {downloading === asset.id
                  ? "正在读取原件…"
                  : `下载原件 ${asset.file_name}`}
              </Button>
            </li>
          ))}
        </ul>
      )}
      {onImport && selected.length > 0 && (
        <section className="space-y-2" aria-label="冻结前的原件顺序">
          <p className="text-sm">所选原件顺序（最多 200 个）：</p>
          <ol className="max-h-64 space-y-2 overflow-y-auto">
            {selected.map((asset, index) => (
              <li
                key={asset.id}
                className="flex min-w-0 flex-wrap items-center gap-2 rounded-lg border p-2"
              >
                <span className="min-w-0 flex-1 break-words">
                  {index + 1}. {asset.file_name}
                  <span className="block text-xs break-all">{asset.id}</span>
                </span>
                <Button
                  type="button"
                  variant="outline"
                  aria-label={`上移所选原件 ${asset.id}`}
                  disabled={disabled || upload.locked || index === 0}
                  onClick={() =>
                    setSelected((previous) => {
                      const next = [...previous];
                      [next[index - 1], next[index]] = [
                        next[index],
                        next[index - 1],
                      ];
                      return next;
                    })
                  }
                >
                  上移
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  aria-label={`下移所选原件 ${asset.id}`}
                  disabled={
                    disabled || upload.locked || index === selected.length - 1
                  }
                  onClick={() =>
                    setSelected((previous) => {
                      const next = [...previous];
                      [next[index], next[index + 1]] = [
                        next[index + 1],
                        next[index],
                      ];
                      return next;
                    })
                  }
                >
                  下移
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  aria-label={`移除所选原件 ${asset.id}`}
                  disabled={disabled || upload.locked}
                  onClick={() =>
                    setSelected((previous) =>
                      previous.filter((item) => item.id !== asset.id),
                    )
                  }
                >
                  移除
                </Button>
              </li>
            ))}
          </ol>
          <Button
            type="button"
            disabled={disabled || upload.locked || duplicates}
            onClick={() => onImport([...selected])}
          >
            导入所选 {selected.length} 个原件
          </Button>
        </section>
      )}
      {documents.hasNextPage && (
        <Button
          type="button"
          variant="outline"
          disabled={documents.isFetchingNextPage}
          onClick={() => void documents.fetchNextPage()}
        >
          {documents.isFetchingNextPage
            ? "正在读取文档下一页…"
            : "读取文档下一页"}
        </Button>
      )}
      {error && <p role="alert">{error}</p>}
      <Dialog
        open={show}
        onOpenChange={(next) => {
          if (!next) close();
        }}
      >
        <DialogContent
          showCloseButton={!upload.busy && !pending}
          className="max-h-[calc(100dvh-2rem)] overflow-y-auto"
          onEscapeKeyDown={(event) => {
            if (upload.busy || pending) event.preventDefault();
          }}
          onInteractOutside={(event) => {
            if (upload.busy || pending) event.preventDefault();
          }}
        >
          <DialogHeader>
            <DialogTitle>
              {pending ? "恢复原文上传" : "上传文档原件"}
            </DialogTitle>
            <DialogDescription>
              每个原件为非空 TXT 或 DOCX，最多 20
              MiB。请查阅完整本地原件并确认版权；未知回执保留原键与文件 SHA。
            </DialogDescription>
          </DialogHeader>
          <Label htmlFor={`${id}-file`}>
            {pending ? "重新选择确切原文件" : "选择本地原件"}
          </Label>
          <Input
            ref={fileInput}
            id={`${id}-file`}
            type="file"
            accept=".txt,.docx,text/plain,application/vnd.openxmlformats-officedocument.wordprocessingml.document"
            disabled={upload.busy}
            onChange={(event) => {
              const file = event.target.files?.[0];
              setError(undefined);
              setReviewed(false);
              try {
                if (file) validateDocumentFile(file);
                setLocal(file ?? null);
              } catch (cause) {
                setLocal(null);
                setError(
                  cause instanceof Error ? cause.message : "原文件无效。",
                );
              }
            }}
          />
          {local && (
            <p className="text-sm break-all">
              本地原件：{local.name} · {local.size.toLocaleString("zh-CN")} 字节
            </p>
          )}
          {!pending && (
            <div className="flex items-start gap-2">
              <Checkbox
                id={`${id}-review`}
                checked={reviewed}
                disabled={upload.busy || disabled}
                onCheckedChange={(value) => setReviewed(value === true)}
              />
              <Label htmlFor={`${id}-review`} className="leading-5">
                我已查阅完整本地原件，确认具有使用权限且不包含需要授权的真人素材。
              </Label>
            </div>
          )}
          {upload.intent && (
            <>
              <p className="text-sm">
                原件：{upload.intent.fileName} · {upload.intent.byteSize} 字节
              </p>
              <p className="text-xs break-all">
                SHA：{upload.intent.sha256} · 原键：{upload.intent.key}
              </p>
              <p>核验使用完整原文件与原键，刷新不会自动重传。</p>
            </>
          )}
          {upload.busy && (
            <p role="status">
              已传输 {upload.loaded.toLocaleString("zh-CN")} 字节
              {upload.total ? ` / ${upload.total.toLocaleString("zh-CN")}` : ""}
              ，正在等待服务端确切回执。
            </p>
          )}
          {(upload.error || upload.storageError || error) && (
            <p role="alert">{upload.storageError ?? upload.error ?? error}</p>
          )}
          {upload.uploaded && !pending && (
            <p role="status">
              原件已确认：{upload.uploaded.file_name}。上传未直接添加章节。
            </p>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={upload.busy || pending}
              onClick={close}
            >
              关闭原文上传
            </Button>
            {upload.busy && (
              <Button
                type="button"
                variant="outline"
                onClick={upload.stopWaiting}
              >
                停止等待并保留原上传
              </Button>
            )}
            {upload.storageError ? (
              <Button
                type="button"
                ref={recovery}
                disabled={upload.busy}
                onClick={() => void upload.restoreStorage()}
              >
                恢复文档上传存储
              </Button>
            ) : upload.intent ? (
              <Button
                type="button"
                ref={recovery}
                disabled={upload.busy || !local}
                onClick={() => void upload.replay(local)}
              >
                人工使用原键核验上传
              </Button>
            ) : (
              <Button
                type="button"
                disabled={disabled || upload.locked || !local || !reviewed}
                onClick={() => {
                  if (local) void upload.submit(local);
                }}
              >
                确认并上传原件
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}
