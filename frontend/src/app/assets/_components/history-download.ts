import { getOperation } from "@/api/operations";
import { downloadDocumentAsset, downloadMediaOriginal } from "@/api/media";
import { requireTask, type Task } from "@/components/generation/contracts";
import { RequestError } from "@/lib/request";
import { readErrorMessage } from "@/lib/read-error";
import type { HistoryCardTarget } from "./history-results";

export type DocumentDownloadFormat = "pdf" | "md";
export type HistoryDownloadResult = {
  key: string;
  title: string;
  ok: boolean;
  outputID: string;
  message?: string;
  format: DocumentDownloadFormat | "original";
};
const extensions: Record<string, string> = {
  "image/png": "png",
  "image/jpeg": "jpg",
  "image/webp": "webp",
  "image/gif": "gif",
  "video/mp4": "mp4",
  "video/webm": "webm",
  "video/quicktime": "mov",
  "audio/mpeg": "mp3",
  "audio/mp3": "mp3",
  "audio/wav": "wav",
  "audio/x-wav": "wav",
  "audio/ogg": "ogg",
  "audio/mp4": "m4a",
  "text/plain": "txt",
  "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
    "docx",
  "application/json": "json",
};

function save(blob: Blob, name: string) {
  if (!(blob instanceof Blob) || !blob.size)
    throw new RequestError("invalid_receipt");
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  document.body.appendChild(link);
  try {
    link.click();
  } finally {
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 0);
  }
}

export async function downloadHistoryCards(
  cards: HistoryCardTarget[],
  format: DocumentDownloadFormat,
  signal: AbortSignal,
  onProgress: (completed: number, total: number) => void,
) {
  const tasks = new Map<string, Promise<Task>>();
  const results: HistoryDownloadResult[] = [];
  const total = cards.reduce((count, card) => count + card.outputs.length, 0);
  for (const card of cards) {
    for (const output of card.outputs) {
      if (signal.aborted) return results;
      const title =
        card.task.prompt_preview || card.task.model_name || "生成结果";
      const base =
        title
          .replace(/[<>:"/\\|?*\u0000-\u001f]/g, "_")
          .replace(/[. ]+$/, "")
          .slice(0, 80) || "生成结果";
      const resultFormat =
        output.media_asset_id &&
        ["image", "video", "audio", "document"].includes(output.kind ?? "")
          ? "original"
          : format;
      try {
        if (
          card.task.status !== "completed" ||
          output.can_download !== true ||
          output.moderation_status !== "passed"
        )
          throw new RequestError("forbidden", 403);
        if (output.kind === "document" && output.media_asset_id) {
          const blob = await downloadDocumentAsset(
            { pid: card.task.project_id, asset_id: output.media_asset_id },
            { signal, responseType: "blob" },
          );
          const extension = extensions[blob.type.split(";")[0]] ?? "bin";
          if (!signal.aborted)
            save(blob, `${base}-${output.id.slice(0, 8)}.${extension}`);
        } else if (
          output.media_asset_id &&
          ["image", "video", "audio"].includes(output.kind ?? "")
        ) {
          const blob = await downloadMediaOriginal(
            { pid: card.task.project_id, asset_id: output.media_asset_id },
            { signal, responseType: "blob" },
          );
          const extension = extensions[blob.type.split(";")[0]] ?? "bin";
          if (!signal.aborted)
            save(blob, `${base}-${output.id.slice(0, 8)}.${extension}`);
        } else {
          let task = tasks.get(card.task.id);
          if (!task) {
            task = getOperation(
              { id: card.task.id, project_id: card.task.project_id },
              { signal },
            ).then((value) => requireTask(value, card.task.project_id));
            tasks.set(card.task.id, task);
          }
          const detail = await task;
          if (detail.id !== card.task.id || detail.status !== "completed")
            throw new RequestError("invalid_receipt");
          const text = detail.outputs?.find(
            (item) =>
              item.id === output.id && item.moderation_status === "passed",
          )?.text;
          if (typeof text !== "string" || !text.trim())
            throw new RequestError("invalid_receipt");
          const blob =
            format === "pdf"
              ? await import("./document-pdf").then((module) =>
                  module.renderDocumentPdf(text, title, signal),
                )
              : new Blob([text], { type: "text/markdown;charset=utf-8" });
          if (!signal.aborted)
            save(blob, `${base}-${output.id.slice(0, 8)}.${format}`);
        }
        results.push({
          key: card.key,
          title,
          outputID: output.id,
          ok: true,
          format: resultFormat,
        });
      } catch (reason) {
        if (signal.aborted) return results;
        results.push({
          key: card.key,
          title,
          outputID: output.id,
          ok: false,
          format: resultFormat,
          message:
            reason instanceof RequestError
              ? readErrorMessage(reason)
              : reason instanceof Error
                ? reason.message
                : "下载失败，请重试。",
        });
      }
      onProgress(results.length, total);
    }
  }
  return results;
}
