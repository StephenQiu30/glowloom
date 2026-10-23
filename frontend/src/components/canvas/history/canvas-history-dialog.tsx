"use client";

import { useId, useRef, useState, type ReactNode } from "react";
import useSWR from "@/components/canvas/runtime/lib/use-live-query";
import { listGenerationHistory } from "@/components/canvas/runtime/api/generationBffController";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { accountScope } from "@/components/canvas/runtime/store/account-slice";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { useAuthDialog } from "@/components/canvas/runtime/components/account/auth/auth-dialog-provider";
import { useErrorNotification } from "@/components/canvas/runtime/components/feedback/error-notification";
import { useTransferAbort } from "@/components/canvas/runtime/components/assets/hooks/use-transfer-abort";
import { taskLabels } from "@/components/canvas/runtime/components/generation/constants/task-state";
import { useIsMobile } from "@/hooks/use-mobile";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogTrigger,
  DialogClose,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyContent,
} from "@/components/ui/empty";
import { Spinner } from "@/components/ui/spinner";
import { X } from "lucide-react";
import { HistoryTaskThumbnail } from "./history-task-thumbnail";
import { HistoryTaskDetail } from "./history-task-detail";

type Modality = "image" | "video" | "text";
const modalities = ["image", "video", "text"] as const;
const labels = { all: "全部", image: "图片", video: "视频", text: "文字" };

export function CanvasHistoryDialog({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const [modality, setModality] = useState<Modality | "all">("all");
  const { account, ready, signingOut } = useAccount();
  const scope = ready && !signingOut ? accountScope(account) : "unavailable";
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent
        className="canvas-history-panel"
        showCloseButton={false}
        data-canvas-no-zoom
        data-canvas-shortcuts-ignore
      >
        {open && (
          <HistoryContent
            key={scope}
            modality={modality}
            setModality={setModality}
            close={() => setOpen(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function HistoryContent({
  modality,
  setModality,
  close,
}: {
  modality: Modality | "all";
  setModality: (value: Modality | "all") => void;
  close: () => void;
}) {
  const [view, setView] = useState<{
    modality: Modality | "all";
    pages: number;
    selectedId?: string;
    mobileDetail: boolean;
  }>({ modality, pages: 1, mobileDetail: false });
  const current =
    view.modality === modality
      ? view
      : { modality, pages: 1, mobileDetail: false };
  const { pages, selectedId, mobileDetail } = current;
  const instance = useId();
  const selectionTrigger = useRef<HTMLButtonElement | null>(null);
  const detailHeading = useRef<HTMLHeadingElement | null>(null);
  const isMobile = useIsMobile();
  const { account, ready, signingOut } = useAccount();
  const authenticated = ready && !signingOut && !!account;
  const openAuth = useAuthDialog();
  const scope = accountScope(account);
  const abort = useTransferAbort();
  const history = useSWR(
    authenticated
      ? ["canvas-generation-history", scope, modality, pages, instance]
      : null,
    async (_key, options) => {
      const groups = await Promise.all(
        (modality === "all" ? modalities : [modality]).map(async (kind) => {
          let beforeId: string | number | undefined;
          const records: CanvasContract.GenerationTaskVO[] = [];
          let hasMore = false;
          for (let page = 0; page < pages; page++) {
            const rows = unwrap(
              await listGenerationHistory(
                { modality: kind, beforeId },
                {
                  ...options,
                  signal: AbortSignal.any([
                    ...(options.signal instanceof AbortSignal
                      ? [options.signal]
                      : []),
                    abort.signal,
                  ]),
                },
              ),
            );
            abort.signal.throwIfAborted();
            records.push(...rows);
            hasMore = rows.length === 20;
            if (!hasMore || !rows.at(-1)?.id) break;
            beforeId = rows.at(-1)!.id;
          }
          return { records, hasMore };
        }),
      );
      const unique = new Map<string, CanvasContract.GenerationTaskVO>();
      for (const row of groups.flatMap((group) => group.records))
        if (row.id != null) unique.set(`${row.modality}:${row.id}`, row);
      return {
        modality,
        records: [...unique.values()].sort(
          (a, b) =>
            (b.createTime ?? "").localeCompare(a.createTime ?? "") ||
            String(b.id).localeCompare(String(a.id), "en", { numeric: true }),
        ),
        hasMore: groups.some((group) => group.hasMore),
      };
    },
    {
      shouldRetryOnError: false,
      revalidateOnFocus: false,
      keepPreviousData: true,
    },
  );
  useErrorNotification(history.error, "生成历史读取失败");
  const data = history.data?.modality === modality ? history.data : undefined;
  const rows = data?.records ?? [];
  const selected =
    rows.find((task) => String(task.id) === selectedId) ??
    (!isMobile || mobileDetail ? rows[0] : undefined);
  const returning = () => {
    setView({ ...current, mobileDetail: false });
    requestAnimationFrame(() =>
      selectionTrigger.current?.focus({ preventScroll: true }),
    );
  };
  return (
    <Tabs
      value={modality}
      onValueChange={(value) => setModality(value as Modality | "all")}
      className="min-h-0 flex-1 gap-4"
    >
      <DialogHeader className="history-dialog-header">
        {isMobile && mobileDetail && (
          <Button variant="ghost" size="sm" onClick={returning}>
            返回
          </Button>
        )}
        <DialogTitle
          ref={detailHeading}
          tabIndex={-1}
          className="min-w-0 flex-1 outline-none"
        >
          生成历史
        </DialogTitle>
        <DialogDescription className="hidden shrink-0 text-xs md:block">
          当前账号
        </DialogDescription>
        <span className="sr-only">
          当前账号提交的图片、视频和文字生成任务。
        </span>
        {isMobile && !mobileDetail && (
          <Button
            variant="ghost"
            size="sm"
            disabled={!authenticated || history.isValidating}
            onClick={() => void history.mutate()}
          >
            刷新
          </Button>
        )}
        <DialogClose asChild>
          <Button variant="ghost" size="icon-lg" aria-label="关闭生成历史">
            <X />
          </Button>
        </DialogClose>
      </DialogHeader>
      <div className="history-type-toolbar" hidden={isMobile && mobileDetail}>
        <TabsList aria-label="生成历史类型" className="h-10 w-full">
          {(["all", ...modalities] as const).map((kind) => (
            <TabsTrigger key={kind} value={kind}>
              {labels[kind]}
            </TabsTrigger>
          ))}
        </TabsList>
        <Button
          variant="ghost"
          size="sm"
          className="hidden md:inline-flex"
          disabled={!authenticated || history.isValidating}
          onClick={() => void history.mutate()}
        >
          {history.isValidating && <Spinner data-icon="inline-start" />}刷新
        </Button>
      </div>
      <TabsContent value={modality} className="flex min-h-0 flex-1 flex-col">
        {!authenticated || !rows.length ? (
          <div className="history-feedback">
            {authenticated && history.isLoading ? (
              <p
                role="status"
                className="flex items-center gap-2 text-sm text-muted-foreground"
              >
                <Spinner />
                正在读取历史
              </p>
            ) : (
              <Empty>
                <EmptyHeader>
                  <EmptyTitle>
                    {!authenticated
                      ? "请先登录"
                      : history.error
                        ? "历史暂不可读取"
                        : "还没有生成记录"}
                  </EmptyTitle>
                </EmptyHeader>
                {!authenticated || history.error ? (
                  <EmptyContent>
                    <Button
                      variant="outline"
                      disabled={history.isValidating}
                      onClick={() => {
                        if (!authenticated) {
                          close();
                          openAuth("login");
                        } else void history.mutate();
                      }}
                    >
                      {!authenticated ? "登录" : "重新读取"}
                    </Button>
                  </EmptyContent>
                ) : null}
              </Empty>
            )}
          </div>
        ) : (
          <div
            className="canvas-history-body"
            data-mobile-detail={mobileDetail}
          >
            <section className="history-record-list" aria-label="生成记录列表">
              <ScrollArea className="min-h-0 flex-1">
                <div className="flex min-w-0 flex-col gap-1">
                  {rows.map((task) => (
                    <Button
                      key={`${task.modality}:${task.id}`}
                      variant={selected?.id === task.id ? "secondary" : "ghost"}
                      className="history-task-row"
                      aria-pressed={selected?.id === task.id}
                      aria-label={`查看${labels[(task.modality ?? "image") as Modality] ?? "任务"} ${task.modelName ?? "生成任务"} ${task.createTime ?? ""}`}
                      onClick={(event) => {
                        selectionTrigger.current = event.currentTarget;
                        setView({
                          ...current,
                          selectedId: String(task.id),
                          mobileDetail: true,
                        });
                        requestAnimationFrame(() =>
                          detailHeading.current?.focus({ preventScroll: true }),
                        );
                      }}
                    >
                      <HistoryTaskThumbnail task={task} scope={scope} />
                      <span className="flex min-w-0 flex-1 flex-col gap-1 text-left">
                        <span className="flex min-w-0 items-center justify-between gap-2">
                          <span
                            className="truncate text-sm"
                            title={task.modelName}
                          >
                            {task.modelName || "生成任务"}
                          </span>
                          <span className="shrink-0 text-xs text-muted-foreground">
                            {task.state ? taskLabels[task.state] : "状态待核对"}
                          </span>
                        </span>
                        <span className="truncate text-xs text-muted-foreground">
                          {labels[(task.modality ?? "image") as Modality]} ·{" "}
                          {task.createTime
                            ? new Date(task.createTime).toLocaleString()
                            : ""}
                        </span>
                        <span className="truncate text-xs text-muted-foreground">
                          {task.prompt || "无提示词"}
                        </span>
                      </span>
                    </Button>
                  ))}
                </div>
              </ScrollArea>
              {history.error ? (
                <Button
                  variant="ghost"
                  disabled={history.isValidating}
                  onClick={() => void history.mutate()}
                >
                  重新读取
                </Button>
              ) : data?.hasMore ? (
                <Button
                  variant="ghost"
                  disabled={history.isValidating}
                  onClick={() => setView({ ...current, pages: pages + 1 })}
                >
                  {history.isValidating && <Spinner data-icon="inline-start" />}
                  加载更多
                </Button>
              ) : null}
            </section>
            {selected && (!isMobile || mobileDetail) && (
              <HistoryTaskDetail
                key={`${scope}:${selected.id}`}
                task={selected}
                updated={history.mutate}
              />
            )}
          </div>
        )}
      </TabsContent>
    </Tabs>
  );
}
