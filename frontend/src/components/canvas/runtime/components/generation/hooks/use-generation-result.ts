"use client";

import { useState } from "react";
import useSWR from "@/components/canvas/runtime/lib/use-live-query";
import * as api from "@/components/canvas/runtime/api/generation-bff";
import { previewFile } from "@/components/canvas/runtime/api/file-preview-bff";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { accountScope } from "@/components/canvas/runtime/store/account-slice";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { startFileDownload } from "@/components/canvas/runtime/components/assets/lib/original-download";
import { useTransferAbort } from "@/components/canvas/runtime/components/assets/hooks/use-transfer-abort";

/** Shared fixed-version preview, download and explicit task recovery. */
export function useGenerationResult(
  task: CanvasContract.GenerationTaskVO,
  modality: "image" | "video" | "text",
  updated: () => Promise<unknown>,
) {
  const { account } = useAccount();
  const scope = accountScope(account);
  const downloadAbort = useTransferAbort();
  const [busy, setBusy] = useState(false);
  const [downloadOpen, setDownloadOpen] = useState(false);
  const [selection, setSelection] = useState<{
    taskId: CanvasContract.GenerationTaskVO["id"];
    index: number;
  } | null>(null);
  const selectedIndex =
    selection && selection.taskId === task.id ? selection.index : 1;
  const selectedOutput =
    task.outputs?.find((item) => item.index === selectedIndex) ??
    task.outputs?.[0];
  const outputIndex = selectedOutput?.index ?? 1;
  const outputModality = selectedOutput?.modality ?? modality;
  const fixedFileId = selectedOutput?.fileId ?? task.fileId;
  const fixedVersion = selectedOutput?.fileVersion ?? task.fileVersion;
  const file = useSWR(
    account && task.id && task.state === "SUCCEEDED" && modality !== "text"
      ? [
          "generation-file",
          scope,
          task.id,
          outputIndex,
          fixedFileId,
          fixedVersion,
          outputModality,
        ]
      : null,
    async (_key, options) => {
      if (outputModality === "image") {
        if (!fixedFileId || !fixedVersion)
          throw new Error("生成图片缺少固定版本，请重新读取任务");
        return unwrap(
          await previewFile(
            {
              fileId: fixedFileId,
              version: fixedVersion,
              variant: "preview",
            },
            options,
          ),
        );
      }
      return {
        ...unwrap(
          await api.readGenerationFile(
            { taskId: task.id!, outputIndex },
            options,
          ),
        ),
        status: "READY",
      };
    },
    {
      shouldRetryOnError: false,
      revalidateOnFocus: false,

      onError: (error) => notifyError(error, "文件读取失败"),
    },
  );

  async function refresh() {
    if (!task.id || busy) return;
    setBusy(true);
    try {
      unwrap(await api.refreshGenerationTask({ taskId: task.id }));
      await updated();
    } catch (error) {
      notifyError(error, "任务查询失败");
    } finally {
      setBusy(false);
    }
  }

  const outputs = task.outputs?.length
    ? task.outputs
    : task.fileId && task.fileVersion
      ? [{ fileId: task.fileId, fileVersion: task.fileVersion }]
      : [];
  const exportFiles = outputs.flatMap((output) =>
    output.fileId && output.fileVersion
      ? [{ fileId: output.fileId, version: output.fileVersion }]
      : [],
  );
  async function downloadCurrent() {
    try {
      const result = unwrap(
        await api.readGenerationFile(
          { taskId: task.id!, outputIndex },
          { signal: downloadAbort.signal },
        ),
      );
      downloadAbort.signal.throwIfAborted();
      startFileDownload(result.url!);
    } catch (error) {
      notifyError(error, "结果下载尚未发起");
    }
  }

  return {
    file,
    busy,
    downloadOpen,
    setDownloadOpen,
    setSelection,
    outputIndex,
    outputModality,
    outputs,
    exportFiles,
    refresh,
    downloadCurrent,
  };
}
