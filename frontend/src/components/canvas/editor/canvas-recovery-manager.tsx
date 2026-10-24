"use client";

import { ContentState } from "@/components/layout/content-state";
import { useEffect, useRef, useState } from "react";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
} from "@/components/ui/alert-dialog";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { collaborationJournal } from "@glowloom/canvas-collab/journal";
type CanvasRecoveryUsage = Awaited<
  ReturnType<typeof collaborationJournal.summarizeActor>
>[number];

/** Account-only storage metadata; canvas qualification is never inferred here. */
export function CanvasRecoveryManager() {
  const { account, ready, signingOut } = useAccount();
  if (!ready || signingOut || !account) return null;
  return (
    <RecoveryManager key={String(account.id)} actorKey={`USER:${account.id}`} />
  );
}

function RecoveryManager({ actorKey }: { actorKey: string }) {
  const [open, setOpen] = useState(false);
  const [usage, setUsage] = useState<CanvasRecoveryUsage[]>([]);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const [selected, setSelected] = useState<CanvasRecoveryUsage | null>(null);
  const [removing, setRemoving] = useState(false);
  const active = useRef(true);
  const busy = useRef(false);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const read = async () => {
    setLoading(true);
    setFailed(false);
    try {
      const summaries = await collaborationJournal.summarizeActor(
        window.location.origin,
        actorKey,
      );
      if (active.current) setUsage(summaries);
    } catch (error) {
      if (active.current) {
        setFailed(true);
        notifyError(error, "页面内存记录读取未完成");
      }
    } finally {
      if (active.current) setLoading(false);
    }
  };
  const discard = async () => {
    if (!selected || busy.current || !active.current) return;
    busy.current = true;
    setRemoving(true);
    try {
      await collaborationJournal.removeCanvas({
        origin: window.location.origin,
        actorKey,
        canvasId: selected.canvasId,
      });
      if (!active.current) return;
      setSelected(null);
      await read();
    } catch (error) {
      if (active.current) notifyError(error, "页面内存记录清理未完成");
    } finally {
      busy.current = false;
      if (active.current) setRemoving(false);
    }
  };
  return (
    <>
      <Button
        variant="outline"
        onClick={() => {
          setOpen(true);
          void read();
        }}
      >
        管理页面内存恢复记录
      </Button>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!busy.current) setOpen(value);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>页面内存恢复记录</DialogTitle>
            <DialogDescription>
              仅管理此账户在当前页面的恢复空间。按画布编号显示用量；不展示画布内容。已删除或无法访问的画布也可明确放弃页面内存记录。
            </DialogDescription>
          </DialogHeader>
          {loading ? (
            <ContentState
              className="min-h-40"
              state="loading"
              title="正在读取页面内存记录"
            />
          ) : failed ? (
            <ContentState
              className="min-h-40"
              state="error"
              title="恢复记录暂时无法读取"
              description="请重试读取；尚未确认的恢复记录不会被清除。"
              actions={<Button onClick={() => void read()}>重试</Button>}
            />
          ) : (
            <div className="grid max-h-80 gap-4 overflow-y-auto">
              <p role="status">
                {usage.reduce((total, value) => total + value.count, 0)} / 16 条
                ·{" "}
                {(
                  usage.reduce((total, value) => total + value.bytes, 0) /
                  1024 /
                  1024
                ).toFixed(2)}{" "}
                / 64 MiB
              </p>
              {usage.length === 0 && (
                <ContentState
                  className="min-h-40"
                  title="没有页面内存恢复记录"
                />
              )}
              {usage.map((value) => (
                <div key={value.canvasId} className="grid gap-2">
                  <p>画布 {value.canvasId}</p>
                  <p className="text-sm text-muted-foreground">
                    {value.count} 条 · {(value.bytes / 1024).toFixed(1)} KiB
                    {value.unsupported
                      ? " · 包含无法恢复的版本或格式，原记录仍保留"
                      : ""}
                  </p>
                  <Button
                    variant="outline"
                    aria-label={`放弃画布 ${value.canvasId} 的页面内存记录`}
                    onClick={() => setSelected(value)}
                  >
                    放弃此画布的页面内存记录
                  </Button>
                </div>
              ))}
            </div>
          )}
        </DialogContent>
      </Dialog>
      <AlertDialog
        open={!!selected}
        onOpenChange={(value) => {
          if (!value && !busy.current) setSelected(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>放弃此画布的页面内存记录？</AlertDialogTitle>
            <AlertDialogDescription>
              将删除画布 {selected?.canvasId}{" "}
              在此账户的未同步草稿、原命令与未应用参数。已提交的服务端操作不会撤销。其他画布继续保留；关闭或刷新页面会释放全部未确认内容。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={removing}>继续保留</AlertDialogCancel>
            <Button
              variant="destructive"
              disabled={removing}
              onClick={() => void discard()}
            >
              {removing ? "正在清理" : "确认放弃页面内存记录"}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
