"use client";
import { useEffect, useState } from "react";
import { Plus, Search, RotateCw, Users, MoreHorizontal } from "lucide-react";
import { ProductShell } from "@/components/layout/product-shell";
import { useSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { listAdminUsers } from "@/api/identity";
import { requireUserPage, userError, type User } from "./contracts";
import { UserDialog, type UserAction } from "./user-dialog";
export function UsersPage() {
  return (
    <ProductShell screen="users">
      <UserDirectory />
    </ProductShell>
  );
}
function UserDirectory() {
  const session = useSession();
  const [items, setItems] = useState<User[]>([]),
    [cursor, setCursor] = useState<string | undefined>(),
    [query, setQuery] = useState(""),
    [filter, setFilter] = useState("all"),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [retry, setRetry] = useState(0),
    [notice, setNotice] = useState(""),
    [dialog, setDialog] = useState<{ action: UserAction; user?: User } | null>(
      null,
    );
  useEffect(() => {
    const c = new AbortController();
    listAdminUsers({ limit: 50 }, { signal: c.signal })
      .then(requireUserPage)
      .then((p) => {
        if (!c.signal.aborted) {
          setItems(p.items);
          setCursor(p.next_cursor);
          setError("");
        }
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(userError(e));
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [retry]);
  function refresh() {
    setLoading(true);
    setRetry((n) => n + 1);
  }
  async function more() {
    if (loading) return;
    setLoading(true);
    try {
      const p = requireUserPage(await listAdminUsers({ limit: 50, cursor }));
      setItems((v) => [
        ...v,
        ...p.items.filter((u) => !v.some((o) => o.id === u.id)),
      ]);
      setCursor(p.next_cursor);
      setError("");
    } catch (e) {
      setError(userError(e));
    } finally {
      setLoading(false);
    }
  }
  const shown = items.filter(
    (u) =>
      (filter === "all" ||
        (filter === "locked" ? u.login_locked : u.status === filter)) &&
      (u.login_name + " " + u.display_name)
        .toLowerCase()
        .includes(query.trim().toLowerCase()),
  );
  return (
    <>
      <div className="flow-page-heading users-heading">
        <div>
          <h1>用户管理</h1>
          <p>管理组织内账号、角色和访问权限。</p>
        </div>
        <Button onClick={() => setDialog({ action: "create" })}>
          <Plus />
          创建用户
        </Button>
      </div>
      <div className="subject-toolbar">
        <div className="subject-search">
          <Search size={16} />
          <Input
            aria-label="搜索用户"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索已加载用户"
            maxLength={100}
          />
        </div>
        <div className="form-label !mb-0">
          <select
            aria-label="用户状态筛选"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          >
            <option value="all">全部状态</option>
            <option value="active">已启用</option>
            <option value="disabled">已停用</option>
            <option value="locked">登录锁定</option>
          </select>
        </div>
        <Button
          variant="ghost"
          className="ml-auto"
          disabled={loading}
          onClick={refresh}
        >
          <RotateCw className={loading ? "animate-spin" : ""} />
          刷新
        </Button>
      </div>
      {notice && (
        <p role="status" className="flow-notice">
          {notice}
        </p>
      )}
      {error && (
        <div role="alert" className="flow-error">
          {error}
          <Button variant="ghost" disabled={loading} onClick={refresh}>
            重试读取
          </Button>
        </div>
      )}
      <div className="users-table-card">
        <table className="users-table">
          <thead>
            <tr>
              <th>登录账号</th>
              <th>显示名</th>
              <th>角色</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {shown.map((u) => (
              <tr key={u.id}>
                <td>{u.login_name}</td>
                <td>
                  <strong className="font-medium">{u.display_name}</strong>
                  {u.id === session.actor.id && (
                    <small className="user-self">当前账号</small>
                  )}
                </td>
                <td>{u.role === "admin" ? "管理员" : "创作者"}</td>
                <td>
                  <span className={"user-state " + u.status}>
                    {u.status === "active" ? "已启用" : "已停用"}
                    {u.login_locked && " · 登录锁定"}
                  </span>
                </td>
                <td>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setDialog({ action: "edit", user: u })}
                  >
                    编辑
                  </Button>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={"管理 " + u.display_name}
                      >
                        <MoreHorizontal />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem
                        onSelect={() => setDialog({ action: "edit", user: u })}
                      >
                        编辑用户
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        disabled={u.id === session.actor.id || !u.login_locked}
                        onSelect={() =>
                          setDialog({ action: "unlock", user: u })
                        }
                      >
                        解除登录锁定
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        disabled={u.id === session.actor.id}
                        onSelect={() =>
                          setDialog({ action: "password", user: u })
                        }
                      >
                        重置密码
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        disabled={u.id === session.actor.id}
                        onSelect={() =>
                          setDialog({
                            action:
                              u.status === "active" ? "disable" : "enable",
                            user: u,
                          })
                        }
                      >
                        {u.status === "active" ? "停用用户" : "启用用户"}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {loading && items.length === 0 ? (
          <div role="status" className="empty-state">
            正在读取用户…
          </div>
        ) : !error && shown.length === 0 ? (
          <div className="empty-state">
            <Users size={32} />
            <h2>
              {query || filter !== "all"
                ? "没有匹配的用户"
                : "当前组织还没有用户"}
            </h2>
            <p>
              {query || filter !== "all"
                ? "调整搜索条件，或加载更多用户。"
                : "创建账号后即可邀请成员登录。"}
            </p>
          </div>
        ) : null}
      </div>
      <div className="users-pagination">
        <span>
          已加载 {items.length} 位用户
          {query || filter !== "all" ? `，当前显示 ${shown.length} 位` : ""}
        </span>
        {cursor && (
          <Button
            variant="outline"
            disabled={loading}
            onClick={() => void more()}
          >
            {loading ? "正在读取…" : "加载更多用户"}
          </Button>
        )}
      </div>
      {dialog && (
        <UserDialog
          key={dialog.action + (dialog.user?.id ?? "new")}
          {...dialog}
          self={dialog.user?.id === session.actor.id}
          onClose={() => {
            setDialog(null);
            refresh();
          }}
          onSaved={(message) => {
            setDialog(null);
            setNotice(message);
            refresh();
          }}
        />
      )}
    </>
  );
}
