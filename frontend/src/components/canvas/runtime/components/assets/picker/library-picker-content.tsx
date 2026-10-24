"use client";
import { useEffect, useRef, useState } from "react";
import { ProjectAssetDialog } from "@/components/media/project-asset-dialog";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { getMediaPreview, uploadMediaAsset } from "@/api/media";
import { requirePreview, validateUpload } from "@/components/media/library";
import { isProjectID } from "@/components/projects/project";
import type { InsertAssetPayload } from "./asset-picker-modal";

type Props = {
  projectId?: string;
  initialScope?: string;
  compact?: boolean;
  contained?: boolean;
  insertOnUpload?: boolean;
  kinds?: readonly string[];
  onInsert: (payload: InsertAssetPayload) => void;
  sourceFile?: File;
  sourceFiles?: readonly File[];
  onSourceFilesComplete?: () => void;
};
export async function canvasAssetPayload(
  projectID: string,
  asset: API.AssetSummary,
  signal?: AbortSignal,
): Promise<InsertAssetPayload> {
  if (!isProjectID(asset.id) || asset.project_id !== projectID)
    throw new Error("素材身份不匹配");
  const result = await getMediaPreview(
    { pid: projectID, asset_id: asset.id },
    { signal },
  );
  if (result.asset?.revision !== asset.revision)
    throw new Error("素材版本已变化，请重新选择");
  const url = requirePreview(result, asset.id);
  const shared = {
    title: asset.file_name ?? "素材",
    fileId: asset.id,
    fileVersion: String(asset.revision),
  };
  if (asset.kind === "image")
    return {
      ...shared,
      kind: "image",
      dataUrl: url,
      width: asset.width,
      height: asset.height,
    };
  if (asset.kind === "video")
    return {
      ...shared,
      kind: "video",
      url,
      width: asset.width,
      height: asset.height,
    };
  if (asset.kind === "audio") return { ...shared, kind: "audio", url };
  throw new Error("此素材类型不能插入画布");
}
export function LibraryPickerContent({
  projectId,
  onInsert,
  kinds,
  sourceFile,
  sourceFiles,
  onSourceFilesComplete,
}: Props) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const input = useRef<HTMLInputElement>(null);
  const controller = useRef<AbortController | null>(null);
  const receipts = useRef(
    new Map<
      File,
      { key: string; asset?: API.AssetSummary; inserted?: boolean }
    >(),
  );
  useEffect(() => () => controller.current?.abort(), []);
  const files = sourceFiles?.length
    ? sourceFiles
    : sourceFile
      ? [sourceFile]
      : [];
  async function upload(selected: readonly File[]) {
    if (!projectId || busy) return;
    setBusy(true);
    setError("");
    const abort = new AbortController();
    controller.current = abort;
    try {
      for (const file of selected) {
        const invalid = validateUpload(file);
        if (invalid) throw new Error(invalid);
        let receipt = receipts.current.get(file);
        if (!receipt) {
          receipt = { key: crypto.randomUUID() };
          receipts.current.set(file, receipt);
        }
        if (receipt.inserted) continue;
        if (!receipt.asset) {
          const value = await uploadMediaAsset(
            { pid: projectId },
            { local_review_confirmed: true },
            file,
            {
              signal: abort.signal,
              headers: { "Idempotency-Key": receipt.key },
            },
          );
          if (!value.asset) throw new Error("上传回执缺少素材");
          receipt.asset = value.asset;
        }
        const payload = await canvasAssetPayload(
          projectId,
          receipt.asset,
          abort.signal,
        );
        abort.signal.throwIfAborted();
        onInsert(payload);
        receipt.inserted = true;
      }
      onSourceFilesComplete?.();
    } catch (cause) {
      if (!abort.signal.aborted)
        setError(cause instanceof Error ? cause.message : "上传未完成");
    } finally {
      if (!abort.signal.aborted) setBusy(false);
    }
  }
  if (!projectId) return <p role="alert">缺少项目上下文，无法读取素材。</p>;
  return (
    <div className="flex min-h-40 flex-col items-start gap-3 p-3">
      <p className="text-sm text-muted-foreground">
        选择项目素材，或上传文件添加到画布。
      </p>
      <div className="flex flex-wrap gap-2">
        <Button onClick={() => setOpen(true)}>选择素材</Button>
        <Button
          variant="secondary"
          disabled={busy}
          onClick={() => input.current?.click()}
        >
          上传文件
        </Button>
      </div>
      <Input
        ref={input}
        type="file"
        hidden
        multiple
        accept="image/*,video/*,audio/*"
        onChange={(event) => {
          const chosen = Array.from(event.target.files ?? []);
          event.target.value = "";
          void upload(chosen);
        }}
      />
      {files.length > 0 && (
        <Button disabled={busy} onClick={() => void upload(files)}>
          {busy ? "正在上传" : `上传选中的 ${files.length} 个文件`}
        </Button>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {open && (
        <ProjectAssetDialog
          projectID={projectId}
          initialKind={kinds?.[0] ?? "image"}
          onClose={() => setOpen(false)}
          onAdd={async (assets) => {
            for (const asset of assets) {
              if (kinds && !kinds.includes(asset.kind ?? ""))
                throw new Error("所选素材类型与节点不匹配");
              onInsert(await canvasAssetPayload(projectId, asset));
            }
            setOpen(false);
          }}
        />
      )}
    </div>
  );
}
