"use client";

import { useState } from "react";
import { downloadDocumentAsset, downloadMediaOriginal } from "@/api/media";
import { DownloadButton } from "@/components/media/download-button";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

/** Reuses project-authorized media endpoints; never downloads the preview URL. */
export function ResultMediaActions({
  projectID,
  assetID,
  kind,
  completed,
}: {
  projectID: string;
  assetID: string;
  kind?: string;
  completed: boolean;
}) {
  const [open, setOpen] = useState(false);
  const previewable = ["image", "video", "audio"].includes(kind ?? "");
  return (
    <div className="mt-2 flex flex-wrap gap-2 max-md:[&_button]:min-h-11">
      {previewable ? (
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button variant="outline">预览结果</Button>
          </DialogTrigger>
          <DialogContent
            aria-describedby={undefined}
            className="sm:max-w-[calc(100vw-3rem)] lg:max-w-6xl max-md:[&_[data-slot=dialog-close]]:size-11"
          >
            <DialogTitle>结果预览</DialogTitle>

            <div className="relative flex h-[65dvh] min-h-40 min-w-0 items-center justify-center overflow-hidden rounded-lg bg-muted">
              <ProjectMediaPreview
                projectID={projectID}
                id={assetID}
                kind={kind}
                large
                fit="contain"
              />
            </div>
          </DialogContent>
        </Dialog>
      ) : null}
      {completed && (previewable || kind === "document") ? (
        <DownloadButton
          filename={`生成结果-${assetID}`}
          load={() =>
            (kind === "document"
              ? downloadDocumentAsset
              : downloadMediaOriginal)(
              { pid: projectID, asset_id: assetID },
              { responseType: "blob" },
            )
          }
        >
          下载原件
        </DownloadButton>
      ) : null}
    </div>
  );
}
