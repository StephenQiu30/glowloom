"use client";
import { PageTabsList as TabsList } from "@/components/canvas/runtime/components/layout/workspace/page-tabs-list";
import { PageHeader } from "@/components/canvas/runtime/components/layout/workspace/page-header";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useEffect, useEffectEvent, useMemo, useRef, useState } from "react";
import useSWR, { SWRConfig } from "swr";
import * as api from "@/components/canvas/runtime/api/notification-bff";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  DataTable,
  type DataTableColumn,
} from "@/components/canvas/runtime/components/account/list/data-table";
import { ListPagination } from "@/components/canvas/runtime/components/layout/workspace/list-pagination";
import {
  notifyError,
  useErrorNotification,
} from "@/components/canvas/runtime/components/feedback/error-notification";
import { Tabs, TabsTrigger } from "@/components/ui/tabs";
import { Empty, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import { useNotifications } from "./notification-provider";
import {
  decimal,
  notificationHref,
  unreadBadge,
} from "./lib/notification-state";

const PAGE_SIZES = [20, 50] as const;
function categoryLabel(category?: string) {
  return category === "TASK"
    ? "任务"
    : category === "COLLABORATION"
      ? "协作"
      : category === "ACCOUNT"
        ? "账户"
        : "通知";
}
function notificationTime(value?: string) {
  return value
    ? new Date(value).toLocaleString("zh-CN", {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      })
    : "—";
}

export function NotificationInbox({
  compact = false,
  onNavigate,
}: {
  compact?: boolean;
  onNavigate?: () => void;
}) {
  const inbox = useNotifications();
  const identity = useMemo(
    () => ({ scope: inbox.scope, cache: new Map() }),
    [inbox.scope],
  );
  return (
    <SWRConfig key={identity.scope} value={{ provider: () => identity.cache }}>
      <Inbox
        key={`${inbox.scope}:${inbox.summary?.dataEpoch ?? "initial"}`}
        compact={compact}
        onNavigate={onNavigate}
      />
    </SWRConfig>
  );
}

function Inbox({
  compact,
  onNavigate,
}: {
  compact: boolean;
  onNavigate?: () => void;
}) {
  const inbox = useNotifications();
  const params = useSearchParams();
  const [requests] = useState(() => new Set<AbortController>());
  useEffect(
    () => () => {
      requests.forEach((request) => request.abort());
      requests.clear();
    },
    [requests],
  );
  async function withSignal<T>(operation: (signal: AbortSignal) => Promise<T>) {
    const controller = new AbortController();
    requests.add(controller);
    try {
      return await operation(controller.signal);
    } finally {
      requests.delete(controller);
    }
  }
  const [category, setCategory] = useState<string>("all");
  const [unreadOnly, setUnreadOnly] = useState(false);
  const [pageSize, setPageSize] = useState(20);
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const parameter = params?.get("notificationId");
  const linkedId =
    !compact && parameter && /^[1-9]\d{0,18}$/.test(parameter)
      ? parameter
      : undefined;
  const [selection, setSelection] = useState<{ query?: string; id?: string }>();
  const selected = selection?.query === linkedId ? selection?.id : linkedId;
  function setSelected(id?: string) {
    if (id) attemptedReads.current.delete(id);
    setSelection({ query: linkedId, id });
  }
  const [busy, setBusy] = useState(false);
  const [readError, setReadError] = useState<string>();
  const attemptedReads = useRef(new Set<string>());
  const filtersRef = useRef<HTMLDivElement>(null);
  const detailTrigger = useRef<HTMLButtonElement | null>(null);
  const nextDetailTrigger = useRef<HTMLButtonElement | null>(null);
  const [uncertainAll, setUncertainAll] = useState(false);
  const pendingAll = useRef<
    CanvasContract.NotificationReadAllRequest | undefined
  >(undefined);
  const flight = useRef(false);
  const cursor = cursors.at(-1);
  const epoch = inbox.summary?.dataEpoch;
  const page = useSWR(
    epoch
      ? [
          "notification-page",
          inbox.scope,
          epoch,
          category,
          unreadOnly,
          compact ? 5 : pageSize,
          cursor,
        ]
      : null,
    async () => {
      const received = await withSignal(async (signal) =>
        unwrap(
          await api.listMyNotifications(
            {
              pageSize: compact ? 5 : pageSize,
              current: 1,
              cursor,
              category: category === "all" ? undefined : category,
              unreadOnly,
            },
            { timeout: 10000, signal },
          ),
        ),
      );
      if (!inbox.observe(received, epoch))
        throw new Error("通知已更新，请重新读取。");
      return received;
    },
    { shouldRetryOnError: false, revalidateOnFocus: true },
  );
  const detail = useSWR(
    epoch && selected ? ["notification-detail", inbox.scope, selected] : null,
    async () =>
      withSignal(async (signal) =>
        unwrap(
          await api.getNotificationVoById(
            { id: selected! },
            { timeout: 10000, signal },
          ),
        ),
      ),
    { shouldRetryOnError: false },
  );
  const pageVersion = page.data?.mailboxVersion;
  const pageError = page.error ?? (!epoch ? inbox.error : undefined);
  const pageLoading = page.isLoading || (!epoch && !pageError);
  useErrorNotification(pageError, "通知读取失败");
  useErrorNotification(detail.error, "通知详情读取失败");
  const mailboxVersion = inbox.summary?.mailboxVersion;
  const { mutate: mutatePage } = page;
  const { mutate: mutateDetail } = detail;
  const lastReadTick = useRef(inbox.readTick);
  useEffect(() => {
    const tickChanged = lastReadTick.current !== inbox.readTick;
    lastReadTick.current = inbox.readTick;
    if (
      tickChanged ||
      (pageVersion !== undefined &&
        mailboxVersion !== undefined &&
        decimal(pageVersion)! < decimal(mailboxVersion)!)
    )
      void mutatePage().catch(() => undefined);
  }, [pageVersion, mailboxVersion, inbox.readTick, mutatePage]);
  useEffect(() => {
    if (selected) void mutateDetail().catch(() => undefined);
  }, [mailboxVersion, inbox.readTick, selected, mutateDetail]);

  async function refresh() {
    setCursors([undefined]);
    await Promise.allSettled([inbox.refresh(), page.mutate(), detail.mutate()]);
  }
  async function markRead(id?: CanvasContract.NotificationVO["id"]) {
    if (flight.current) return;
    if (!id && !pendingAll.current && page.data?.maxInboxSeq === undefined)
      return;
    flight.current = true;
    setBusy(true);
    setReadError(undefined);
    const requestEpoch = epoch;
    try {
      let response;
      if (id)
        response = await withSignal(async (signal) =>
          unwrap(
            await api.readNotification({ id }, { timeout: 10000, signal }),
          ),
        );
      else {
        pendingAll.current ??= { throughSeq: page.data!.maxInboxSeq! };
        const request = pendingAll.current;
        response = await withSignal(async (signal) =>
          unwrap(
            await api.readAllNotifications(request, {
              timeout: 10000,
              signal,
            }),
          ),
        );
        pendingAll.current = undefined;
        setUncertainAll(false);
      }
      inbox.observe(response, requestEpoch);
      await Promise.allSettled([page.mutate(), detail.mutate()]);
    } catch (error) {
      if (error instanceof Error && error.name === "AbortError") return;
      if (!id) setUncertainAll(true);
      else setReadError(String(id));
      notifyError(error, id ? "未能标记已读" : "全部已读尚未确认");
    } finally {
      flight.current = false;
      setBusy(false);
    }
  }
  const item = detail.data;
  const href = item && notificationHref(item);
  const readOpenedDetail = useEffectEvent(
    (id: CanvasContract.NotificationVO["id"]) => void markRead(id),
  );
  useEffect(() => {
    if (
      !selected ||
      !item ||
      item.read ||
      detail.error ||
      busy ||
      String(item.id) !== selected ||
      attemptedReads.current.has(selected)
    )
      return;
    attemptedReads.current.add(selected);
    readOpenedDetail(item.id);
  }, [selected, item, detail.error, busy]);
  const count = unreadBadge(inbox.summary?.unreadCount);
  const columns: DataTableColumn<CanvasContract.NotificationVO>[] = [
    {
      id: "notification",
      header: "通知",
      cell: ({ row }) => (
        <div className="flex w-68 min-w-0 flex-col gap-1 lg:w-96">
          <span className="truncate font-medium">{row.original.title}</span>
          <span className="truncate text-xs text-muted-foreground">
            {row.original.body}
          </span>
        </div>
      ),
    },
    {
      id: "category",
      header: "类型",
      cell: ({ row }) => categoryLabel(row.original.category),
    },
    {
      id: "status",
      header: "状态",
      cell: ({ row }) => (row.original.read ? "已读" : "未读"),
    },
    {
      id: "time",
      header: "时间",
      cell: ({ row }) => (
        <time dateTime={row.original.occurredAt}>
          {notificationTime(row.original.occurredAt)}
        </time>
      ),
    },
    {
      id: "actions",
      header: "操作",
      cell: ({ row }) => (
        <Button
          variant="ghost"
          size="sm"
          aria-label={`查看：${row.original.title}`}
          onClick={(event) => {
            detailTrigger.current = event.currentTarget;
            nextDetailTrigger.current =
              event.currentTarget
                .closest("tr")
                ?.nextElementSibling?.querySelector<HTMLButtonElement>(
                  "button",
                ) ?? null;
            setSelected(String(row.original.id));
          }}
        >
          查看
        </Button>
      ),
    },
  ];
  return (
    <div
      className={
        compact
          ? "flex min-h-0 flex-col"
          : "account-management-page notification-inbox flex min-w-0 flex-col gap-6"
      }
      aria-busy={pageLoading || busy}
    >
      {!compact && (
        <>
          <PageHeader
            title="通知中心"
            description={
              <p className="text-sm text-muted-foreground" aria-live="polite">
                {count === undefined ? "未读数量暂不可用" : `${count} 条未读`}
              </p>
            }
          >
            <Button
              variant="ghost"
              disabled={
                busy || (!uncertainAll && (!page.data || count === "0"))
              }
              onClick={() => void markRead()}
            >
              {uncertainAll ? "重试全部已读" : "全部通知已读"}
            </Button>
          </PageHeader>
          <div
            ref={filtersRef}
            className="flex flex-wrap items-center justify-between gap-4"
          >
            <Tabs
              value={category}
              onValueChange={(value) => {
                setCategory(value);
                setCursors([undefined]);
              }}
            >
              <TabsList
                aria-label="通知分类"
                className="group-data-horizontal/tabs:h-control w-80 max-w-full"
              >
                <TabsTrigger value="all">全部</TabsTrigger>
                <TabsTrigger value="TASK">任务</TabsTrigger>
                <TabsTrigger value="COLLABORATION">协作</TabsTrigger>
                <TabsTrigger value="ACCOUNT">账户</TabsTrigger>
              </TabsList>
            </Tabs>
            <Field orientation="horizontal" className="w-auto">
              <Checkbox
                id="notification-unread"
                checked={unreadOnly}
                onCheckedChange={(checked) => {
                  setUnreadOnly(checked === true);
                  setCursors([undefined]);
                }}
              />
              <FieldLabel htmlFor="notification-unread">仅看未读</FieldLabel>
            </Field>
          </div>
        </>
      )}
      {pageError && (
        <div className="flex shrink-0 justify-end">
          <Button
            size="sm"
            variant="outline"
            disabled={page.isValidating}
            onClick={() => void refresh()}
          >
            重新读取
          </Button>
        </div>
      )}
      {compact ? (
        <div className="min-h-0 overflow-y-auto overscroll-contain">
          {pageLoading && !page.data ? (
            <div className="space-y-3" aria-label="正在读取通知">
              {[0, 1, 2].map((key) => (
                <Skeleton key={key} className="h-20 w-full" />
              ))}
            </div>
          ) : !page.data?.items?.length ? (
            <Empty>
              <EmptyHeader>
                <EmptyTitle>
                  {pageError ? "通知读取失败" : "暂无通知"}
                </EmptyTitle>
              </EmptyHeader>
            </Empty>
          ) : (
            <ul className="divide-y divide-border" aria-label="最近通知">
              {page.data.items.map((notification) => (
                <li key={notification.id}>
                  <Link
                    className="flex min-w-0 flex-col gap-1 rounded-md px-2 py-3 outline-none hover:bg-muted focus-visible:ring-1 focus-visible:ring-ring/50"
                    href={`/notifications?notificationId=${notification.id}`}
                    onClick={onNavigate}
                  >
                    <span className="flex items-center justify-between gap-3">
                      <span className="truncate font-medium">
                        {notification.title}
                      </span>
                      {!notification.read && (
                        <span className="shrink-0 text-xs text-muted-foreground">
                          未读
                        </span>
                      )}
                    </span>
                    <span className="line-clamp-2 text-sm break-words text-muted-foreground">
                      {notification.body}
                    </span>
                    <time
                      className="text-xs text-muted-foreground"
                      dateTime={notification.occurredAt}
                    >
                      {categoryLabel(notification.category)} ·{" "}
                      {notificationTime(notification.occurredAt)}
                    </time>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </div>
      ) : (
        <>
          <div className="min-w-0 [--spacing-management-row:3.5rem] [&_table]:min-w-200">
            <DataTable
              columns={columns}
              data={page.data?.items ?? []}
              label="通知列表"
              loading={pageLoading}
              emptyTitle={
                pageError
                  ? "通知读取失败"
                  : unreadOnly
                    ? "没有未读通知"
                    : "暂无通知"
              }
            />
          </div>
          <ListPagination
            mode="cursor"
            label="通知分页"
            itemCount={page.data?.items?.length}
            current={cursors.length}
            pageSize={pageSize}
            pageSizeOptions={PAGE_SIZES}
            disabled={pageLoading || page.isValidating || !!pageError}
            hasNextPage={!!page.data?.nextCursor}
            onPageChange={(next) =>
              setCursors((value) =>
                next < value.length
                  ? value.slice(0, -1)
                  : [...value, page.data!.nextCursor!],
              )
            }
            onPageSizeChange={(size) => {
              setPageSize(size);
              setCursors([undefined]);
            }}
          />
          <Sheet
            open={!!selected}
            onOpenChange={(open) => {
              if (!open) setSelected(undefined);
            }}
          >
            <SheetContent
              className="data-[side=right]:w-full data-[side=right]:sm:max-w-120 [&_[data-slot=sheet-close]]:size-11 sm:[&_[data-slot=sheet-close]]:size-7"
              onCloseAutoFocus={(event) => {
                event.preventDefault();
                if (detailTrigger.current?.isConnected)
                  detailTrigger.current.focus();
                else if (nextDetailTrigger.current?.isConnected)
                  nextDetailTrigger.current.focus();
                else
                  filtersRef.current
                    ?.querySelector<HTMLButtonElement>("button")
                    ?.focus();
              }}
            >
              <SheetHeader className="pr-16">
                <SheetTitle className="break-words">
                  {item?.title ?? "通知详情"}
                </SheetTitle>
                <SheetDescription>
                  {item
                    ? `${categoryLabel(item.category)} · ${notificationTime(item.occurredAt)}`
                    : "通知内容"}
                </SheetDescription>
              </SheetHeader>
              <div className="flex min-h-0 flex-col gap-4 overflow-y-auto px-4 pb-4">
                {detail.isLoading && <Skeleton className="h-24" />}
                {detail.error && (
                  <Empty>
                    <EmptyHeader>
                      <EmptyTitle>通知暂时无法查看</EmptyTitle>
                    </EmptyHeader>
                    <Button
                      variant="outline"
                      onClick={() => void detail.mutate()}
                    >
                      重试
                    </Button>
                  </Empty>
                )}
                {item && !detail.error && (
                  <>
                    <p className="text-sm break-words whitespace-pre-wrap">
                      {item.body}
                    </p>
                    {readError === selected && !item.read && (
                      <Button
                        variant="outline"
                        disabled={busy}
                        onClick={() => void markRead(item.id)}
                      >
                        重试标记已读
                      </Button>
                    )}
                    {href && (
                      <Button asChild>
                        <Link href={href}>
                          查看
                          {item.actionType === "GENERATION_TASK"
                            ? "任务"
                            : item.actionType === "CREDIT_ACCOUNT"
                              ? "积分"
                              : "邀请"}
                        </Link>
                      </Button>
                    )}
                  </>
                )}
              </div>
            </SheetContent>
          </Sheet>
        </>
      )}
    </div>
  );
}
