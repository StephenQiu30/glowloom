"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import Link from "next/link";
import { DirectoryTools } from "@/components/directory/directory-tools";
import { UserActions } from "./user-actions";
import { GovernanceLists } from "@/components/teams/governance-lists";
import {
  Confirmation,
  type ConfirmAction,
} from "@/components/governance/confirmation";
import { LoadingState } from "@/components/layout/loading-state";
import {
  Table,
  TableHeader,
  TableRow,
  TableHead,
  TableBody,
  TableCell,
} from "@/components/ui/table";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { useEffect, useRef, useState } from "react";
import { BasicLayout } from "@/components/layout/basic-layout";
import { listPlatformUsers, getPlatformUser } from "@/api/identity";
import { RequestError } from "@/lib/request";
import { authErrorMessage } from "@/components/auth/contracts";
import { useSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
export function UserDirectory() {
  const session = useSession();
  const [toolsOpen, setToolsOpen] = useState(false);
  const [governanceOpen, setGovernanceOpen] = useState(false);
  const [action, setAction] = useState<ConfirmAction | null>(null);
  const [page, setPage] = useState<API.PlatformUserPageView | null>(null);
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("");
  const [role, setRole] = useState("");
  const [filters, setFilters] = useState<API.DirectoryExportFilter>({
    search: "",
    status: "",
    platform_role: "",
  });
  const [verified, setVerified] = useState("");
  const [inTeam, setInTeam] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [order, setOrder] = useState("created_desc");
  const [pending, setPending] = useState(true);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  const [detail, setDetail] = useState<API.PlatformUserDetailView | null>(null);
  const [forbidden, setForbidden] = useState(false);
  const flight = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    flight.current = controller;
    listPlatformUsers({ ...filters, limit: 50 }, { signal: controller.signal })
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
  }, [filters, retry, session.actor.id]);
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
  async function more() {
    if (!page?.next_cursor || pending) return;
    const controller = new AbortController();
    flight.current = controller;
    setPending(true);
    try {
      const next = await listPlatformUsers(
        { ...filters, limit: 50, cursor: page.next_cursor },
        { signal: controller.signal },
      );
      if (!controller.signal.aborted)
        setPage({
          ...next,
          items: [
            ...page.items,
            ...next.items.filter(
              (item) => !page.items.some((old) => old.id === item.id),
            ),
          ],
        });
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
  return (
    <>
      <div className="flow-page-heading users-heading">
        <div>
          <h1>平台用户管理</h1>
          <p>
            邮箱身份、全局账号状态与平台角色。管理操作以当前服务端允许的工作流为准。
          </p>
        </div>
      </div>
      <div className="mb-4 flex flex-wrap gap-4">
        <Link href="/platform/teams" className="underline">
          平台团队管理
        </Link>
        <Link href="/platform/settings" className="underline">
          平台设置
        </Link>
      </div>
      <form
        className="subject-toolbar flex-wrap"
        onSubmit={(e) => {
          e.preventDefault();
          if (from && to && new Date(from) >= new Date(to)) {
            setError("结束时间必须晚于开始时间。");
            return;
          }
          setPending(true);
          setPage(null);
          setDetail(null);
          setFilters({
            search: query.trim(),
            status,
            platform_role: role,
            order,
            ...(verified ? { verified: verified === "true" } : {}),
            ...(inTeam ? { in_team: inTeam === "true" } : {}),
            ...(from ? { from: new Date(from).toISOString() } : {}),
            ...(to ? { to: new Date(to).toISOString() } : {}),
          });
        }}
      >
        <Input
          aria-label="搜索用户邮箱或显示名"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="邮箱或显示名"
          maxLength={100}
          className="max-w-sm"
        />
        <NativeSelect
          aria-label="用户状态筛选"
          value={status}
          onChange={(e) => setStatus(e.target.value)}
        >
          <NativeSelectOption value="">全部状态</NativeSelectOption>
          <NativeSelectOption value="active">启用</NativeSelectOption>
          <NativeSelectOption value="disabled">禁用</NativeSelectOption>
          <NativeSelectOption value="deleted">已注销</NativeSelectOption>
        </NativeSelect>
        <NativeSelect
          aria-label="平台角色筛选"
          value={role}
          onChange={(e) => setRole(e.target.value)}
        >
          <NativeSelectOption value="">全部角色</NativeSelectOption>
          <NativeSelectOption value="creator">创作者</NativeSelectOption>
          <NativeSelectOption value="platform_admin">
            平台管理员
          </NativeSelectOption>
        </NativeSelect>
        <NativeSelect
          aria-label="邮箱验证筛选"
          value={verified}
          onChange={(e) => setVerified(e.target.value)}
        >
          <NativeSelectOption value="">全部验证状态</NativeSelectOption>
          <NativeSelectOption value="true">邮箱已验证</NativeSelectOption>
          <NativeSelectOption value="false">邮箱未验证</NativeSelectOption>
        </NativeSelect>
        <NativeSelect
          aria-label="团队关系筛选"
          value={inTeam}
          onChange={(e) => setInTeam(e.target.value)}
        >
          <NativeSelectOption value="">全部团队关系</NativeSelectOption>
          <NativeSelectOption value="true">有有效团队关系</NativeSelectOption>
          <NativeSelectOption value="false">无有效团队关系</NativeSelectOption>
        </NativeSelect>
        <Input
          type="datetime-local"
          aria-label="注册时间起"
          value={from}
          onChange={(e) => setFrom(e.target.value)}
        />
        <Input
          type="datetime-local"
          aria-label="注册时间止（不含）"
          value={to}
          onChange={(e) => setTo(e.target.value)}
        />
        <NativeSelect
          aria-label="注册时间排序"
          value={order}
          onChange={(e) => setOrder(e.target.value)}
        >
          <NativeSelectOption value="created_desc">
            最新注册优先
          </NativeSelectOption>
          <NativeSelectOption value="created_asc">
            最早注册优先
          </NativeSelectOption>
        </NativeSelect>
        <Button disabled={pending || forbidden}>查询</Button>
        <Button
          type="button"
          variant="outline"
          disabled={pending}
          onClick={() => {
            setPending(true);
            setForbidden(false);
            setRetry((n) => n + 1);
          }}
        >
          刷新
        </Button>
      </form>
      {error ? <FeedbackToast message={error} /> : null}
      {pending && !page ? <LoadingState>正在读取用户…</LoadingState> : null}
      <Button
        variant="outline"
        disabled={forbidden}
        aria-expanded={toolsOpen}
        onClick={() => setToolsOpen((v) => !v)}
      >
        统计与导出
      </Button>
      {toolsOpen && !forbidden ? (
        <DirectoryTools
          key={`${session.actor.id}:${JSON.stringify(filters)}`}
          filter={filters}
        />
      ) : null}
      {page?.as_of ? (
        <p className="text-sm text-muted-foreground">
          目录读取时间：{new Date(page.as_of).toLocaleString()}
        </p>
      ) : null}
      {page ? (
        <div className="users-table-card overflow-x-auto">
          <Table className="users-table">
            <TableHeader>
              <TableRow>
                <TableHead>邮箱</TableHead>
                <TableHead>显示名</TableHead>
                <TableHead>平台角色</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>关联团队</TableHead>
                <TableHead>详情</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {page.items.map((user) => (
                <TableRow key={user.id}>
                  <TableCell>{user.email}</TableCell>
                  <TableCell>
                    {user.display_name}
                    {user.id === session.actor.id ? " · 当前账号" : ""}
                  </TableCell>
                  <TableCell>
                    {user.platform_role === "platform_admin"
                      ? "平台管理员"
                      : "创作者"}
                  </TableCell>
                  <TableCell>
                    {user.status === "deleted" ? "已注销" : user.status}
                  </TableCell>
                  <TableCell>{user.active_team_count}</TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      disabled={pending}
                      onClick={() => void inspect(user.id)}
                    >
                      查看 {user.display_name}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          {page.items.length === 0 ? (
            <p className="empty-state">没有匹配的用户。</p>
          ) : null}
        </div>
      ) : null}
      {page?.next_cursor ? (
        <Button
          variant="outline"
          disabled={pending}
          onClick={() => void more()}
        >
          加载更多用户
        </Button>
      ) : null}
      <Dialog
        open={Boolean(detail)}
        onOpenChange={(open) => {
          if (!open) {
            flight.current?.abort();
            setDetail(null);
          }
        }}
      >
        <DialogContent className="max-h-[85dvh] overflow-y-auto">
          <DialogTitle>账号详情</DialogTitle>
          <DialogDescription>
            平台治理视图包含成员摘要，不包含私人创作内容。
          </DialogDescription>
          {detail ? (
            <div className="space-y-3">
              <p>{detail.user.email}</p>
              <p>
                {detail.user.display_name} · {detail.user.status} ·{" "}
                {detail.user.platform_role}
              </p>
              <p>
                邮箱{detail.user.email_verified_at ? "已验证" : "未验证"}
                ；资料版本 {detail.user.revision}
              </p>
              <h3>团队成员摘要</h3>
              {detail.teams.length ? (
                detail.teams.map((team) => (
                  <p key={team.team_id}>
                    {team.team_id} · {team.role} · {team.status}
                  </p>
                ))
              ) : (
                <p>没有关联团队。</p>
              )}
              <UserActions detail={detail} onAction={setAction} />
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
      <Button
        variant="outline"
        onClick={() => setGovernanceOpen((v) => !v)}
        aria-expanded={governanceOpen}
      >
        账号邀请与平台授权
      </Button>
      {governanceOpen ? (
        <GovernanceLists reload={retry} onAction={setAction} />
      ) : null}
      <Confirmation
        key={action?.title ?? "closed"}
        action={action}
        onClose={() => setAction(null)}
        onDone={async () => {
          if (detail) {
            const next = await getPlatformUser({ id: detail.user.id });
            setDetail(next);
          }
          setRetry((n) => n + 1);
        }}
      />
    </>
  );
}
export function UsersPage() {
  return (
    <BasicLayout screen="users">
      <UserDirectory />
    </BasicLayout>
  );
}
