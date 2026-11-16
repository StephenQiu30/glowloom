"use client";

import Link from "next/link";
import { useState } from "react";
import useSWR from "swr";
import { Bell } from "lucide-react";
import { listNotifications } from "@/api/notifications";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Popover,
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { ContentState } from "@/components/layout/content-state";
import { useIsMobile } from "@/hooks/use-mobile";
import { useNotificationCenter } from "./notification-provider";
import { NotificationItems } from "./notification-items";
import { requireInboxNotification } from "./contracts";

export function NotificationBell({ onLogin }: { onLogin: () => void }) {
  const center = useNotificationCenter();
  const [open, setOpen] = useState(false);
  const mobile = useIsMobile();
  const page = useSWR(
    open && center?.signedIn
      ? ["recent-notifications", center.identity, center.revision]
      : null,
    () => listNotifications({ limit: 5 }),
    { shouldRetryOnError: false },
  );
  if (!center?.signedIn)
    return (
      <Button
        variant="ghost"
        size="toolbar-icon"
        aria-label="消息中心"
        onClick={onLogin}
      >
        <Bell aria-hidden="true" />
      </Button>
    );
  const count = center.summary?.unread_count;
  const trigger = (
    <Button
      variant="ghost"
      size="toolbar-icon"
      className="relative"
      aria-label={
        count === undefined
          ? "消息中心，未读数量暂不可用"
          : `消息中心，${count}条未读`
      }
    >
      <Bell aria-hidden="true" />
      {count ? (
        <Badge variant="secondary" className="absolute -top-1 -right-1 px-1">
          {count > 99 ? "99+" : count}
        </Badge>
      ) : null}
    </Button>
  );
  const content = (
    <div className="notification-surface flex min-h-0 flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground" aria-live="polite">
          {count === undefined ? "未读数量暂不可用" : `${count} 条未读`}
        </p>
        <ReadAllButton />
      </div>
      {center.summaryError ? (
        <Button
          variant="outline"
          onClick={() => void center.refresh().catch(() => {})}
        >
          重新读取未读数量
        </Button>
      ) : null}
      {page.error ? (
        <ContentState
          className="min-h-32"
          state="error"
          title="通知读取失败"
          actions={
            <Button variant="outline" onClick={() => void page.mutate()}>
              重新读取
            </Button>
          }
        />
      ) : page.isLoading ? (
        <ContentState
          className="min-h-32"
          state="loading"
          title="正在读取通知"
        />
      ) : !page.data?.items?.length ? (
        <ContentState className="min-h-32" title="暂无通知" />
      ) : (
        <div className="max-h-[min(60dvh,480px)] overflow-y-auto overscroll-contain">
          <NotificationItems
            items={page.data.items.map(requireInboxNotification)}
            onNavigate={() => setOpen(false)}
          />
        </div>
      )}
      <Button asChild variant="outline">
        <Link href="/notifications" onClick={() => setOpen(false)}>
          查看全部
        </Link>
      </Button>
    </div>
  );
  return mobile ? (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>{trigger}</SheetTrigger>
      <SheetContent
        className="w-full data-[side=right]:w-full [&_[data-slot=sheet-close]]:size-11"
        aria-describedby={undefined}
      >
        <SheetHeader>
          <SheetTitle>消息</SheetTitle>
        </SheetHeader>
        <div className="min-h-0 overflow-y-auto px-4 pb-4">{content}</div>
      </SheetContent>
    </Sheet>
  ) : (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent
        align="end"
        className="w-104 max-w-[calc(100vw-2rem)] gap-4 p-6"
      >
        <PopoverHeader>
          <PopoverTitle>消息</PopoverTitle>
        </PopoverHeader>
        {content}
      </PopoverContent>
    </Popover>
  );
}

export function ReadAllButton() {
  const center = useNotificationCenter();
  if (!center) return null;
  return (
    <div className="flex flex-col items-end gap-2">
      <Button
        variant="outline"
        size="sm"
        disabled={
          center.readAllPending ||
          !center.summary ||
          (!center.readAllError && center.summary.unread_count === 0)
        }
        onClick={() => void center.readAll()}
      >
        {center.readAllPending
          ? "标记中…"
          : center.readAllError
            ? "重试一键已读"
            : "一键已读"}
      </Button>
      {center.readAllError ? (
        <Alert>
          <AlertTitle>标记失败，未读消息已保留</AlertTitle>
        </Alert>
      ) : null}
    </div>
  );
}
