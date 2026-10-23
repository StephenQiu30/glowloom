"use client";

import {
  useEffect,
  useEffectEvent,
  useImperativeHandle,
  useRef,
  useState,
  type Ref,
  type ReactNode,
} from "react";
import useSWR from "@/components/canvas/runtime/lib/use-live-query";
import * as api from "@/components/canvas/runtime/api/assetZipBffController";
import { ApiError, unwrap } from "@/components/canvas/runtime/lib/request";
import { accountScope } from "@/components/canvas/runtime/store/account-slice";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { Spinner } from "@/components/ui/spinner";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Empty,
  EmptyContent,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatBytes } from "@/components/canvas/runtime/lib/media/image-utils";
import {
  FileTransfersPanel,
  type FileTransfersHandle,
  ZipImportRecordsContext,
} from "./file-transfers-panel";
import { toast } from "sonner";
import { useTransferAbort } from "../hooks/use-transfer-abort";

import {
  assetLibraryTarget,
  useAssetLibrary,
} from "../library/asset-library-select";

const labels: Record<string, string> = {
  PREPARING: "正在预检",
  WAITING_CONFIRMATION: "等待确认",
  QUEUED: "等待导入",
  RUNNING: "正在导入",
  PARTIAL: "部分完成",
  CANCELLING: "正在取消",
  CANCELLED: "已取消",
  FAILED: "未完成",
  EXPIRED: "已过期",
  PENDING: "待导入",
  PLANNED: "待导入",
  PUBLISHING: "正在发布",
  SAVE_PENDING: "正在保存",
  SAVED: "已完成",
  SKIPPED: "已跳过",
  UNKNOWN: "结果待核对",
  READY: "已完成",
  PACKING: "正在打包",
};
type Pending =
  | { operation: "prepare"; body: CanvasContract.AssetZipImportRequest }
  | { operation: "start" | "cancel"; body: CanvasContract.AssetZipJobCommand };
type Props = {
  target?: string;
  children?: ReactNode;
  uploadRef: Ref<FileTransfersHandle>;
  disabled?: boolean;
  showRecords?: boolean;
  sourceFile?: File;
  onBusyChange?: (busy: boolean) => void;
  onImported?: (job: CanvasContract.AssetZipImportVO) => void | Promise<void>;
  onUploadCloseAutoFocus?: (event: Event) => void;
};

export function AssetZipTransfers({
  uploadRef,
  target = "GENERAL",
  children,
  disabled = false,
  showRecords = true,
  sourceFile,
  onBusyChange,
  onImported,
  onUploadCloseAutoFocus,
}: Props) {
  const { account } = useAccount();
  const scope = accountScope(account);
  const inner = useRef<FileTransfersHandle>(null);
  const abort = useTransferAbort();
  const writing = useRef(false);
  const consumed = useRef<File>(undefined);
  const [uploadOpen, setUploadOpen] = useState(false);
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [uploadBusy, setUploadBusy] = useState(false);
  const [sourceName, setSourceName] = useState("");
  const [job, setJob] = useState<CanvasContract.AssetZipImportVO>();
  const [page, setPage] = useState(1);
  const [pending, setPending] = useState<Pending>();
  const [unknown, setUnknown] = useState(false);
  const notifyJob = useRef<string>(undefined);
  const reportBusy = useEffectEvent((value: boolean) => onBusyChange?.(value));
  const completed = useEffectEvent(
    (result: CanvasContract.AssetZipImportVO) => {
      void Promise.resolve(onImported?.(result)).catch((cause) =>
        notifyError(cause, "导入素材暂不可选用"),
      );
      toast.success(`已导入 ${Number(result.saved ?? 0)} 个素材`);
    },
  );
  useEffect(() => reportBusy(busy || uploadBusy), [busy, uploadBusy]);
  useImperativeHandle(uploadRef, () => ({
    selectFile: () => inner.current?.selectFile(),
    uploadFile: async (file) => {
      await inner.current?.uploadFile(file);
    },
  }));
  useEffect(() => {
    if (!sourceFile || consumed.current === sourceFile) return;
    consumed.current = sourceFile;
    void inner.current?.uploadFile(sourceFile);
  }, [sourceFile]);

  const imports = useSWR(
    account && showRecords ? ["asset-import-jobs", scope] : null,
    (_key, options) =>
      api
        .listAssetZipImports({
          ...options,
          signal: AbortSignal.any([
            ...(options.signal instanceof AbortSignal ? [options.signal] : []),
            abort.signal,
          ]),
        })
        .then(unwrap),
    {
      shouldRetryOnError: false,

      onError: (error) => notifyError(error, "导入任务暂不可读取"),
    },
  );
  const detail = useSWR(
    account && open && job?.jobId && !unknown
      ? ["asset-import", scope, job.jobId, page]
      : null,
    (_key, options) =>
      api
        .getAssetZipImport(
          { jobId: job?.jobId, current: String(page) },
          {
            ...options,
            signal: AbortSignal.any([
              ...(options.signal instanceof AbortSignal
                ? [options.signal]
                : []),
              abort.signal,
            ]),
          },
        )
        .then(unwrap),
    {
      shouldRetryOnError: false,
      revalidateOnFocus: true,

      onError: (error) => notifyError(error, "导入进度暂不可读取"),
    },
  );
  const current = detail.data?.jobId === job?.jobId ? detail.data : job;
  const targetLibrary = useAssetLibrary(current?.projectId);
  useEffect(() => {
    if (
      !current?.jobId ||
      current.jobId !== notifyJob.current ||
      current.status !== "READY"
    )
      return;
    notifyJob.current = undefined;
    completed(current);
    void imports.mutate();
  }, [current, imports]);

  async function submit(command: Pending) {
    if (writing.current || disabled) return;
    writing.current = true;
    setBusy(true);
    setPending(command);
    setOpen(true);
    try {
      const result = unwrap(
        await (command.operation === "prepare"
          ? api.prepareAssetZipImport(command.body, { signal: abort.signal })
          : command.operation === "start"
            ? api.startAssetZipImport(command.body, { signal: abort.signal })
            : api.cancelAssetZipImport(command.body, { signal: abort.signal })),
      );
      abort.signal.throwIfAborted();
      if (!result.jobId) throw new Error("导入回执缺少任务标识");
      setJob(result);
      setPage(1);
      setUnknown(false);
      setPending(undefined);
      if (command.operation !== "cancel") notifyJob.current = result.jobId;
      await detail.mutate(result, { revalidate: false });
      void imports.mutate();
    } catch (cause) {
      if (abort.signal.aborted) return;
      const uncertain =
        !(cause instanceof ApiError) || Math.floor(cause.code / 100) >= 500;
      setUnknown(uncertain);
      if (!uncertain) setPending(undefined);
      notifyError(
        cause,
        uncertain ? "导入结果待核对，请查询原任务" : "ZIP导入未完成",
      );
    } finally {
      writing.current = false;
      if (!abort.signal.aborted) setBusy(false);
    }
  }
  async function prepare(
    file: CanvasContract.FileTransferVO,
    frozenTarget = target,
  ) {
    if (
      unknown ||
      !file.fileId ||
      !file.version ||
      file.biz !== "asset_zip" ||
      file.status !== "READY"
    )
      throw new Error("请等待素材ZIP核验完成");
    setJob(undefined);
    setSourceName(file.fileName ?? "素材ZIP");
    await submit({
      operation: "prepare",
      body: {
        commandId: crypto.randomUUID(),
        ...assetLibraryTarget(frozenTarget),
        sourceFileId: file.fileId,
        sourceVersion: file.version,
      },
    });
  }
  async function query() {
    if (writing.current) return;
    setBusy(true);
    try {
      const result = unwrap(
        await api.getAssetZipImport(
          job?.jobId
            ? { jobId: job.jobId, current: String(page) }
            : { commandId: pending?.body.commandId },
          { signal: abort.signal },
        ),
      );
      abort.signal.throwIfAborted();
      setJob(result);
      setUnknown(false);
      setPending(undefined);
      await detail.mutate(result, { revalidate: false });
      void imports.mutate();
    } catch (cause) {
      notifyError(cause, "原任务尚不可读取");
    } finally {
      if (!abort.signal.aborted) setBusy(false);
    }
  }
  function view(item: CanvasContract.AssetZipImportVO) {
    if (unknown || busy) return;
    setJob(item);
    setPage(1);
    setOpen(true);
    setSourceName(item.sourceArchiveName ?? "素材ZIP");
  }
  const total = Number(current?.total ?? 0);
  const saved = Number(current?.saved ?? 0);
  const failed = Number(current?.failed ?? 0);
  const skipped = Number(current?.skipped ?? 0);
  const pageSize = Number(current?.pageSize ?? 50);
  const pages = Math.max(1, Math.ceil(total / pageSize));
  const mayStart =
    !unknown &&
    total - saved - failed - skipped > 0 &&
    ["WAITING_CONFIRMATION", "PARTIAL"].includes(current?.status ?? "");
  return (
    <div
      className={
        showRecords || children ? "flex min-w-0 flex-col gap-6" : "contents"
      }
    >
      <FileTransfersPanel
        target={target}
        mode="zip"
        inputId="asset-zip-input"
        uploadRef={inner}
        uploadOpen={uploadOpen}
        onUploadOpenChange={setUploadOpen}
        onUploadCloseAutoFocus={onUploadCloseAutoFocus}
        onUploadStarted={() => undefined}
        onBusyChange={setUploadBusy}
        disabled={disabled || busy || unknown}
        showRecords={showRecords && !children}
        onAdopt={prepare}
      />
      {!children && !showRecords && current && (
        <Button variant="outline" onClick={() => setOpen(true)}>
          查看导入任务
        </Button>
      )}
      {!children && showRecords && (
        <>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-muted-foreground">
              导入任务在后台继续，刷新后可从记录重新打开。
            </p>
            <Button
              variant="outline"
              disabled={imports.isValidating}
              onClick={() => void imports.mutate()}
            >
              刷新任务
            </Button>
          </div>
          {imports.isLoading ? (
            <p role="status">
              <Spinner />
              正在读取导入任务…
            </p>
          ) : imports.error ? (
            <Empty>
              <EmptyHeader>
                <EmptyTitle>导入任务暂不可读取</EmptyTitle>
              </EmptyHeader>
              <EmptyContent>
                <Button variant="outline" onClick={() => void imports.mutate()}>
                  重新读取
                </Button>
              </EmptyContent>
            </Empty>
          ) : imports.data?.length ? (
            <Table aria-label="素材ZIP导入任务">
              <TableHeader>
                <TableRow>
                  <TableHead>素材ZIP</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>结果</TableHead>
                  <TableHead>操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {imports.data.map((item) => (
                  <TableRow key={item.jobId}>
                    <TableCell className="max-w-full break-all whitespace-normal">
                      {item.sourceArchiveName}
                    </TableCell>
                    <TableCell>
                      <Badge variant="secondary">
                        {labels[item.status ?? ""] ?? item.status}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      成功 {Number(item.saved ?? 0)} / {Number(item.total ?? 0)}
                      ，失败 {Number(item.failed ?? 0)}，跳过{" "}
                      {Number(item.skipped ?? 0)}
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="outline"
                        disabled={busy || unknown}
                        onClick={() => view(item)}
                      >
                        查看导入任务
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <Empty>
              <EmptyHeader>
                <EmptyTitle>还没有导入任务</EmptyTitle>
              </EmptyHeader>
            </Empty>
          )}
        </>
      )}
      <ZipImportRecordsContext
        value={{
          items: imports.data ?? [],
          isLoading: imports.isLoading,
          isValidating: imports.isValidating,
          error: imports.error,
          disabled: busy || unknown,
          refresh: () => imports.mutate(),
          prepare,
          view,
          label: (status) => labels[status ?? ""] ?? status,
        }}
      >
        {children}
      </ZipImportRecordsContext>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent
          className="dialog-assets max-h-dialog flex flex-col overflow-y-auto"
          onCloseAutoFocus={onUploadCloseAutoFocus}
        >
          <DialogHeader className="shrink-0">
            <DialogTitle>素材ZIP导入</DialogTitle>
            <DialogDescription>
              保存到：
              {current?.projectId
                ? (targetLibrary.data?.projectPath ?? "项目素材库")
                : "通用素材"}
              。
              {current?.sourceArchiveName ||
                sourceName ||
                "上传ZIP后查看原文件名与目录，再确认导入。"}
            </DialogDescription>
          </DialogHeader>
          {current ? (
            <>
              <p role="status" className="shrink-0 text-sm">
                {labels[current.status ?? ""] ?? current.status} · 共 {total}{" "}
                项，可导入 {total - skipped} 项，预计展开{" "}
                {formatBytes(Number(current.expandedBytes ?? 0))}
              </p>
              <p className="shrink-0 text-sm text-muted-foreground">
                成功 {saved} · 失败 {failed} · 跳过 {skipped} · 待处理{" "}
                {Math.max(0, total - saved - failed - skipped)}
              </p>
              {total > 0 && (
                <Progress
                  className="shrink-0"
                  aria-label="素材导入处理进度"
                  value={Math.min(
                    100,
                    ((saved + failed + skipped) / total) * 100,
                  )}
                />
              )}
              <div className="min-h-0 overflow-y-auto">
                <Table aria-label="素材导入结果">
                  <TableHeader>
                    <TableRow>
                      <TableHead>原文件与目录</TableHead>
                      <TableHead>大小</TableHead>
                      <TableHead>状态</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {current.items?.map((item) => (
                      <TableRow key={item.ordinal}>
                        <TableCell className="break-all whitespace-pre-wrap">
                          {item.relativePath || item.fileName}
                          {item.reason && (
                            <div>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => toast.info(item.reason)}
                              >
                                查看原因
                              </Button>
                              {item.status === "SKIPPED" && (
                                <p className="text-xs text-muted-foreground">
                                  {item.reason}
                                </p>
                              )}
                            </div>
                          )}
                        </TableCell>
                        <TableCell>
                          {formatBytes(Number(item.size ?? 0))}
                        </TableCell>
                        <TableCell>
                          <Badge variant="secondary">
                            {labels[item.status ?? ""] ?? item.status}
                          </Badge>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
              {pages > 1 && (
                <nav
                  className="flex shrink-0 flex-wrap items-center justify-end gap-2"
                  aria-label="导入条目分页"
                >
                  <Button
                    variant="outline"
                    disabled={busy || unknown || page <= 1}
                    onClick={() => setPage(page - 1)}
                  >
                    上一页
                  </Button>
                  <span className="text-sm">
                    第 {page} / {pages} 页
                  </span>
                  <Button
                    variant="outline"
                    disabled={busy || unknown || page >= pages}
                    onClick={() => setPage(page + 1)}
                  >
                    下一页
                  </Button>
                </nav>
              )}
            </>
          ) : busy ? (
            <p role="status" className="flex items-center gap-2">
              <Spinner />
              正在预检素材ZIP…
            </p>
          ) : (
            <Empty>
              <EmptyHeader>
                <EmptyTitle>
                  {unknown ? "预检结果待核对" : "ZIP尚未通过预检"}
                </EmptyTitle>
              </EmptyHeader>
            </Empty>
          )}
          <DialogFooter className="shrink-0 flex-wrap sm:justify-center sm:[&>button]:w-40">
            <Button variant="outline" onClick={() => setOpen(false)}>
              关闭
            </Button>
            {unknown && (
              <>
                <Button
                  variant="outline"
                  disabled={busy}
                  onClick={() => void query()}
                >
                  查询原任务
                </Button>
                {pending && (
                  <Button disabled={busy} onClick={() => void submit(pending)}>
                    重试原命令
                  </Button>
                )}
              </>
            )}
            {!unknown && detail.error && (
              <Button variant="outline" onClick={() => void query()}>
                重新读取进度
              </Button>
            )}
            {mayStart && (
              <Button
                disabled={busy || disabled || !current?.jobId}
                onClick={() =>
                  current?.jobId &&
                  void submit({
                    operation: "start",
                    body: {
                      jobId: current.jobId,
                      commandId: crypto.randomUUID(),
                    },
                  })
                }
              >
                {current?.status === "PARTIAL" ? "继续导入" : "开始导入"}
              </Button>
            )}
            {!unknown &&
              current?.jobId &&
              ["WAITING_CONFIRMATION", "QUEUED", "RUNNING", "PARTIAL"].includes(
                current.status ?? "",
              ) && (
                <Button
                  variant="outline"
                  disabled={busy}
                  onClick={() =>
                    current.jobId &&
                    void submit({
                      operation: "cancel",
                      body: {
                        jobId: current.jobId,
                        commandId: crypto.randomUUID(),
                      },
                    })
                  }
                >
                  取消导入
                </Button>
              )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
