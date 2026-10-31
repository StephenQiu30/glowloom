"use client";
import { ContentState } from "@/components/layout/content-state";
import { useState } from "react";
import useSWR from "swr";
import { useCanvasHistory } from "../hooks/use-canvas-history";
import * as api from "@/components/canvas/runtime/api/canvas-document-bff";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
} from "@/components/ui/alert-dialog";
import {
  notifyError,
  useErrorNotification,
} from "@/components/canvas/runtime/components/feedback/error-notification";
import { CanvasVersionPreview } from "./canvas-version-preview";
import { fromCanvasGraph } from "../lib/canvas-document-adapter";
import {
  canvasVersionReasons,
  isCurrentCanvasVersion,
} from "../lib/canvas-version-history";
import type { CanvasScope } from "../hooks/use-canvas-document";

export function CanvasVersionHistory(
  props: Parameters<typeof CanvasVersionHistorySession>[0],
) {
  return (
    <CanvasVersionHistorySession
      key={[
        props.identity,
        props.scope,
        props.document.canvasId,
        props.document.revision,
        props.document.projectBindingRevision,
        props.open,
      ].join(":")}
      {...props}
    />
  );
}

function CanvasVersionHistorySession({
  open,
  onOpenChange,
  scope,
  identity,
  enabled,
  document,
  canRestore,
  restoring,
  onRestore,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  scope: CanvasScope;
  identity: string;
  enabled: boolean;
  document: CanvasContract.CanvasVO;
  canRestore: boolean;
  restoring: boolean;
  onRestore: (
    snapshotId: string,
  ) => Promise<CanvasContract.CanvasCommandResultVO | null>;
}) {
  const { history, page, restart, previous, next } = useCanvasHistory({
    open,
    enabled,
    scope,
    identity,
    document,
  });
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [confirm, setConfirm] = useState(false);
  const canvasId = String(document.canvasId);
  const preview = useSWR(
    enabled && open && selectedId
      ? ["canvas-version-preview", identity, scope, canvasId, selectedId]
      : null,
    async () => {
      const snapshot = unwrap(
        await (
          scope === "LOCAL" ? api.localGetCanvasSnapshot : api.getCanvasSnapshot
        )({ canvasId, snapshotId: selectedId! }),
      );
      fromCanvasGraph(snapshot.graph);
      return snapshot;
    },
    { shouldRetryOnError: false, revalidateOnFocus: false },
  );
  useErrorNotification(history.error, "版本历史读取失败");
  useErrorNotification(preview.error, "历史版本预览失败");
  const selected = preview.data;
  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent className="data-[side=right]:w-full data-[side=right]:sm:max-w-4xl">
          <SheetHeader>
            <SheetTitle>版本历史</SheetTitle>
            <SheetDescription>
              修改会自动保留。查看历史不会改变当前画布。
            </SheetDescription>
          </SheetHeader>
          <div className="grid min-h-0 flex-1 gap-6 overflow-y-auto p-4 pt-0 md:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
            <div className="space-y-3">
              <p className="text-sm text-muted-foreground">
                当前版本 {String(document.revision)}
              </p>
              {history.isLoading && (
                <ContentState
                  className="min-h-40"
                  state="loading"
                  title="正在读取版本历史"
                />
              )}
              {history.error && (
                <ContentState
                  className="min-h-40"
                  state="error"
                  title="版本历史暂不可读取"
                  actions={
                    <Button
                      variant="outline"
                      onClick={() => {
                        restart();
                      }}
                    >
                      重新读取历史
                    </Button>
                  }
                />
              )}
              {history.data?.records.map((version) => (
                <Button
                  key={String(version.snapshotId)}
                  variant={
                    selectedId === String(version.snapshotId)
                      ? "secondary"
                      : "outline"
                  }
                  className="h-auto w-full flex-col items-start gap-1 py-3 text-left whitespace-normal"
                  aria-pressed={selectedId === String(version.snapshotId)}
                  onClick={() => setSelectedId(String(version.snapshotId))}
                >
                  <span>
                    版本 {String(version.sourceRevision)}
                    {isCurrentCanvasVersion(version, document) ? " · 当前" : ""}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {canvasVersionReasons[version.reason] ?? "保留的版本"} ·{" "}
                    {new Date(version.createdAt).toLocaleString()}
                  </span>
                  <span className="truncate">{version.title}</span>
                </Button>
              ))}
              {!history.isLoading &&
                !history.error &&
                history.data?.records.length === 0 && (
                  <ContentState
                    className="min-h-40"
                    title="暂无版本历史"
                    description="下次修改会自动保留当前内容。"
                  />
                )}
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page === 1}
                  onClick={previous}
                >
                  上一页
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!history.data?.hasMore}
                  onClick={next}
                >
                  下一页
                </Button>
              </div>
            </div>
            <div className="min-w-0 space-y-4">
              {!selectedId && (
                <ContentState className="min-h-40" title="选择版本，预览内容" />
              )}
              {preview.isLoading && (
                <ContentState
                  className="min-h-40"
                  state="loading"
                  title="正在加载只读预览"
                />
              )}
              {preview.error && (
                <ContentState
                  className="min-h-40"
                  state="error"
                  title="预览暂时无法读取"
                  actions={
                    <Button onClick={() => void preview.mutate()}>
                      重新读取预览
                    </Button>
                  }
                />
              )}
              {selected && (
                <>
                  <h2 className="font-medium">
                    {selected.title} · 版本 {String(selected.sourceRevision)}
                  </h2>
                  <CanvasVersionPreview
                    key={String(selected.snapshotId)}
                    graph={selected.graph}
                    enabled={enabled && open && scope === "USER"}
                    mediaContext={{
                      canvasId: document.canvasId,
                      bindingRevision: document.projectBindingRevision,
                      snapshotId: selected.snapshotId,
                    }}
                  />
                  <Button
                    disabled={
                      !canRestore ||
                      restoring ||
                      String(selected.projectBindingRevision) !==
                        String(document.projectBindingRevision) ||
                      isCurrentCanvasVersion(selected, document)
                    }
                    onClick={() => setConfirm(true)}
                  >
                    恢复此版本
                  </Button>
                  {String(selected.projectBindingRevision) !==
                    String(document.projectBindingRevision) && (
                    <p className="text-sm text-muted-foreground">
                      归入项目之前的个人历史仅可预览。
                    </p>
                  )}
                </>
              )}
            </div>
          </div>
        </SheetContent>
      </Sheet>
      <AlertDialog
        open={confirm && open}
        onOpenChange={(value) => {
          if (!restoring) setConfirm(value);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>恢复这个版本？</AlertDialogTitle>
            <AlertDialogDescription>
              当前修改会先自动保存并保留在历史中。恢复后可继续编辑，也可以再次恢复。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={restoring}>取消</AlertDialogCancel>
            <Button
              disabled={!selected || !canRestore || restoring}
              onClick={async () => {
                if (!selected) return;
                try {
                  if (await onRestore(String(selected.snapshotId))) {
                    setConfirm(false);
                    onOpenChange(false);
                  }
                } catch (error) {
                  notifyError(error, "恢复未完成");
                }
              }}
            >
              {restoring ? "正在恢复" : "确认恢复"}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
