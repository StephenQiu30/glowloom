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
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
type FixedReference = {
  fileId?: string;
  fileVersion?: string;
  publicAssetId?: string;
  publicRevision?: string;
};
export type InsertAssetPayload =
  | { kind: "subject"; subject: UnifiedSubject }
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
  getDocumentEpoch?: () => string | null;
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
  getDocumentEpoch,
  onInsert,
  onClose,
  kinds,
  sourceFile,
  sourceFiles,
  onSourceFilesComplete,
}: Props) {
  const request = useRef<AbortController | null>(null);
  const epochReader = useRef(getDocumentEpoch);
  useEffect(() => {
    epochReader.current = getDocumentEpoch;
  }, [getDocumentEpoch]);
  const opened = useRef<{
    active: boolean;
    projectID?: string;
    epoch: string | null | undefined;
    scope: number;
    session: number;
  }>({ active: false, epoch: undefined, scope: 0, session: 0 });
  useEffect(() => {
    opened.current = {
      active: open,
      projectID: projectId,
      epoch: epochReader.current?.(),
      scope: getScopeGeneration(),
      session: getSessionGeneration(),
    };
    return () => {
      opened.current.active = false;
      request.current?.abort();
    };
  }, [open, projectId]);
  function stillCurrent() {
    const snapshot = opened.current;
    return (
      snapshot.active &&
      snapshot.projectID === projectId &&
      (!epochReader.current || snapshot.epoch !== null) &&
      snapshot.epoch === epochReader.current?.() &&
      snapshot.scope === getScopeGeneration() &&
      snapshot.session === getSessionGeneration()
    );
  }
  if (!open) return null;
  if (projectId && !sourceFile && !sourceFiles?.length) {
    return (
      <ProjectAssetDialog
        key={projectId}
        projectID={projectId}
        initialKind={kinds?.[0] ?? "image"}
        allowedKinds={kinds ?? ["image", "video", "audio", "document"]}
        browseSources={!kinds}
        onUnifiedSubject={
          kinds
            ? undefined
            : async (subject) => {
                if (!stillCurrent())
                  throw new Error(
                    "画布或创作空间已变化，请关闭选择器后重新添加。",
                  );
                onInsert({ kind: "subject", subject });
                onClose();
              }
        }
        selectionLimit={kinds ? 1 : undefined}
        confirmLabel={kinds ? "替换素材" : undefined}
        initialUpload={initialUpload}
        canvasPresentation
        onClose={onClose}
        onAdd={async (assets) => {
          if (!stillCurrent())
            throw new Error("画布或创作空间已变化，请关闭选择器后重新添加。");
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
            !stillCurrent() ||
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
            从参考素材选择内容，点击即可引用到画布。
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
