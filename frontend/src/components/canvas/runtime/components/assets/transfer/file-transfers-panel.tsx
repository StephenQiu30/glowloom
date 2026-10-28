"use client";
import { ContentState } from "@/components/layout/content-state";
import { uploadGrantedPart } from "@/components/canvas/runtime/lib/media/granted-transfer";
import { uploadTarget } from "../lib/asset-upload-target";
import { waitForFileTransfer } from "@/components/canvas/runtime/lib/online-projections";
import { Input } from "@/components/ui/input";

import {
  createContext,
  useContext,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
  type Ref,
} from "react";
import useSWR from "@/components/canvas/runtime/lib/use-live-query";
import { FileUp, RefreshCw } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty";
import { toast } from "sonner";
import * as api from "@/components/canvas/runtime/api/fileBffController";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { useAuthDialog } from "@/components/canvas/runtime/components/account/auth/auth-dialog-provider";
import {
  notifyError,
  useErrorNotification,
} from "@/components/canvas/runtime/components/feedback/error-notification";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Spinner } from "@/components/ui/spinner";
import { Progress } from "@/components/ui/progress";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { downloadFixedFile } from "../lib/original-download";
import { useTransferAbort } from "../hooks/use-transfer-abort";
import { formatBytes } from "@/components/canvas/runtime/lib/media/image-utils";

const statusLabels: Record<string, string> = {
  INITIALIZING: "正在初始化",
  UPLOADING: "等待上传或续传",
  VERIFYING: "正在校验",
  READY: "可下载",
  CANCELLING: "正在取消",
  CANCELLED: "已取消",
  FAILED: "校验失败",
  REJECTING: "校验失败，正在清理",
};
const fileKindLabels: Record<string, string> = {
  asset_zip: "素材ZIP",
  project_image: "图片",
  project_video: "视频",
  project_audio: "音频",
  script_document: "文本",
};

export type FileTransfersHandle = {
  selectFile: () => void;
  uploadFile: (file: File) => Promise<void>;
};
const assetBizTypes = [
  "project_image",
  "project_video",
  "project_audio",
  "script_document",
];
export type ZipImportRecords = {
  items: CanvasContract.AssetZipImportVO[];
  isLoading: boolean;
  isValidating: boolean;
  error: unknown;
  disabled: boolean;
  refresh: () => Promise<unknown>;
  prepare: (
    file: CanvasContract.FileTransferVO,
    target?: string,
  ) => Promise<void>;
  view: (job: CanvasContract.AssetZipImportVO) => void;
  label: (status?: string) => string | undefined;
};
export const ZipImportRecordsContext = createContext<
  ZipImportRecords | undefined
>(undefined);

type TransferProps = {
  target?: string;
  mode?: "normal" | "zip";
  zipImports?: ZipImportRecords;
  picker?: {
    accept: string;
    bizTypes: readonly string[];
    label: string;
    description: string;
  };
  onAdopt?: (
    file: CanvasContract.FileTransferVO,
    target?: string,
  ) => Promise<void>;
  uploadOpen: boolean;
  onUploadOpenChange: (open: boolean) => void;
  onUploadCloseAutoFocus?: (event: Event) => void;
  uploadRef: Ref<FileTransfersHandle>;
  onUploadStarted: () => void;
  onBusyChange: (busy: boolean) => void;
  disabled: boolean;
  showRecords?: boolean;
  inputId?: string;
};

export function FileTransfersPanel({
  onAdopt,
  uploadOpen,
  onUploadOpenChange,
  zipImports: suppliedZipImports,
  ...props
}: TransferProps) {
  const contextZipImports = useContext(ZipImportRecordsContext);
  const zipImports = suppliedZipImports ?? contextZipImports;
  const { account, ready } = useAccount();
  const openAuth = useAuthDialog();
  return (
    <section
      aria-label={
        props.showRecords === false
          ? undefined
          : zipImports
            ? "上传任务"
            : props.mode === "zip"
              ? "ZIP导入源文件"
              : "普通素材上传记录"
      }
      className={
        props.showRecords === false ? "contents" : "flex flex-col gap-4"
      }
    >
      {!ready ? (
        <ContentState
          className="min-h-40"
          state="loading"
          title="正在读取账户"
        />
      ) : account ? (
        <TransferContent
          key={String(account.id)}
          ownerId={String(account.id)}
          onAdopt={onAdopt}
          uploadOpen={uploadOpen}
          onUploadOpenChange={onUploadOpenChange}
          {...props}
          zipImports={zipImports}
        />
      ) : (
        <ContentState
          className="min-h-40"
          title="登录后上传文件"
          actions={<Button onClick={() => openAuth()}>登录</Button>}
        />
      )}
    </section>
  );
}

function TransferContent({
  ownerId,
  target = "GENERAL",
  mode = "normal",
  zipImports,
  picker,
  onAdopt,
  uploadOpen,
  onUploadOpenChange,
  onUploadCloseAutoFocus,
  uploadRef,
  onUploadStarted,
  onBusyChange,
  disabled,
  showRecords = true,
  inputId = "server-file-input",
}: TransferProps & { ownerId: string }) {
  const isZip = mode === "zip";
  const label = isZip ? "素材ZIP" : (picker?.label ?? "素材");
  const downloadAbort = useTransferAbort();
  const [file, setFile] = useState<File>();
  const [biz, setBiz] = useState("");
  const [commandId, setCommandId] = useState<string>();
  const [resumeId, setResumeId] = useState<string>();
  const [activeId, setActiveId] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [transferring, setTransferring] = useState(false);
  const [progress, setProgress] = useState(0);
  const [cursors, setCursors] = useState<Array<string | undefined>>([
    undefined,
  ]);
  const controller = useRef<AbortController | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const selection = useRef<CanvasContract.FileTransferVO | null>(null);
  const mounted = useRef(true);
  const files = useSWR(
    showRecords ? ["file-transfers", ownerId, cursors.at(-1)] : null,
    async (_key, options) =>
      unwrap(
        await api.listFileTransfers({ beforeId: cursors.at(-1) }, options),
      ),
    {
      shouldRetryOnError: false,
    },
  );
  const visibleFiles = (files.data ?? []).filter((item) =>
    isZip || (zipImports && item.biz === "asset_zip")
      ? item.biz === "asset_zip"
      : (picker?.bizTypes ?? assetBizTypes).includes(item.biz ?? ""),
  );
  useErrorNotification(files.error, "文件列表读取失败");
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      queueMicrotask(() => {
        if (!mounted.current) controller.current?.abort();
      });
    };
  }, []);

  async function upload({
    file,
    biz,
    commandId,
    resumeId,
  }: {
    file?: File;
    biz: string;
    commandId?: string;
    resumeId?: string;
  }) {
    if (!file || !commandId) return;
    if (controller.current) return;
    const abort = new AbortController();
    controller.current = abort;
    const options = { signal: abort.signal };
    setBusy(true);
    onBusyChange(true);
    onUploadStarted();
    setTransferring(true);
    setCursors([undefined]);
    try {
      const frozenTarget = uploadTarget(ownerId, resumeId ?? commandId, target);
      const session = unwrap(
        resumeId
          ? await api.getFileTransfer({ fileId: resumeId }, options)
          : await api.initFileTransfer(
              { commandId, biz, fileName: file.name, size: file.size },
              options,
            ),
      );
      const { fileId } = session;
      const partSize = Number(session.partSize);
      if (!fileId || !Number.isSafeInteger(partSize) || partSize <= 0)
        throw new Error("上传会话无效");
      uploadTarget(ownerId, fileId, frozenTarget);
      setActiveId(fileId);
      setResumeId(fileId);
      if (session.fileName !== file.name || Number(session.size) !== file.size)
        throw new Error("请选择该会话原始名称和大小相同的文件");
      if (session.status !== "UPLOADING") {
        setTransferring(false);
        await adoptWhenReady(session, abort.signal);
        return;
      }
      const count = Math.ceil(file.size / partSize);
      const done = new Map(
        (session.parts ?? [])
          .filter(
            (part) =>
              part.number != null &&
              part.number >= 1 &&
              part.number <= count &&
              Number(part.size) ===
                Math.min(partSize, file.size - (part.number - 1) * partSize),
          )
          .map((part) => [part.number!, Number(part.size)]),
      );
      const uploaded = new Map(done);
      const remaining = Array.from(
        { length: count },
        (_, index) => index + 1,
      ).filter((number) => !done.has(number));
      const updateProgress = () => {
        if (mounted.current)
          setProgress(
            Math.floor(
              ([...uploaded.values()].reduce((sum, size) => sum + size, 0) /
                file.size) *
                100,
            ),
          );
      };
      updateProgress();
      let next = 0;
      let firstFailure: unknown;
      const worker = async () => {
        try {
          while (next < remaining.length && !abort.signal.aborted) {
            const number = remaining[next++];
            const signed = unwrap(
              await api.signFilePart({ fileId, partNumber: number }, options),
            );
            if (!signed.url) throw new Error("分片授权无效");
            const blob = file.slice(
              (number - 1) * partSize,
              Math.min(number * partSize, file.size),
            );
            await uploadGrantedPart(signed.url, blob, abort.signal);
            uploaded.set(number, blob.size);
            updateProgress();
          }
        } catch (error) {
          if (!abort.signal.aborted) firstFailure = error;
          abort.abort();
          throw error;
        }
      };
      const results = await Promise.allSettled(
        Array.from({ length: 3 }, worker),
      );
      if (firstFailure) throw firstFailure;
      const failed = results.find((result) => result.status === "rejected");
      if (failed?.status === "rejected") throw failed.reason;
      if (!abort.signal.aborted) {
        await api.completeFileTransfer({ fileId }, options).then(unwrap);
        setTransferring(false);
        await adoptWhenReady(
          unwrap(await api.getFileTransfer({ fileId }, options)),
          abort.signal,
        );
      }
    } catch (error) {
      if (mounted.current) {
        notifyError(error, "上传未完成，可查询原会话后继续");
        if (!abort.signal.aborted) onUploadOpenChange(true);
      }
    } finally {
      if (mounted.current) {
        setBusy(false);
        setTransferring(false);
        onBusyChange(false);
        void files.mutate();
      }
      controller.current = null;
    }
  }

  async function adoptWhenReady(
    session: CanvasContract.FileTransferVO,
    signal: AbortSignal,
  ) {
    if (!session.fileId) throw new Error("上传会话无效");
    const current = await waitForFileTransfer(
      session.fileId,
      session,
      () =>
        api
          .getFileTransfer({ fileId: session.fileId! }, { signal })
          .then(unwrap),
      signal,
    );
    signal.throwIfAborted();
    if (current.status === "READY") {
      if (zipImports && current.biz === "asset_zip") {
        await zipImports.prepare(
          current,
          uploadTarget(ownerId, current.fileId!, target),
        );
      } else if (
        onAdopt &&
        (isZip
          ? current.biz === "asset_zip"
          : (picker?.bizTypes ?? assetBizTypes).includes(current.biz ?? ""))
      ) {
        await onAdopt(current, uploadTarget(ownerId, current.fileId!, target));
      } else toast.success(`${label}已上传`);
      return;
    }
    throw new Error("文件尚不可用，请在上传记录中查看状态");
  }

  async function selectFile(event: React.ChangeEvent<HTMLInputElement>) {
    const selected = event.target.files?.[0];
    event.target.value = "";
    await select(selected);
  }

  async function select(selected?: File) {
    if (!selected || busy || disabled || controller.current) return;
    const selectingZip = isZip || (zipImports && /\.zip$/i.test(selected.name));
    if (!!selectingZip !== /\.zip$/i.test(selected.name)) {
      notifyError(
        new Error(
          isZip ? "请上传ZIP素材文件" : "ZIP请通过独立的导入素材ZIP入口导入",
        ),
        "素材未上传",
      );
      return;
    }
    const selectedTarget = target;
    const previous = selection.current;
    let inferred = selectingZip ? "asset_zip" : previous?.biz;
    if (!inferred) {
      const abort = new AbortController();
      controller.current = abort;
      setBusy(true);
      onBusyChange(true);
      try {
        const choice = unwrap(
          await api.resolveFileUpload(
            { fileName: selected.name, size: selected.size },
            { signal: abort.signal },
          ),
        );
        inferred = choice.biz;
      } catch (error) {
        notifyError(error, "文件用途读取失败");
        return;
      } finally {
        controller.current = null;
        if (mounted.current) {
          setBusy(false);
          onBusyChange(false);
        }
      }
    }
    if (!mounted.current) return;
    if (
      !inferred ||
      (!selectingZip && !(picker?.bizTypes ?? assetBizTypes).includes(inferred))
    ) {
      notifyError(
        new Error("请选择支持的图片、视频、音频或文本文件"),
        "素材未上传",
      );
      return;
    }
    const command = crypto.randomUUID();
    uploadTarget(ownerId, command, selectedTarget);
    setFile(selected);
    setBiz(inferred);
    setCommandId(command);
    setResumeId(previous?.fileId);
    setActiveId(undefined);
    setProgress(0);
    onUploadOpenChange(false);
    await upload({
      file: selected,
      biz: inferred,
      commandId: command,
      resumeId: previous?.fileId,
    });
  }

  async function act(
    action: "cancel" | "read" | "complete",
    item: CanvasContract.FileTransferVO,
  ) {
    setBusy(true);
    try {
      if (!item.fileId) throw new Error("上传记录缺少文件标识，请重新读取");
      if (action === "read") {
        await downloadFixedFile(
          { fileId: item.fileId, fileVersion: item.version },
          downloadAbort.signal,
        );
      } else {
        unwrap(
          await (
            action === "cancel"
              ? api.cancelFileTransfer
              : api.completeFileTransfer
          )({ fileId: item.fileId }),
        );
        if (
          action === "cancel" &&
          (activeId === item.fileId || resumeId === item.fileId)
        ) {
          setFile(undefined);
          setCommandId(undefined);
          setResumeId(undefined);
          setActiveId(undefined);
          setProgress(0);
          if (fileInput.current) fileInput.current.value = "";
        }
        await files.mutate();
      }
    } catch (error) {
      notifyError(error, "文件操作未完成");
    } finally {
      if (mounted.current) setBusy(false);
    }
  }

  useImperativeHandle(uploadRef, () => ({
    selectFile() {
      if (busy || disabled) return;
      selection.current = null;
      fileInput.current?.click();
    },
    async uploadFile(file) {
      if (busy || disabled) return;
      selection.current = null;
      await select(file);
    },
  }));

  return (
    <div
      className={
        showRecords ? "file-transfers flex flex-col gap-6" : "contents"
      }
    >
      <Input
        ref={fileInput}
        id={inputId}
        type="file"
        accept={
          isZip
            ? ".zip,application/zip"
            : zipImports
              ? "image/*,video/*,audio/*,.txt,.md,.zip,application/zip"
              : (picker?.accept ?? "image/*,video/*,audio/*,.txt,.md")
        }
        className="hidden"
        aria-label={`选择${label}`}
        onChange={selectFile}
      />
      <Dialog open={uploadOpen} onOpenChange={onUploadOpenChange}>
        <DialogContent
          className="project-action-dialog"
          onCloseAutoFocus={onUploadCloseAutoFocus}
        >
          <DialogHeader>
            <DialogTitle>
              {resumeId ? `继续上传${label}` : `上传${label}`}
            </DialogTitle>
            <DialogDescription>
              {isZip
                ? "ZIP核验通过后查看原文件名与目录，再确认开始导入。"
                : (picker?.description ??
                  "支持图片、视频、音频和文本；服务端核验通过后添加到参考素材。")}
            </DialogDescription>
          </DialogHeader>
          {file && (
            <p className="text-sm break-all text-muted-foreground">
              已选择：{file.name}（{formatBytes(file.size)}）
            </p>
          )}
          <div className="flex flex-wrap items-center gap-2">
            <Button
              disabled={busy || disabled || !file}
              onClick={() => {
                onUploadOpenChange(false);
                void upload({ file, biz, commandId, resumeId });
              }}
            >
              {resumeId ? "继续上传" : "重试上传"}
            </Button>
            {transferring && (
              <Button
                variant="outline"
                onClick={() => controller.current?.abort()}
              >
                暂停传输
              </Button>
            )}
            <span role="status" className="text-sm text-muted-foreground">
              {busy && activeId
                ? `已传输 ${progress}%`
                : "文件格式和大小以服务端校验结果为准。"}
            </span>
          </div>
        </DialogContent>
      </Dialog>
      {transferring && (
        <div className="flex flex-col gap-2" role="status">
          <p className="text-sm break-all text-muted-foreground">
            正在上传「{file?.name}」，已传输 {progress}%
          </p>
          <Progress value={progress} aria-label={`${label}上传进度`} />
          {!showRecords && (
            <Button
              variant="outline"
              onClick={() => controller.current?.abort()}
            >
              暂停传输
            </Button>
          )}
        </div>
      )}
      {!showRecords && busy && !transferring && (
        <p
          role="status"
          className="flex items-center gap-2 text-sm text-muted-foreground"
        >
          <Spinner />
          正在核验{label}…
        </p>
      )}
      {showRecords ? (
        <>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p role="status" className="text-sm text-muted-foreground">
              {transferring
                ? `正在上传${file ? `「${file.name}」` : ""}，已传输 ${progress}%`
                : busy
                  ? `正在核验${label}…`
                  : zipImports
                    ? null
                    : isZip
                      ? "ZIP上传完成后可确认导入；未完成的上传可从记录续传。"
                      : "支持图片、视频、音频和文本；未完成的上传可从记录续传。"}
            </p>
            <div className="flex items-center gap-2">
              {transferring && (
                <Button
                  variant="outline"
                  onClick={() => controller.current?.abort()}
                >
                  暂停传输
                </Button>
              )}
              <Button
                variant="ghost"
                disabled={
                  busy || files.isValidating || zipImports?.isValidating
                }
                onClick={() => {
                  void files.mutate();
                  void zipImports?.refresh();
                }}
              >
                <RefreshCw data-icon="inline-start" aria-hidden="true" />
                刷新记录
              </Button>
            </div>
          </div>
          {files.isLoading || zipImports?.isLoading ? (
            <ContentState state="loading" title="正在读取上传记录" />
          ) : files.error || zipImports?.error ? (
            <ContentState
              state="error"
              title="上传记录暂时无法读取"
              description="请重试读取上传记录。"
              actions={
                <Button
                  disabled={files.isValidating || zipImports?.isValidating}
                  onClick={() => {
                    void files.mutate();
                    void zipImports?.refresh();
                  }}
                >
                  {files.isValidating || zipImports?.isValidating
                    ? "正在重试…"
                    : "重试"}
                </Button>
              }
            />
          ) : !visibleFiles.length && !zipImports?.items.length ? (
            <Empty className="min-h-72">
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <FileUp aria-hidden="true" />
                </EmptyMedia>
                <EmptyTitle>
                  {zipImports
                    ? "还没有上传任务"
                    : files.data?.length || cursors.length > 1
                      ? isZip
                        ? "当前页没有ZIP上传记录"
                        : "当前页没有普通素材上传记录"
                      : "还没有上传记录"}
                </EmptyTitle>
                <EmptyDescription>
                  {cursors.length > 1 && !files.data?.length
                    ? "可返回较新记录继续查看。"
                    : files.data?.length
                      ? "可翻阅更早记录，或选择文件开始上传。"
                      : isZip
                        ? "点击“导入素材ZIP”选择压缩包。"
                        : "点击“上传素材”选择图片、视频、音频或文本。"}
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <Table className="file-transfers-table" aria-label="上传文件列表">
              <TableHeader>
                <TableRow>
                  <TableHead>文件</TableHead>
                  <TableHead>类型</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>大小</TableHead>
                  <TableHead>操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {visibleFiles.map((item) => (
                  <TableRow key={item.fileId}>
                    <TableCell className="file-transfer-name">
                      {item.fileName}
                    </TableCell>
                    <TableCell>
                      {fileKindLabels[item.biz ?? ""] || "文件"}
                    </TableCell>
                    <TableCell>
                      <Badge variant="secondary">
                        {transferring && item.fileId === activeId
                          ? `上传中 · ${progress}%`
                          : (statusLabels[item.status ?? ""] ?? item.status)}
                      </Badge>
                    </TableCell>
                    <TableCell>{formatBytes(Number(item.size))}</TableCell>
                    <TableCell>
                      <div className="flex flex-wrap items-center gap-2">
                        {item.status === "READY" && (
                          <>
                            {onAdopt && (
                              <Button
                                variant="outline"
                                disabled={busy || disabled}
                                onClick={() =>
                                  void (
                                    item.biz === "asset_zip" && zipImports
                                      ? zipImports.prepare(
                                          item,
                                          uploadTarget(
                                            ownerId,
                                            item.fileId!,
                                            target,
                                          ),
                                        )
                                      : onAdopt(
                                          item,
                                          uploadTarget(
                                            ownerId,
                                            item.fileId!,
                                            target,
                                          ),
                                        )
                                  ).catch((cause: unknown) =>
                                    notifyError(cause, "素材尚未保存"),
                                  )
                                }
                              >
                                {item.biz === "asset_zip"
                                  ? "准备导入"
                                  : "添加到参考素材"}
                              </Button>
                            )}
                            <Button
                              variant="outline"
                              disabled={busy}
                              onClick={() => void act("read", item)}
                            >
                              {item.biz === "asset_zip" ? "下载源ZIP" : "下载"}
                            </Button>
                          </>
                        )}
                        {item.status === "UPLOADING" && (
                          <>
                            <Button
                              variant="outline"
                              disabled={busy}
                              onClick={() => {
                                selection.current = item;
                                fileInput.current?.click();
                              }}
                            >
                              恢复上传
                            </Button>
                            <Button
                              variant="ghost"
                              disabled={busy}
                              onClick={() => void act("complete", item)}
                            >
                              查询完成
                            </Button>
                          </>
                        )}
                        {["UPLOADING", "FAILED"].includes(
                          item.status ?? "",
                        ) && (
                          <Button
                            variant="ghost"
                            disabled={busy}
                            onClick={() => void act("cancel", item)}
                          >
                            取消上传
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
                {zipImports?.items.map((job) => (
                  <TableRow key={`import-${job.jobId}`}>
                    <TableCell className="file-transfer-name">
                      {job.sourceArchiveName}
                    </TableCell>
                    <TableCell>ZIP导入</TableCell>
                    <TableCell>
                      <Badge variant="secondary">
                        {zipImports.label(job.status)}
                      </Badge>
                    </TableCell>
                    <TableCell>—</TableCell>
                    <TableCell>
                      <Button
                        variant="outline"
                        disabled={zipImports.disabled}
                        onClick={() => zipImports.view(job)}
                      >
                        查看导入任务
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
          {(!!files.data?.length || cursors.length > 1) && (
            <nav
              aria-label="上传记录分页"
              className="workspace-pagination flex"
            >
              <Button
                variant="outline"
                disabled={busy || files.isValidating || cursors.length === 1}
                onClick={() => setCursors((values) => values.slice(0, -1))}
              >
                较新文件
              </Button>
              <span className="text-sm text-muted-foreground">
                第 {cursors.length} 页，每页最多 50 条
              </span>
              <Button
                variant="outline"
                disabled={
                  busy || files.isValidating || files.data?.length !== 50
                }
                onClick={() => {
                  const last = files.data?.at(-1)?.registrationId;
                  if (last != null)
                    setCursors((values) => [...values, String(last)]);
                }}
              >
                更早文件
              </Button>
            </nav>
          )}
        </>
      ) : (
        (busy || transferring) && (
          <p role="status" className="text-sm text-muted-foreground">
            {transferring
              ? `正在上传${file ? `「${file.name}」` : ""}，已传输 ${progress}%`
              : "正在核验图片…"}
          </p>
        )
      )}
    </div>
  );
}
