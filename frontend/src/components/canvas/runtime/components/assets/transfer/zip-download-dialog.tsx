"use client";

import { useRef, useState, type MouseEvent } from "react";
import useSWR from "@/components/canvas/runtime/lib/use-live-query";
import * as api from "@/components/canvas/runtime/api/assetZipBffController";
import { ApiError, unwrap } from "@/components/canvas/runtime/lib/request";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { accountScope } from "@/components/canvas/runtime/store/account-slice";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatBytes } from "@/components/canvas/runtime/lib/media/image-utils";
import { toast } from "sonner";
import { useTransferAbort } from "../hooks/use-transfer-abort";

const labels: Record<string, string> = {
  QUEUED: "等待打包",
  PREPARING: "正在准备",
  PACKING: "正在打包",
  READY: "ZIP已就绪",
  FAILED: "打包未完成",
  CANCELLED: "已取消",
  EXPIRED: "已过期",
  CANCELLING: "正在取消",
};
type Selection = Omit<CanvasContract.AssetZipExportRequest, "commandId">;
type Pending =
  | { operation: "export"; body: CanvasContract.AssetZipExportRequest }
  | { operation: "cancel"; body: CanvasContract.AssetZipJobCommand };
export function ZipDownloadDialog({
  request: selection,
  job: existing,
  names,
  title = "打包下载",
  onClose,
}: {
  request?: Selection;
  job?: CanvasContract.AssetZipExportVO;
  names?: string[];
  title?: string;
  onClose: () => void;
}) {
  const { account } = useAccount();
  const abort = useTransferAbort();
  const writing = useRef(false);
  const nameInput = useRef<HTMLInputElement>(null);
  const [archiveName, setArchiveName] = useState(selection?.archiveName ?? "");
  const [texts, setTexts] = useState(selection?.texts ?? []);
  const [invalid, setInvalid] = useState(false);
  const [job, setJob] = useState(existing);
  const [pending, setPending] = useState<Pending>();
  const [unknown, setUnknown] = useState(false);
  const [busy, setBusy] = useState(false);
  const read = useSWR(
    account && job?.jobId && !unknown
      ? ["asset-export", accountScope(account), job.jobId]
      : null,
    (_key, options) =>
      api
        .getAssetZipExport(
          { jobId: job?.jobId },
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

      onError: (error) => notifyError(error, "ZIP进度暂不可读取"),
    },
  );
  const current = read.data?.jobId === job?.jobId ? read.data : job;
  async function run(command: Pending) {
    if (writing.current) return;
    writing.current = true;
    setBusy(true);
    setPending(command);
    try {
      const result = unwrap(
        await (command.operation === "export"
          ? api.exportAssetZip(command.body, { signal: abort.signal })
          : api.cancelAssetZipExport(command.body, { signal: abort.signal })),
      );
      abort.signal.throwIfAborted();
      if (!result.jobId) throw new Error("ZIP回执缺少任务标识");
      setJob(result);
      setUnknown(false);
      setPending(undefined);
      await read.mutate(result, { revalidate: false });
    } catch (cause) {
      if (abort.signal.aborted) return;
      const uncertain =
        !(cause instanceof ApiError) || Math.floor(cause.code / 100) >= 500;
      setUnknown(uncertain);
      if (!uncertain) setPending(undefined);
      notifyError(
        cause,
        uncertain ? "打包结果待核对，请查询原任务" : "ZIP打包未完成",
      );
    } finally {
      writing.current = false;
      if (!abort.signal.aborted) setBusy(false);
    }
  }
  function start() {
    if (!selection || writing.current) return;
    if (archiveName && !/\.zip$/i.test(archiveName)) {
      setInvalid(true);
      nameInput.current?.focus();
      toast.error("包名需以.zip结尾");
      return;
    }
    setInvalid(false);
    void run({
      operation: "export",
      body: {
        ...selection,
        ...(archiveName ? { archiveName } : {}),
        ...(texts.length ? { texts } : {}),
        commandId: crypto.randomUUID(),
      },
    });
  }
  async function query() {
    if (writing.current) return;
    writing.current = true;
    setBusy(true);
    try {
      const result = unwrap(
        await api.getAssetZipExport(
          job?.jobId
            ? { jobId: job.jobId }
            : { commandId: pending?.body.commandId },
          { signal: abort.signal },
        ),
      );
      abort.signal.throwIfAborted();
      setJob(result);
      setUnknown(false);
      setPending(undefined);
      await read.mutate(result, { revalidate: false });
    } catch (cause) {
      notifyError(cause, "原ZIP任务尚不可读取");
    } finally {
      writing.current = false;
      if (!abort.signal.aborted) setBusy(false);
    }
  }
  async function download(
    event: MouseEvent<HTMLAnchorElement>,
    part: CanvasContract.Part,
  ) {
    event.preventDefault();
    if (!current?.jobId || writing.current) return;
    writing.current = true;
    setBusy(true);
    try {
      const result = unwrap(
        await api.getAssetZipExport(
          { jobId: current.jobId },
          { signal: abort.signal },
        ),
      );
      abort.signal.throwIfAborted();
      const fresh = result.parts?.find((item) => item.jobId === part.jobId);
      if (
        fresh?.status !== "READY" ||
        !fresh.url?.startsWith("/api/file/content?ticket=")
      )
        throw new Error("ZIP尚不可下载，请重新读取任务");
      const anchor = document.createElement("a");
      anchor.href = fresh.url;
      anchor.download = fresh.archiveName ?? "素材.zip";
      anchor.rel = "noreferrer";
      document.body.append(anchor);
      anchor.click();
      anchor.remove();
      setJob(result);
      await read.mutate(result, { revalidate: false });
      toast.info("已发起ZIP下载");
    } catch (cause) {
      notifyError(cause, "ZIP下载尚未发起");
    } finally {
      writing.current = false;
      if (!abort.signal.aborted) setBusy(false);
    }
  }
  const count =
    (selection?.assetIds?.length ?? 0) +
    (selection?.files?.length ?? 0) +
    (selection?.texts?.length ?? 0);
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="dialog-assets max-h-dialog overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {current
              ? "每个ZIP独立解压，保留素材原文件名与目录；下载前重新授权原包。"
              : `已选择 ${count} 项素材。确认后在后台按原名打包。`}
          </DialogDescription>
        </DialogHeader>
        {!current && (
          <FieldGroup>
            {names?.length ? (
              <ul className="max-h-72 overflow-y-auto text-sm">
                {names.map((name, index) => (
                  <li className="break-all whitespace-pre-wrap" key={index}>
                    {name}
                  </li>
                ))}
              </ul>
            ) : null}
            <Field data-invalid={invalid}>
              <FieldLabel htmlFor="asset-zip-name">ZIP包名</FieldLabel>
              <Input
                id="asset-zip-name"
                ref={nameInput}
                value={archiveName}
                maxLength={255}
                aria-invalid={invalid}
                disabled={busy || unknown}
                placeholder="例如：创作素材.zip"
                onChange={(event) => setArchiveName(event.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                完整上传批次沿用原ZIP名；单项采用原文件名。混合素材需填写包名。
              </p>
            </Field>
            {texts.map((text, index) => (
              <Field key={index}>
                <FieldLabel htmlFor={`asset-zip-text-${index}`}>
                  文字文件名
                </FieldLabel>
                <Input
                  id={`asset-zip-text-${index}`}
                  value={text.fileName}
                  disabled={busy || unknown}
                  onChange={(event) =>
                    setTexts((current) =>
                      current.map((item, position) =>
                        position === index
                          ? { ...item, fileName: event.target.value }
                          : item,
                      ),
                    )
                  }
                />
              </Field>
            ))}
          </FieldGroup>
        )}
        {current && (
          <>
            <p role="status" className="text-sm">
              {labels[current.status ?? ""] ?? "状态待核对"}
            </p>
            <Table aria-label="独立ZIP下载包">
              <TableHeader>
                <TableRow>
                  <TableHead>ZIP文件</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>大小</TableHead>
                  <TableHead>操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {current.parts?.map((part) => (
                  <TableRow key={part.jobId}>
                    <TableCell className="break-all whitespace-pre-wrap">
                      {part.archiveName}
                    </TableCell>
                    <TableCell>
                      <Badge variant="secondary">
                        {labels[part.status ?? ""] ?? part.status}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      {part.size != null
                        ? formatBytes(Number(part.size))
                        : "待打包"}
                    </TableCell>
                    <TableCell>
                      {part.status === "READY" &&
                      part.url?.startsWith("/api/file/content?ticket=") ? (
                        <Button variant="outline" disabled={busy} asChild>
                          <a
                            href={part.url}
                            download={part.archiveName}
                            onClick={(event) => void download(event, part)}
                          >
                            下载ZIP
                          </a>
                        </Button>
                      ) : null}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </>
        )}
        {busy && (
          <p role="status" className="flex items-center gap-2 text-sm">
            <Spinner />
            正在提交或读取ZIP任务…
          </p>
        )}
        <DialogFooter className="flex-wrap sm:justify-center sm:[&>button]:w-40">
          <Button variant="outline" onClick={onClose}>
            关闭
          </Button>
          {!current && !unknown && (
            <Button disabled={busy || !count} onClick={start}>
              开始打包
            </Button>
          )}
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
                <Button disabled={busy} onClick={() => void run(pending)}>
                  重试原命令
                </Button>
              )}
            </>
          )}
          {current && !unknown && (
            <Button
              variant="outline"
              disabled={busy || read.isValidating}
              onClick={() => void query()}
            >
              重新读取任务
            </Button>
          )}
          {current?.jobId &&
            !unknown &&
            ["QUEUED", "PREPARING", "PACKING"].includes(
              current.status ?? "",
            ) && (
              <Button
                variant="outline"
                disabled={busy}
                onClick={() =>
                  current.jobId &&
                  void run({
                    operation: "cancel",
                    body: {
                      jobId: current.jobId,
                      commandId: crypto.randomUUID(),
                    },
                  })
                }
              >
                取消打包
              </Button>
            )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
