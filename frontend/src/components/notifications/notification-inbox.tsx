"use client";

import { useRef, useState } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import useSWR from "swr";
import { listNotifications } from "@/api/notifications";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { FieldGroup } from "@/components/ui/field";
import { PageHeader } from "@/components/canvas/runtime/components/layout/workspace/page-header";
import { ContentState } from "@/components/layout/content-state";
import { NotificationItems } from "./notification-items";
import { NotificationDetail } from "./notification-detail";
import {
  NotificationSelect,
  NotificationDateField,
  dateBoundary,
} from "./notification-filters";
import { ReadAllButton } from "./notification-bell";
import { useNotificationCenter } from "./notification-provider";
import { notificationCategories, requireInboxNotification } from "./contracts";

export function NotificationInbox() {
  const center = useNotificationCenter();
  const router = useRouter();
  const params = useSearchParams();
  const [category, setCategory] = useState("all");
  const [unread, setUnread] = useState("all");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [selected, setSelected] = useState<string>();
  const focus = useRef<HTMLElement | null>(null);
  const heading = useRef<HTMLDivElement>(null);
  const cursor = cursors.at(-1);
  const page = useSWR(
    center?.signedIn
      ? [
          "notification-list",
          center.identity,
          center.revision,
          category,
          unread,
          from,
          to,
          cursor,
        ]
      : null,
    () =>
      listNotifications({
        category: category === "all" ? undefined : category,
        unread_only: unread === "unread",
        from: dateBoundary(from),
        to: dateBoundary(to, true),
        cursor,
        limit: 20,
      }),
    { shouldRetryOnError: false, refreshInterval: 30000 },
  );
  const detailId = selected ?? params.get("notification") ?? undefined;
  function filter(change: () => void) {
    change();
    setCursors([undefined]);
  }
  function close() {
    setSelected(undefined);
    if (params.has("notification"))
      router.replace("/notifications", { scroll: false });
  }
  return (
    <div className="notification-surface flex min-w-0 flex-col gap-6">
      <div ref={heading} tabIndex={-1}>
        <PageHeader
          title="通知中心"
          description={
            <span aria-live="polite">
              {center?.summary
                ? `${center.summary.unread_count} 条未读`
                : "未读数量暂不可用"}
            </span>
          }
        >
          <ReadAllButton />
        </PageHeader>
      </div>
      <Tabs
        value={category}
        onValueChange={(value) => filter(() => setCategory(value))}
      >
        <TabsList className="h-auto max-w-full flex-wrap" aria-label="通知分类">
          {notificationCategories.map(([key, label]) => (
            <TabsTrigger key={key} value={key}>
              {label}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
      <FieldGroup className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <NotificationSelect
          label="阅读状态"
          value={unread}
          onChange={(value) => filter(() => setUnread(value))}
          options={[
            ["all", "全部消息"],
            ["unread", "仅看未读"],
          ]}
        />
        <NotificationDateField
          label="开始日期"
          value={from}
          onChange={(value) => filter(() => setFrom(value))}
        />
        <NotificationDateField
          label="结束日期"
          value={to}
          onChange={(value) => filter(() => setTo(value))}
        />
      </FieldGroup>
      {page.error ? (
        <ContentState
          state="error"
          title="通知读取失败"
          actions={
            <Button variant="outline" onClick={() => void page.mutate()}>
              重新读取
            </Button>
          }
        />
      ) : page.isLoading ? (
        <ContentState state="loading" title="正在读取通知" />
      ) : !page.data?.items?.length ? (
        <ContentState
          title={
            category !== "all" || unread !== "all" || from || to
              ? "没有符合条件的通知"
              : "暂无通知"
          }
        />
      ) : (
        <NotificationItems
          items={page.data.items.map(requireInboxNotification)}
          onSelect={(item, target) => {
            focus.current = target;
            setSelected(item.id);
          }}
        />
      )}
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">每页 20 条</p>
        <div className="flex gap-2">
          <Button
            variant="outline"
            disabled={cursors.length === 1 || page.isLoading}
            onClick={() => setCursors((value) => value.slice(0, -1))}
          >
            上一页
          </Button>
          <Button
            variant="outline"
            disabled={!page.data?.next_cursor || page.isLoading}
            onClick={() =>
              setCursors((value) => [...value, page.data!.next_cursor!])
            }
          >
            下一页
          </Button>
        </div>
      </div>
      {detailId ? (
        <NotificationDetail
          key={`${center?.identity}:${detailId}`}
          id={detailId}
          onClose={close}
          returnFocus={focus}
          fallbackFocus={heading}
        />
      ) : null}
    </div>
  );
}
