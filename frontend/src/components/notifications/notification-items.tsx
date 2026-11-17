"use client";

import Link from "next/link";
import { Bell, Users } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Item,
  ItemContent,
  ItemTitle,
  ItemDescription,
  ItemMedia,
  ItemGroup,
} from "@/components/ui/item";
import { notificationDate, type InboxNotification } from "./contracts";

export function NotificationItems({
  items,
  onSelect,
  onNavigate,
}: {
  items: InboxNotification[];
  onSelect?: (item: InboxNotification, target: HTMLButtonElement) => void;
  onNavigate?: () => void;
}) {
  return (
    <ItemGroup aria-label="通知列表" className="flex min-w-0 flex-col gap-2">
      {items.map((item) => (
        <div role="listitem" key={item.id}>
          <Item asChild className="notification-item min-h-22 text-left">
            {onSelect ? (
              <Button
                type="button"
                data-notification-id={item.id}
                variant="ghost"
                className="h-auto w-full items-start justify-start whitespace-normal"
                onClick={(event) => onSelect(item, event.currentTarget)}
              >
                <NotificationBody item={item} />
              </Button>
            ) : (
              <Link
                data-notification-id={item.id}
                href={`/notifications?notification=${item.id}`}
                onClick={onNavigate}
              >
                <NotificationBody item={item} />
              </Link>
            )}
          </Item>
        </div>
      ))}
    </ItemGroup>
  );
}
function NotificationBody({ item }: { item: InboxNotification }) {
  const Icon = item.category === "collaboration" ? Users : Bell;
  return (
    <>
      <ItemMedia variant="icon">
        <Icon aria-hidden="true" />
      </ItemMedia>
      <ItemContent className="min-w-0 gap-1">
        <div className="flex min-w-0 items-center justify-between gap-2">
          <ItemTitle className="min-w-0 truncate">{item.title}</ItemTitle>
          {!item.read ? <Badge variant="secondary">未读</Badge> : null}
        </div>
        <ItemDescription className="line-clamp-1">{item.body}</ItemDescription>
        <p className="text-xs text-muted-foreground">
          <time dateTime={item.occurred_at}>
            {notificationDate(item.occurred_at)}
          </time>
          {item.status === "expired"
            ? " · 已结束"
            : item.status === "disabled"
              ? " · 已下线"
              : ""}
        </p>
      </ItemContent>
    </>
  );
}
