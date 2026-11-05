"use client";
import { ContentState } from "@/components/layout/content-state";
import { DirectoryTools } from "@/components/directory/directory-tools";
import { UserActions } from "./user-actions";
import { AccountGovernanceLists } from "./account-governance-lists";
import {
  Confirmation,
  type ConfirmAction,
} from "@/components/governance/confirmation";
import { LoadingState } from "@/components/layout/loading-state";
import {
  DataTable,
  type DataTableColumn,
} from "@/components/canvas/runtime/components/account/list/data-table";
import { ListPagination } from "@/components/canvas/runtime/components/layout/workspace/list-pagination";
import {
  AdminHeading,
  SelectionBar,
} from "@/components/governance/admin-layout";
import { useBatchOperation } from "@/components/governance/use-batch-operation";
import { BatchDialog } from "@/components/governance/batch-dialog";
import { userBatchAction } from "@/components/governance/batch-actions";
import { Badge } from "@/components/ui/badge";
import { ColumnMenu } from "@/components/governance/column-menu";
import { ChevronDown, Plus } from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { useIsMobile } from "@/hooks/use-mobile";
import {
  Sheet,
  SheetContent,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useEffect, useId, useRef, useState } from "react";
import { PageLayout } from "@/components/layout/page-layout";
import { listPlatformUsers, getPlatformUser } from "@/api/identity";
import { RequestError } from "@/lib/request";
import { authErrorMessage } from "@/components/auth/contracts";
import { useSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel } from "@/components/ui/field";

const defaultFilters = {
  search: "",
  status: "",
  platform_role: "",
  order: "created_desc",
} satisfies API.DirectoryExportFilter;

export function UserDirectory() {
  const session = useSession();
  if (session.actor.platform_role !== "platform_admin")
    return (
      <ContentState
        state="error"
        title="暂无访问权限"
        description="此页面仅向平台管理员开放。"
      />
    );
  return <UserAdministration key={session.actor.id} />;
}
function UserAdministration() {
  const session = useSession();
  const [toolsOpen, setToolsOpen] = useState(false);
  const [governanceOpen, setGovernanceOpen] = useState(false);
  const [action, setAction] = useState<ConfirmAction | null>(null);
  const [page, setPage] = useState<API.PlatformUserPageView | null>(null);
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("");
  const [role, setRole] = useState("");
  const [filters, setFilters] =
    useState<API.DirectoryExportFilter>(defaultFilters);
  const [verified, setVerified] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [order, setOrder] = useState<string>(defaultFilters.order);
  const [pending, setPending] = useState(true);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  const [detail, setDetail] = useState<API.PlatformUserDetailView | null>(null);
  const [forbidden, setForbidden] = useState(false);
  const [selectedUsers, setSelectedUsers] = useState<API.PlatformUserView[]>(
    [],
  );
  const [cursors, setCursors] = useState<string[]>([""]);
  const [pageSize, setPageSize] = useState(20);
  const [advanced, setAdvanced] = useState(false);
  const filterId = useId();
  const appliedAdvancedCount =
    Number(filters.verified !== undefined) +
    Number(Boolean(filters.from || filters.to)) +
    Number(filters.order === "created_asc");
  const mobile = useIsMobile();
  const [visibility, setVisibility] = useState<Record<string, boolean>>({});
  const { registerResultTrigger, registerFallbackTrigger, ...batch } =
    useBatchOperation<API.PlatformUserView>((ids) => {
      setSelectedUsers((rows) => rows.filter((r) => !ids.includes(r.id)));
      setPending(true);
      setRetry((n) => n + 1);
    });
  const locked = pending || forbidden || batch.busy || batch.unresolved;
  function canSelectUser(user: API.PlatformUserView) {
    return user.id !== session.actor.id && user.status !== "deleted";
  }
  const selectedInPage = selectedUsers.filter((r) =>
    page?.items.some((item) => item.id === r.id && canSelectUser(item)),
  );
  function resetPage() {
    setCursors([""]);
    setSelectedUsers([]);
  }
  function applyFilters(next: API.DirectoryExportFilter) {
    resetPage();
    setError("");
    setPending(true);
    setPage(null);
    setDetail(null);
    setFilters(next);
  }
  const flight = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    flight.current = controller;
    listPlatformUsers(
      { ...filters, limit: pageSize, cursor: cursors.at(-1) || undefined },
      { signal: controller.signal },
    )
      .then((p) => {
        if (!controller.signal.aborted) {
          setPage(p);
          setError("");
        }
      })
      .catch((reason) => {
        if (!controller.signal.aborted) {
          if (
            reason instanceof RequestError &&
            [401, 403].includes(reason.status)
          ) {
            setForbidden(true);
            setSelectedUsers([]);
            setPage(null);
            setDetail(null);
          }
          setError(authErrorMessage(reason));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setPending(false);
      });
    return () => {
      controller.abort();
      flight.current?.abort();
    };
  }, [filters, retry, session.actor.id, cursors, pageSize]);
  async function inspect(id: string) {
    if (pending) return;
    const controller = new AbortController();
    flight.current = controller;
    setPending(true);
    setError("");
    try {
      const result = await getPlatformUser(
        { id },
        { signal: controller.signal },
      );
      if (!controller.signal.aborted) setDetail(result);
    } catch (reason) {
      if (!controller.signal.aborted) {
        if (
          reason instanceof RequestError &&
          [401, 403].includes(reason.status)
        ) {
          setForbidden(true);
          setPage(null);
          setDetail(null);
        }
        setError(authErrorMessage(reason));
      }
    } finally {
      if (!controller.signal.aborted) setPending(false);
    }
  }
  const columns: DataTableColumn<API.PlatformUserView>[] = [
    {
      id: "identity",
      header: "用户",
      cell: ({ row }) => (
        <div className="flex min-w-0 flex-col gap-1">
          <span className="break-words whitespace-normal">
            {row.original.display_name}
            {row.original.id === session.actor.id ? " · 当前账号" : ""}
          </span>
          <span className="text-xs break-all whitespace-normal text-muted-foreground">
            {row.original.email}
          </span>
        </div>
      ),
    },
    {
      id: "role",
      header: "平台角色",
      cell: ({ row }) =>
        row.original.platform_role === "platform_admin"
          ? "平台管理员"
          : "创作者",
    },
    {
      id: "status",
      header: "状态",
      cell: ({ row }) => (
        <Badge variant="secondary">
          {(
            { active: "启用", disabled: "已禁用", deleted: "已注销" } as Record<
              string,
              string
            >
          )[row.original.status] ?? "未知"}
        </Badge>
      ),
    },
    {
      id: "created",
      header: "注册日期",
      cell: ({ row }) =>
        row.original.create_time
          ? new Date(row.original.create_time).toLocaleDateString()
          : "—",
    },
    {
      id: "actions",
      header: "操作",
      cell: ({ row }) => (
        <Button
          variant="ghost"
          disabled={locked}
          onClick={() => void inspect(row.original.id)}
          aria-label={`查看 ${row.original.display_name}`}
        >
          查看
        </Button>
      ),
    },
  ];
  if (batch.denied)
    return (
      <ContentState
        state="error"
        title="暂无访问权限"
        description="管理员权限或会话已失效，请重新登录后继续。"
      />
    );
  return (
    <div className="flex w-full flex-col gap-5">
      <AdminHeading title="用户管理" />
      <Collapsible asChild open={advanced} onOpenChange={setAdvanced}>
        <form
          ref={registerFallbackTrigger}
          tabIndex={-1}
          aria-label="用户筛选"
          className="flex min-w-0 flex-col gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (locked) return;
            if (from && to && new Date(from) >= new Date(to)) {
              setError("结束时间必须晚于开始时间。");
              return;
            }
            applyFilters({
              search: query.trim(),
              status,
              platform_role: role,
              order,
              ...(verified ? { verified: verified === "true" } : {}),
              ...(from ? { from: new Date(from).toISOString() } : {}),
              ...(to ? { to: new Date(to).toISOString() } : {}),
            });
          }}
        >
          <div className="flex flex-wrap items-center gap-2">
            <Input
              disabled={locked}
              aria-label="搜索用户邮箱或显示名"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="邮箱或显示名"
              maxLength={100}
              className="sm:w-56 xl:w-60"
            />
            <Select
              disabled={locked}
              value={status || "all"}
              onValueChange={(value) => setStatus(value === "all" ? "" : value)}
            >
              <SelectTrigger
                aria-label="用户状态筛选"
                className="max-w-full min-w-28"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="all">全部状态</SelectItem>
                  <SelectItem value="active">启用</SelectItem>
                  <SelectItem value="disabled">禁用</SelectItem>
                  <SelectItem value="deleted">已注销</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
            <Select
              disabled={locked}
              value={role || "all"}
              onValueChange={(value) => setRole(value === "all" ? "" : value)}
            >
              <SelectTrigger
                aria-label="平台角色筛选"
                className="max-w-full min-w-36"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="all">全部角色</SelectItem>
                  <SelectItem value="creator">创作者</SelectItem>
                  <SelectItem value="platform_admin">平台管理员</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>

            <div
              className="flex flex-wrap items-center gap-2"
              role="group"
              aria-label="筛选操作"
            >
              <Button disabled={locked}>查询</Button>
              <Button
                type="button"
                variant="outline"
                disabled={locked}
                onClick={() => {
                  setQuery(defaultFilters.search);
                  setStatus(defaultFilters.status);
                  setRole(defaultFilters.platform_role);
                  setVerified("");
                  setFrom("");
                  setTo("");
                  setOrder(defaultFilters.order);
                  applyFilters({ ...defaultFilters });
                }}
              >
                重置
              </Button>
              <CollapsibleTrigger asChild>
                <Button
                  type="button"
                  variant="ghost"
                  className="group/filter-toggle"
                >
                  {advanced ? "收起筛选" : "更多筛选"}
                  {!advanced && appliedAdvancedCount > 0
                    ? `（${appliedAdvancedCount}）`
                    : null}
                  <ChevronDown
                    data-icon="inline-end"
                    className="group-aria-expanded/filter-toggle:rotate-180"
                    aria-hidden="true"
                  />
                </Button>
              </CollapsibleTrigger>
            </div>
            <Button
              type="button"
              variant="outline"
              disabled={pending || batch.busy || batch.unresolved}
              onClick={() => {
                setPending(true);
                setForbidden(false);
                setRetry((n) => n + 1);
              }}
            >
              刷新
            </Button>
            {!mobile ? (
              <ColumnMenu
                columns={[
                  { id: "role", label: "平台角色" },
                  { id: "created", label: "注册日期" },
                ]}
                visibility={visibility}
                onChange={setVisibility}
              />
            ) : null}
            <div
              className="ml-auto flex flex-wrap gap-2"
              role="group"
              aria-label="账号目录操作"
            >
              <Button
                type="button"
                variant="outline"
                disabled={forbidden}
                aria-expanded={toolsOpen}
                onClick={() => setToolsOpen((v) => !v)}
              >
                统计与导出
              </Button>
              <Button
                type="button"
                disabled={forbidden}
                aria-expanded={governanceOpen}
                onClick={() => setGovernanceOpen((v) => !v)}
              >
                <Plus data-icon="inline-start" />
                账号邀请
              </Button>
            </div>
          </div>
          <CollapsibleContent className="border-t pt-3">
            <div className="grid gap-x-4 gap-y-3 sm:grid-cols-2 xl:grid-cols-4">
              <Field>
                <FieldLabel htmlFor={`${filterId}-verified`}>
                  邮箱验证
                </FieldLabel>
                <Select
                  disabled={locked}
                  value={verified || "all"}
                  onValueChange={(value) =>
                    setVerified(value === "all" ? "" : value)
                  }
                >
                  <SelectTrigger id={`${filterId}-verified`} className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="all">全部验证状态</SelectItem>
                      <SelectItem value="true">邮箱已验证</SelectItem>
                      <SelectItem value="false">邮箱未验证</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel htmlFor={`${filterId}-from`}>注册时间起</FieldLabel>
                <Input
                  id={`${filterId}-from`}
                  disabled={locked}
                  className="min-w-0"
                  type="datetime-local"
                  value={from}
                  onChange={(e) => setFrom(e.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={`${filterId}-to`}>
                  注册时间止（不含）
                </FieldLabel>
                <Input
                  id={`${filterId}-to`}
                  disabled={locked}
                  className="min-w-0"
                  type="datetime-local"
                  value={to}
                  onChange={(e) => setTo(e.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={`${filterId}-order`}>
                  注册时间排序
                </FieldLabel>
                <Select
                  disabled={locked}
                  value={order}
                  onValueChange={setOrder}
                >
                  <SelectTrigger id={`${filterId}-order`} className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="created_desc">最新注册优先</SelectItem>
                      <SelectItem value="created_asc">最早注册优先</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
            </div>
          </CollapsibleContent>
        </form>
      </Collapsible>
      {error ? (
        <ContentState
          state="error"
          title={forbidden ? "暂无访问权限" : "用户目录暂时无法读取"}
          description={error}
          actions={
            <Button
              disabled={pending}
              onClick={() => {
                setError("");
                setPending(true);
                setForbidden(false);
                setRetry((n) => n + 1);
              }}
            >
              重新读取
            </Button>
          }
        />
      ) : null}
      {pending && !page ? <LoadingState>正在读取用户…</LoadingState> : null}
      {toolsOpen && !forbidden ? (
        <DirectoryTools
          key={`${session.actor.id}:${JSON.stringify(filters)}`}
          filter={filters}
        />
      ) : null}
      {page && !forbidden ? (
        <>
          {selectedInPage.length ? (
            <SelectionBar
              count={selectedInPage.length}
              total={page.items.length}
              disabled={locked || batch.unresolved}
              onClear={() => setSelectedUsers([])}
            >
              {(
                ["enable", "revoke_sessions", "disable", "delete"] as const
              ).map((kind) => (
                <Button
                  key={kind}
                  variant={kind === "delete" ? "destructive" : "outline"}
                  disabled={locked || batch.unresolved}
                  onClick={() =>
                    void batch.prepare(
                      selectedInPage,
                      userBatchAction(kind, session.actor.id),
                    )
                  }
                >
                  {
                    {
                      enable: "启用账号",
                      revoke_sessions: "撤销会话",
                      disable: "禁用账号",
                      delete: "删除账号",
                    }[kind]
                  }
                </Button>
              ))}
            </SelectionBar>
          ) : !pending &&
            page.items.length &&
            !page.items.some(canSelectUser) ? (
            <p role="status" className="text-sm text-muted-foreground">
              本页没有可批量操作的账号。已注销账号和当前登录账号不可选。
            </p>
          ) : null}
          <DataTable
            density="comfortable"
            columns={columns}
            data={page.items}
            label="用户目录"
            emptyTitle="没有匹配的用户"
            columnVisibility={{
              role: !mobile && visibility.role !== false,
              created: !mobile && visibility.created !== false,
            }}
            selection={{
              selected: selectedInPage,
              onChange: setSelectedUsers,
              getLabel: (r) => `${r.display_name} ${r.email}`,
              pageLabel: "选择当前页全部用户",
              canSelect: canSelectUser,
              disabled: locked || batch.unresolved,
            }}
          />
          <ListPagination
            mode="cursor"
            label="用户分页"
            current={cursors.length}
            pageSize={pageSize}
            pageSizeOptions={[20, 50]}
            itemCount={page.items.length}
            hasNextPage={!!page.next_cursor}
            disabled={locked || batch.unresolved}
            onPageChange={(next) => {
              setSelectedUsers([]);
              setPending(true);
              setDetail(null);
              setCursors((current) =>
                next < current.length
                  ? current.slice(0, -1)
                  : [...current, page.next_cursor],
              );
            }}
            onPageSizeChange={(size) => {
              setPageSize(size);
              resetPage();
              setPending(true);
            }}
          />
        </>
      ) : null}
      {batch.showResult ? (
        <Button
          variant="outline"
          className="self-start"
          ref={registerResultTrigger}
          onClick={() => batch.setOpen(true)}
        >
          {batch.action?.compact ? "处理未完成的操作" : "查看批量操作结果"}
        </Button>
      ) : null}
      {!forbidden ? <BatchDialog key={batch.generation} batch={batch} /> : null}
      <Sheet
        open={Boolean(detail)}
        onOpenChange={(open) => {
          if (!open) {
            flight.current?.abort();
            setDetail(null);
          }
        }}
      >
        <SheetContent className="overflow-y-auto p-6 data-[side=right]:w-full data-[side=right]:sm:max-w-[520px]">
          <SheetTitle>账号详情</SheetTitle>
          <SheetDescription>管理账号资料、登录状态与安全。</SheetDescription>
          {detail ? (
            <div className="flex flex-col gap-3">
              <h2 className="text-xl font-medium break-words">
                {detail.user.display_name}
              </h2>
              <p className="text-sm break-all text-muted-foreground">
                {detail.user.email}
              </p>
              <div className="flex gap-2">
                <Badge variant="secondary">
                  {{ active: "启用", disabled: "已禁用", deleted: "已注销" }[
                    detail.user.status
                  ] ?? detail.user.status}
                </Badge>
                <Badge variant="outline">
                  邮箱{detail.user.email_verified_at ? "已验证" : "未验证"}
                </Badge>
              </div>
              <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-4 py-4 text-sm">
                <dt className="text-muted-foreground">平台角色</dt>
                <dd className="text-right">
                  {detail.user.platform_role === "platform_admin"
                    ? "平台管理员"
                    : "创作者"}
                </dd>
                <dt className="text-muted-foreground">注册日期</dt>
                <dd className="text-right">
                  {detail.user.create_time
                    ? new Date(detail.user.create_time).toLocaleDateString()
                    : "—"}
                </dd>
                <dt className="text-muted-foreground">最近登录</dt>
                <dd className="text-right">
                  {detail.user.last_login_at
                    ? new Date(detail.user.last_login_at).toLocaleString()
                    : "—"}
                </dd>
              </dl>
              <UserActions detail={detail} onAction={setAction} />
            </div>
          ) : null}
        </SheetContent>
      </Sheet>
      {governanceOpen ? (
        <AccountGovernanceLists reload={retry} onAction={setAction} />
      ) : null}
      <Confirmation
        key={action?.title ?? "closed"}
        action={action}
        onClose={() => setAction(null)}
        onDone={async () => {
          if (detail) {
            try {
              const next = await getPlatformUser({ id: detail.user.id });
              setDetail(next.user.status === "deleted" ? null : next);
            } catch {
              // The write result is already known. Refresh the directory separately.
              setDetail(null);
            }
          }
          setPending(true);
          setRetry((n) => n + 1);
        }}
      />
    </div>
  );
}
export function UsersPage() {
  return (
    <PageLayout screen="users">
      <UserDirectory />
    </PageLayout>
  );
}
