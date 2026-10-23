"use client";

import Link from "next/link";
import { Suspense, useState } from "react";
import { Bell } from "lucide-react";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
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
  SheetDescription,
  SheetTrigger,
} from "@/components/ui/sheet";
import { useIsMobile } from "@/hooks/use-mobile";
import { useOptionalNotifications } from "./notification-provider";
import { NotificationInbox } from "./notification-inbox";
import { unreadBadge } from "./lib/notification-state";

export function NotificationBell() {
  const { account, signingOut } = useAccount();
  const inbox = useOptionalNotifications();
  const mobile = useIsMobile();
  const [open, setOpen] = useState(false);
  if (!account || signingOut || !inbox) return null;
  const count = unreadBadge(inbox.summary?.unreadCount);
  const trigger = (
    <Button
      variant="ghost"
      size="icon-lg"
      className="relative size-11 sm:size-9"
      aria-label={
        count === undefined ? "通知，未读数量暂不可用" : `通知，${count}条未读`
      }
    >
      <Bell />
      {count !== undefined && count !== "0" && (
        <Badge
          className="pointer-events-none absolute -top-1 -right-1 px-1 text-xs"
          variant="secondary"
        >
          {count}
        </Badge>
      )}
    </Button>
  );
  const content = (
    <Suspense fallback={<p role="status">正在读取通知…</p>}>
      <NotificationInbox compact onNavigate={() => setOpen(false)} />
      <Button variant="ghost" className="shrink-0" asChild>
        <Link href="/notifications" onClick={() => setOpen(false)}>
          查看全部
        </Link>
      </Button>
    </Suspense>
  );
  return mobile ? (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>{trigger}</SheetTrigger>
      <SheetContent className="data-[side=right]:w-full data-[side=right]:sm:max-w-lg [&_[data-slot=sheet-close]]:size-11">
        <SheetHeader className="pr-16">
          <SheetTitle>通知</SheetTitle>
          <SheetDescription>
            {count === undefined ? "未读数量暂不可用" : `${count} 条未读`}
          </SheetDescription>
        </SheetHeader>
        <div className="flex min-h-0 flex-1 flex-col gap-4 px-4 pb-4">
          {content}
        </div>
      </SheetContent>
    </Sheet>
  ) : (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent
        align="end"
        className="max-h-(--radix-popover-content-available-height) w-104 max-w-[calc(100vw-2rem)] gap-4 p-4"
        aria-label="通知"
      >
        <PopoverHeader className="shrink-0 flex-row items-center justify-between">
          <div className="flex flex-col gap-1">
            <PopoverTitle>通知</PopoverTitle>
            <p className="text-xs text-muted-foreground" aria-live="polite">
              {count === undefined ? "未读数量暂不可用" : `${count} 条未读`}
            </p>
          </div>
          <Button variant="ghost" size="sm" onClick={() => setOpen(false)}>
            关闭
          </Button>
        </PopoverHeader>
        {content}
      </PopoverContent>
    </Popover>
  );
}
