"use client";

import Link from "next/link";
import { Fragment } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Item,
  ItemContent,
  ItemTitle,
  ItemDescription,
} from "@/components/ui/item";
import { Separator } from "@/components/ui/separator";
import {
  categoryLabel,
  notificationDate,
  type InboxNotification,
} from "./contracts";

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
    <div role="list" aria-label="通知列表" className="flex min-w-0 flex-col">
      {items.map((item) => (
        <Fragment key={item.id}>
          <div role="listitem">
            <Item asChild className="min-h-11 text-left">
              {onSelect ? (
                <Button
                  type="button"
                  variant="ghost"
                  className="h-auto w-full justify-start whitespace-normal"
                  onClick={(event) => onSelect(item, event.currentTarget)}
                >
                  <NotificationBody item={item} />
                </Button>
              ) : (
                <Link
                  href={`/notifications?notification=${item.id}`}
                  onClick={onNavigate}
                >
                  <NotificationBody item={item} />
                </Link>
              )}
            </Item>
          </div>
          <Separator />
        </Fragment>
      ))}
    </div>
  );
}
function NotificationBody({ item }: { item: InboxNotification }) {
  return (
    <ItemContent className="min-w-0 gap-2">
      <div className="flex min-w-0 items-start justify-between gap-3">
        <ItemTitle className="min-w-0">{item.title}</ItemTitle>
        {!item.read ? <Badge variant="secondary">未读</Badge> : null}
      </div>
      <ItemDescription>{item.body}</ItemDescription>
      <p className="text-xs text-muted-foreground">
        {categoryLabel(item.category)} ·{" "}
        <time dateTime={item.occurred_at}>
          {notificationDate(item.occurred_at)}
        </time>
      </p>
    </ItemContent>
  );
}
