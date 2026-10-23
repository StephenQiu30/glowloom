"use client";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { LibraryPickerContent } from "./library-picker-content";
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
  kinds?: readonly string[];
  onInsert: (payload: InsertAssetPayload) => void;
  onClose: () => void;
  sourceFile?: File;
  sourceFiles?: readonly File[];
  onSourceFilesComplete?: () => void;
};
export function AssetPickerModal({
  open,
  projectId,
  onInsert,
  onClose,
  kinds,
  sourceFile,
  sourceFiles,
  onSourceFilesComplete,
}: Props) {
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
