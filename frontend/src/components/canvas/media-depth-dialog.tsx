"use client";
import { useState, useCallback } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  MediaDepthPanel,
  type MediaDepthPanelProps,
} from "./media-depth-panel";
export function MediaDepthDialog({
  onClose,
  onLockChange,
  restoreFocus,
  ...props
}: MediaDepthPanelProps & { onClose: () => void; restoreFocus?: () => void }) {
  const [locked, setLocked] = useState(false);
  const reportLock = useCallback(
    (value: boolean) => {
      setLocked(value);
      onLockChange?.(value);
    },
    [onLockChange],
  );
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !locked) onClose();
      }}
    >
      <DialogContent
        className="max-h-[90dvh] min-w-0 overflow-y-auto sm:max-w-3xl"
        showCloseButton={false}
        onCloseAutoFocus={
          restoreFocus
            ? (event) => {
                event.preventDefault();
                restoreFocus();
              }
            : undefined
        }
        onEscapeKeyDown={(event) => {
          if (locked) event.preventDefault();
        }}
        onInteractOutside={(event) => {
          if (locked) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>视频深度</DialogTitle>
          <DialogDescription>
            冻结已保存的视频原件，生成完整相对深度，审核后可下载或采纳到当前画布。
          </DialogDescription>
        </DialogHeader>
        <MediaDepthPanel {...props} onLockChange={reportLock} />
        <DialogFooter>
          <Button variant="ghost" disabled={locked} onClick={onClose}>
            关闭深度任务
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
