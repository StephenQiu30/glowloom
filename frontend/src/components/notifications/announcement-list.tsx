"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import useSWR from "swr";
import { listPlatformNotifications } from "@/api/notifications";
import { useSession } from "@/components/auth/session";
import { PageLayout } from "@/components/layout/page-layout";
import { AdminHeading } from "@/components/governance/admin-layout";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemTitle,
  ItemDescription,
} from "@/components/ui/item";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetFooter,
} from "@/components/ui/sheet";
import { FieldGroup } from "@/components/ui/field";
import { Separator } from "@/components/ui/separator";
import {
  NotificationSelect,
  NotificationDateField,
  dateBoundary,
} from "./notification-filters";
import { requireAnnouncement, notificationDate } from "./contracts";
import { positionOptions, audienceOptions } from "./announcement-editor";

export const announcementStates = [
  ["all", "全部状态"],
  ["draft", "草稿"],
  ["scheduled", "待开始"],
  ["active", "展示中"],
  ["expired", "已结束"],
  ["disabled", "已下线"],
] as const;
export const announcementStatus = (value: string) =>
  announcementStates.find(([key]) => key === value)?.[1] ?? value;
const initialFilters = {
  status: "all",
  placement: "all",
  audience: "all",
  from: "",
  to: "",
};
const optionLabel = (
  options: readonly (readonly [string, string])[],
  value?: string,
) => options.find(([key]) => key === value)?.[1] ?? "—";
export { optionLabel };

export function AnnouncementList() {
  const { actor } = useSession();
  const [query, setQuery] = useState("");
  const [keyword, setKeyword] = useState("");
  const [filters, setFilters] = useState(initialFilters);
  const [draft, setDraft] = useState(initialFilters);
  const [filterOpen, setFilterOpen] = useState(false);
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const cursor = cursors.at(-1);
  const page = useSWR(
    actor.platform_role === "platform_admin"
      ? ["announcement-list", actor.id, keyword, filters, cursor]
      : null,
    () =>
      listPlatformNotifications({
        search: keyword || undefined,
        status: filters.status === "all" ? undefined : filters.status,
        placement: filters.placement === "all" ? undefined : filters.placement,
        audience: filters.audience === "all" ? undefined : filters.audience,
        from: dateBoundary(filters.from),
        to: dateBoundary(filters.to, true),
        cursor,
        limit: 20,
      }),
    { shouldRetryOnError: false },
  );
  const search = useMemo(
    () => ({
      value: query,
      onValueChange: setQuery,
      onSearch: (value: string) => {
        setKeyword(value.trim());
        setCursors([undefined]);
      },
      label: "搜索通知",
      placeholder: "搜索通知标题",
    }),
    [query],
  );
  const items = (page.data?.items ?? []).map(requireAnnouncement);
  return (
    <PageLayout screen="platform/notifications" search={search}>
      <div className="notification-surface flex min-w-0 flex-col gap-6">
        <AdminHeading title="通知管理">
          <Button
            variant="outline"
            onClick={() => {
              setDraft(filters);
              setFilterOpen(true);
            }}
          >
            筛选
          </Button>
          <Button asChild>
            <Link href="/platform/notifications/new">新建通知</Link>
          </Button>
        </AdminHeading>
        <div className="flex flex-wrap items-center gap-3">
          <FieldGroup className="w-44">
            <NotificationSelect
              label="状态"
              value={filters.status}
              options={announcementStates}
              onChange={(status) => {
                setFilters((value) => ({ ...value, status }));
                setCursors([undefined]);
              }}
            />
          </FieldGroup>
          {keyword ||
          Object.entries(filters).some(
            ([key, value]) =>
              value !== initialFilters[key as keyof typeof initialFilters],
          ) ? (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setFilters(initialFilters);
                setKeyword("");
                setQuery("");
                setCursors([undefined]);
              }}
            >
              重置
            </Button>
          ) : null}
        </div>
        {page.error ? (
          <ContentState
            state="error"
            title="通知管理暂时无法读取"
            actions={
              <Button variant="outline" onClick={() => void page.mutate()}>
                重新读取
              </Button>
            }
          />
        ) : page.isLoading ? (
          <ContentState
            state="loading"
            loadingLayout="table"
            title="正在读取通知"
          />
        ) : !items.length ? (
          <ContentState title="没有符合条件的通知" />
        ) : (
          <>
            <div className="hidden md:block">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>标题</TableHead>
                    <TableHead>展示位置</TableHead>
                    <TableHead>接收范围</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead>生效时间</TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((item) => (
                    <TableRow key={item.id}>
                      <TableCell className="max-w-80 truncate">
                        {item.draft.title}
                      </TableCell>
                      <TableCell>
                        {optionLabel(positionOptions, item.draft.placement)}
                      </TableCell>
                      <TableCell>
                        {optionLabel(audienceOptions, item.draft.audience)}
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary">
                          {announcementStatus(item.status)}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        {item.draft.starts_at
                          ? notificationDate(item.draft.starts_at)
                          : "—"}
                      </TableCell>
                      <TableCell className="text-right">
                        <Button variant="ghost" size="sm" asChild>
                          <Link href={`/platform/notifications/${item.id}`}>
                            查看
                          </Link>
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            <div className="flex flex-col gap-2 md:hidden">
              {items.map((item) => (
                <div key={item.id}>
                  <Item>
                    <ItemContent>
                      <ItemTitle>{item.draft.title}</ItemTitle>
                      <ItemDescription>
                        {optionLabel(positionOptions, item.draft.placement)} ·{" "}
                        {announcementStatus(item.status)}
                      </ItemDescription>
                      <p className="text-xs text-muted-foreground">
                        {optionLabel(audienceOptions, item.draft.audience)}
                      </p>
                    </ItemContent>
                    <ItemActions>
                      <Button variant="ghost" asChild>
                        <Link href={`/platform/notifications/${item.id}`}>
                          查看
                        </Link>
                      </Button>
                    </ItemActions>
                  </Item>
                  <Separator />
                </div>
              ))}
            </div>
          </>
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
        <Sheet open={filterOpen} onOpenChange={setFilterOpen}>
          <SheetContent
            className="notification-surface w-full overflow-y-auto data-[side=right]:w-full [&_[data-slot=sheet-close]]:size-11"
            aria-describedby={undefined}
          >
            <SheetHeader>
              <SheetTitle>筛选通知</SheetTitle>
            </SheetHeader>
            <FieldGroup className="px-4">
              <NotificationSelect
                label="发布状态"
                value={draft.status}
                onChange={(value) =>
                  setDraft((current) => ({ ...current, status: value }))
                }
                options={announcementStates}
              />
              <NotificationSelect
                label="展示位置"
                value={draft.placement}
                onChange={(value) =>
                  setDraft((current) => ({ ...current, placement: value }))
                }
                options={[["all", "全部位置"], ...positionOptions]}
              />
              <NotificationSelect
                label="接收范围"
                value={draft.audience}
                onChange={(value) =>
                  setDraft((current) => ({ ...current, audience: value }))
                }
                options={[["all", "全部范围"], ...audienceOptions]}
              />
              <NotificationDateField
                label="生效开始日期"
                value={draft.from}
                onChange={(value) =>
                  setDraft((current) => ({ ...current, from: value }))
                }
              />
              <NotificationDateField
                label="生效结束日期"
                value={draft.to}
                onChange={(value) =>
                  setDraft((current) => ({ ...current, to: value }))
                }
              />
            </FieldGroup>
            <SheetFooter>
              <Button
                variant="outline"
                onClick={() => setDraft(initialFilters)}
              >
                重置
              </Button>
              <Button
                onClick={() => {
                  setFilters(draft);
                  setCursors([undefined]);
                  setFilterOpen(false);
                }}
              >
                应用筛选
              </Button>
            </SheetFooter>
          </SheetContent>
        </Sheet>
      </div>
    </PageLayout>
  );
}
