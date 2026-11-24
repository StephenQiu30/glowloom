"use client";
import { SelectionToolbar } from "@/components/governance/admin-layout";
import { usePageSelection } from "@/components/governance/use-page-selection";
import { IconAction } from "@/components/ui/icon-action";
import { Search, SlidersHorizontal, Plus, Eye, FunnelX } from "lucide-react";

import { DEFAULT_PAGE_SIZE } from "@/lib/pagination";
import { ListPagination } from "@/components/canvas/runtime/components/layout/workspace/list-pagination";
import { Input } from "@/components/ui/input";
import { useState } from "react";
import useSWR from "swr";
import { listPlatformNotifications } from "@/api/notifications";
import { useSession } from "@/components/auth/session";
import { PageLayout } from "@/components/layout/page-layout";
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
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetFooter,
} from "@/components/ui/sheet";
import { FieldGroup } from "@/components/ui/field";
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
  const [pageSize, setPageSize] = useState<number>(DEFAULT_PAGE_SIZE);
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const cursor = cursors.at(-1);
  const page = useSWR(
    actor.platform_role === "platform_admin"
      ? ["announcement-list", actor.id, keyword, filters, cursor, pageSize]
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
        limit: pageSize,
      }),
    { shouldRetryOnError: false },
  );
  const items = (page.data?.items ?? []).map(requireAnnouncement);
  const checks = usePageSelection(
    items,
    JSON.stringify([actor.id, keyword, filters, Boolean(page.error)]),
    "通知",
    (row) => row.draft.title || row.id,
  );
  return (
    <PageLayout screen="platform/notifications" hideSearch>
      <div className="notification-surface flex min-w-0 flex-col gap-6">
        <h1 className="sr-only">通知管理</h1>
        <form
          aria-label="通知筛选"
          className="flex flex-wrap items-center gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            setKeyword(query.trim());
            setCursors([undefined]);
          }}
        >
          <Input
            className="sm:w-60"
            aria-label="搜索通知"
            placeholder="搜索通知标题"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          <FieldGroup className="max-w-40 [&_[data-slot=field-label]]:sr-only">
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
          <IconAction type="submit" label="搜索通知标题" icon={Search} />
          <IconAction
            label="筛选"
            icon={SlidersHorizontal}
            onClick={() => {
              setDraft(filters);
              setFilterOpen(true);
            }}
          />
          {keyword ||
          Object.entries(filters).some(
            ([key, value]) =>
              value !== initialFilters[key as keyof typeof initialFilters],
          ) ? (
            <IconAction
              label="重置筛选"
              icon={FunnelX}
              onClick={() => {
                setFilters(initialFilters);
                setKeyword("");
                setQuery("");
                setCursors([undefined]);
              }}
            />
          ) : null}
          <SelectionToolbar>
            {" "}
            <IconAction
              label="新建通知"
              icon={Plus}
              variant="default"
              href="/platform/notifications/new"
              className="ml-auto"
            />
          </SelectionToolbar>
        </form>
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
            <div className="min-w-0 overflow-hidden rounded-md border">
              <Table aria-label="通知列表">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-10">{checks.selectAll}</TableHead>
                    <TableHead>标题</TableHead>
                    <TableHead className="hidden md:table-cell">
                      展示位置
                    </TableHead>
                    <TableHead className="hidden md:table-cell">
                      接收范围
                    </TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead className="hidden md:table-cell">
                      生效时间
                    </TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((item) => (
                    <TableRow key={item.id}>
                      <TableCell>{checks.selectRow(item)}</TableCell>
                      <TableCell className="max-w-48 truncate md:max-w-80">
                        {item.draft.title}
                      </TableCell>
                      <TableCell className="hidden md:table-cell">
                        {optionLabel(positionOptions, item.draft.placement)}
                      </TableCell>
                      <TableCell className="hidden md:table-cell">
                        {optionLabel(audienceOptions, item.draft.audience)}
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary">
                          {announcementStatus(item.status)}
                        </Badge>
                      </TableCell>
                      <TableCell className="hidden md:table-cell">
                        {item.draft.starts_at
                          ? notificationDate(item.draft.starts_at)
                          : "—"}
                      </TableCell>
                      <TableCell className="text-right">
                        <IconAction
                          label={`查看 ${item.draft.title}`}
                          href={`/platform/notifications/${item.id}`}
                          icon={Eye}
                        />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </>
        )}
        {checks.actions}
        <ListPagination
          mode="cursor"
          label="公告分页"
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
                重置筛选
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
