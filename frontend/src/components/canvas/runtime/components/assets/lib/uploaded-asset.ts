export function uploadedAssetRequest(
  file: CanvasContract.FileTransferVO,
  target: Pick<
    CanvasContract.AssetSaveRequest,
    "libraryScope" | "projectId"
  > = {
    libraryScope: "GENERAL",
  },
): CanvasContract.AssetSaveRequest {
  const kinds: Record<string, string> = {
    project_image: "image",
    project_video: "video",
    project_audio: "audio",
    script_document: "text",
  };
  const kind = kinds[file.biz ?? ""];
  if (
    !kind ||
    file.status !== "READY" ||
    !file.fileId ||
    !file.version ||
    !file.fileName
  )
    throw new Error("文件尚未取得可用的素材版本");
  return {
    commandId: crypto.randomUUID(),
    ...target,
    kind,
    title: file.fileName,
    fileId: file.fileId,
    fileVersion: file.version,
    content: kind === "text" ? "" : undefined,
  };
}
