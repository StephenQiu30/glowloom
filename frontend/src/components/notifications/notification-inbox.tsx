"use client";

import { DEFAULT_PAGE_SIZE } from "@/lib/pagination";
import { ListPagination } from "@/components/canvas/runtime/components/layout/workspace/list-pagination";
import { useEffect, useRef, useState } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import useSWR from "swr";
import { listNotifications } from "@/api/notifications";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Field, FieldLabel } from "@/components/ui/field";
import { Switch } from "@/components/ui/switch";
import { ScrollArea } from "@/components/ui/scroll-area";
import { ContentState } from "@/components/layout/content-state";
import { PageLoading } from "@/components/layout/loading-state";
import { FeedbackAlert } from "@/components/feedback/feedback-alert";
import { RequestError } from "@/lib/request";
import { NotificationItems } from "./notification-items";
import { NotificationDetail } from "./notification-detail";
import { ReadAllButton } from "./read-all-button";
import { useNotificationCenter } from "./notification-provider";
import { requireInboxNotification } from "./contracts";

export function NotificationInbox() {
  const center = useNotificationCenter();
  const router = useRouter();
  const params = useSearchParams();
  const [category, setCategory] = useState("interaction");
  const [unread, setUnread] = useState(false);
  const [pageSize, setPageSize] = useState<number>(DEFAULT_PAGE_SIZE);
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [selected, setSelected] = useState<string>();
  const focus = useRef<HTMLElement | null>(null);
  const content = useRef<HTMLDivElement>(null);
  const cursor = cursors.at(-1);
  const page = useSWR(
    center?.signedIn
      ? [
          "notification-list",
          center.identity,
          category,
          unread,
          cursor,
          pageSize,
        ]
      : null,
    async () => {
      const result = await listNotifications({
        category,
        unread_only: unread,
        cursor,
        limit: pageSize,
      });
      if (!Array.isArray(result?.items))
        throw new RequestError("invalid_notification_response");
      return { ...result, items: result.items.map(requireInboxNotification) };
    },
    { shouldRetryOnError: false, refreshInterval: 30000 },
  );
  const previousRevision = useRef(center?.revision);
  const revalidateInbox = page.mutate;
  useEffect(() => {
    if (previousRevision.current === center?.revision) return;
    previousRevision.current = center?.revision;
    // Revalidate the same cache entry so reading a detail does not unmount
    // its trigger while the refreshed read state is in flight.
    void revalidateInbox().catch(() => {});
  }, [center?.revision, revalidateInbox]);
  const detailId = selected ?? params.get("notification") ?? undefined;
  const retryList = (
    <Button
      variant="outline"
      size="feedback"
      onClick={() => void page.mutate().catch(() => {})}
    >
      重试
    </Button>
  );
  function closeDetail() {
    setSelected(undefined);
    if (params.has("notification"))
      router.replace("/notifications", { scroll: false });
  }
  return (
    <div
      ref={content}
      tabIndex={-1}
      className="notification-surface notification-inbox flex min-h-0 flex-1 flex-col px-4 pb-4"
    >
      <Tabs
        value={category}
        className="min-h-0 flex-1 gap-4"
        onValueChange={(value) => {
          setCategory(value);
          setCursors([undefined]);
        }}
      >
        <TabsList variant="notification" aria-label="消息分类">
          <TabsTrigger value="interaction">互动</TabsTrigger>
          <TabsTrigger value="official">官方消息</TabsTrigger>
        </TabsList>
        <div className="flex items-center justify-between gap-3">
          <Field orientation="horizontal" className="w-auto gap-2">
            <Switch
              id="notification-unread"
              checked={unread}
              onCheckedChange={(checked) => {
                setUnread(checked);
                setCursors([undefined]);
              }}
            />
            <FieldLabel htmlFor="notification-unread">仅看未读</FieldLabel>
          </Field>
          <ReadAllButton />
        </div>
        {center?.readAllError ? (
          <FeedbackAlert tone="error" title="标记已读失败" />
        ) : null}
        {page.data && page.error && !page.isValidating ? (
          <FeedbackAlert
            tone="error"
            title="消息刷新失败"
            actions={retryList}
          />
        ) : null}
        <TabsContent value={category} className="flex min-h-0 flex-1 flex-col">
          <ScrollArea
            className="min-h-0 flex-1"
            key={`${category}:${unread}:${cursor ?? "first"}`}
          >
            {!page.data && page.isValidating ? (
              <PageLoading message="正在读取消息" />
            ) : !page.data && page.error ? (
              <ContentState
                state="error"
                title="消息读取失败"
                actions={retryList}
              />
            ) : !page.data?.items?.length ? (
              <ContentState
                className="min-h-64"
                title={unread ? "暂无未读消息" : "暂无消息"}
              />
            ) : (
              <NotificationItems
                items={page.data.items}
                onSelect={(item, target) => {
                  focus.current = target;
                  setSelected(item.id);
                }}
              />
            )}
          </ScrollArea>
          <ListPagination
            mode="cursor"
            label="通知分页"
            current={cursors.length}
            itemCount={page.data?.items?.length}
            pageSize={pageSize}
            pending={page.isLoading}
            hasNextPage={Boolean(page.data?.next_cursor)}
            onPageChange={(next) => {
              if (next < cursors.length) setCursors(cursors.slice(0, -1));
              else if (page.data?.next_cursor)
                setCursors([...cursors, page.data.next_cursor]);
            }}
            onPageSizeChange={(size) => {
              setPageSize(size);
              setCursors([undefined]);
            }}
          />
        </TabsContent>
      </Tabs>
      {detailId ? (
        <NotificationDetail
          key={`${center?.identity}:${detailId}`}
          id={detailId}
          onClose={closeDetail}
          returnFocus={focus}
          fallbackFocus={content}
        />
      ) : null}
    </div>
  );
}
