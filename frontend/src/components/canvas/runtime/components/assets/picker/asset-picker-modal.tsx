"use client";
import { useEffect, useRef } from "react";
import { getScopeGeneration, getSessionGeneration } from "@/lib/request";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  LibraryPickerContent,
  canvasAssetPayload,
} from "./library-picker-content";
import { ProjectAssetDialog } from "@/components/media/project-asset-dialog";
type FixedReference = {
  fileId?: string;
  fileVersion?: string;
  publicAssetId?: string;
  publicRevision?: string;
};
export type InsertAssetPayload =
  | { kind: "text"; content: string; title: string }
  | (FixedReference & {
      kind: "image";
      dataUrl: string;
      width?: number;
      height?: number;
      title: string;
      storageKey?: string;
    })
  | (FixedReference & {
      kind: "video";
      url: string;
      title: string;
      storageKey?: string;
      width?: number;
      height?: number;
    })
  | (FixedReference & { kind: "audio"; url: string; title: string });

type Props = {
  projectId?: string;
  open: boolean;
  defaultTab?: string;
  initialUpload?: boolean;
  kinds?: readonly string[];
  onInsert: (payload: InsertAssetPayload) => void;
  onClose: () => void;
  sourceFile?: File;
  sourceFiles?: readonly File[];
  onSourceFilesComplete?: () => void;
};
export function AssetPickerModal({
  open,
  initialUpload,
  projectId,
  onInsert,
  onClose,
  kinds,
  sourceFile,
  sourceFiles,
  onSourceFilesComplete,
}: Props) {
  const request = useRef<AbortController | null>(null);
  useEffect(() => () => request.current?.abort(), [open, projectId]);
  if (!open) return null;
  if (projectId && !sourceFile && !sourceFiles?.length) {
    return (
      <ProjectAssetDialog
        projectID={projectId}
        initialKind={kinds?.[0] ?? "image"}
        allowedKinds={kinds ?? ["image", "video", "audio"]}
        selectionLimit={kinds ? 1 : undefined}
        confirmLabel={kinds ? "替换素材" : undefined}
        initialUpload={initialUpload}
        canvasPresentation
        onClose={onClose}
        onAdd={async (assets) => {
          request.current?.abort();
          const controller = new AbortController();
          request.current = controller;
          const scope = getScopeGeneration();
          const session = getSessionGeneration();
          const payloads = await Promise.all(
            assets.map((asset) =>
              canvasAssetPayload(projectId, asset, controller.signal),
            ),
          );
          if (
            controller.signal.aborted ||
            scope !== getScopeGeneration() ||
            session !== getSessionGeneration()
          )
            return;
          payloads.forEach((payload) => onInsert(payload));
          onClose();
        }}
      />
    );
  }
  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        if (!value) onClose();
      }}
    >
      <DialogContent className="dialog-canvas flex h-[min(37.5rem,calc(100dvh-2rem))] w-full flex-col overflow-hidden [@media(max-height:600px)]:gap-2">
        <DialogHeader className="shrink-0 pr-6">
          <DialogTitle>选择素材</DialogTitle>
          <DialogDescription className="[@media(max-height:600px)]:sr-only">
            从素材库选择内容，点击即可引用到画布。
          </DialogDescription>
        </DialogHeader>
        <LibraryPickerContent
          key={projectId ?? "standalone"}
          projectId={projectId}
          contained
          insertOnUpload
          onInsert={onInsert}
          kinds={kinds}
          sourceFile={sourceFile}
          sourceFiles={sourceFiles}
          onSourceFilesComplete={onSourceFilesComplete}
        />
      </DialogContent>
    </Dialog>
  );
}
