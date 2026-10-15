"use client";
import { DirectoryTools } from "@/components/directory/directory-tools";
import { useEffect, useState } from "react";
import {
  listTeamMembers,
  teamMemberAvatar,
  removeTeamMember,
  restoreTeamMember,
  suspendTeamMember,
  changeTeamMemberRole,
  proposeTransfer,
} from "@/api/identity";
import { useReadBoundary } from "@/components/governance/use-read-boundary";
import { useOptionalSession } from "@/components/auth/session";
import { BoundAvatar } from "@/components/account/avatar-editor";
import type { ConfirmAction } from "@/components/governance/confirmation";
import { authErrorMessage } from "@/components/auth/contracts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
export function MemberDirectory({
  team,
  reload,
  onAction,
}: {
  team: API.TeamView;
  reload: number;
  onAction: (a: ConfirmAction) => void;
}) {
  const [page, setPage] = useState<API.TeamMemberPageView | null>(null);
  const [search, setSearch] = useState("");
  const [role, setRole] = useState("");
  const [status, setStatus] = useState("");
  const [filter, setFilter] = useState<API.DirectoryExportFilter>({
    search: "",
    role: "",
    status: "",
  });
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [toolsOpen, setToolsOpen] = useState(false);
  const [pending, setPending] = useState(true);
  const [error, setError] = useState("");
  const read = useReadBoundary(
    `${team.id}:${reload}:${JSON.stringify(filter)}`,
  );
  const actor = useOptionalSession();
  const admin = team.capabilities.includes("members:manage");
  useEffect(() => {
    const c = new AbortController();
    listTeamMembers(
      {
        id: team.id,
        limit: 50,
        ...Object.fromEntries(Object.entries(filter).filter(([, v]) => v)),
      },
      { signal: c.signal },
    )
      .then((p) => {
        if (!c.signal.aborted) {
          setPage(p);
          setError("");
        }
      })
      .catch((e) => {
        if (!c.signal.aborted) {
          setPage(null);
          setError(authErrorMessage(e));
        }
      })
      .finally(() => {
        if (!c.signal.aborted) setPending(false);
      });
    return () => c.abort();
  }, [team.id, filter, reload]);
  function mutation(
    member: API.TeamMemberView,
    kind: "suspend" | "restore" | "remove" | "role" | "transfer",
  ) {
    const names = {
      suspend: "停用成员",
      restore: "恢复成员",
      remove: "移出团队",
      role: member.role === "team_admin" ? "改为普通成员" : "设为团队管理员",
      transfer: "提议管理员转交",
    };
    onAction({
      title: names[kind],
      effect:
        kind === "transfer"
          ? "目标成员需主动接受；接受前管理职责保持原状态。"
          : "此操作只影响该成员的团队关系；全局账号和个人作品保留。最后一位可用管理员受服务端保护。",
      reason: true,
      run: (reason, signal, requestID) => {
        const body = { expected_revision: member.revision!, reason };
        const params = { id: team.id, uid: member.user_id };
        if (kind === "role")
          return changeTeamMemberRole(
            params,
            {
              ...body,
              role: member.role === "team_admin" ? "member" : "team_admin",
            },
            { signal },
          );
        if (kind === "transfer")
          return proposeTransfer(
            { id: team.id },
            {
              expected_revision: team.revision,
              reason,
              target_user_id: member.user_id,
            },
            { signal, headers: { "Idempotency-Key": requestID } },
          );
        return {
          suspend: suspendTeamMember,
          restore: restoreTeamMember,
          remove: removeTeamMember,
        }[kind](params, body, { signal });
      },
    });
  }
  return (
    <section className="grid gap-3">
      <h2>团队成员</h2>
      <form
        className="flex flex-wrap gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (from && to && new Date(from) >= new Date(to)) {
            setError("结束时间必须晚于开始时间。");
            return;
          }
          setPending(true);
          setPage(null);
          setFilter({
            search: search.trim(),
            role,
            status,
            ...(from ? { from: new Date(from).toISOString() } : {}),
            ...(to ? { to: new Date(to).toISOString() } : {}),
          });
        }}
      >
        <Input
          aria-label="搜索团队成员"
          placeholder="显示名或邮箱"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <NativeSelect
          aria-label="团队角色筛选"
          value={role}
          onChange={(e) => setRole(e.target.value)}
        >
          <NativeSelectOption value="">全部角色</NativeSelectOption>
          <NativeSelectOption value="member">普通成员</NativeSelectOption>
          <NativeSelectOption value="team_admin">团队管理员</NativeSelectOption>
        </NativeSelect>
        {team.role === "team_admin" ? (
          <NativeSelect
            aria-label="成员状态筛选"
            value={status}
            onChange={(e) => setStatus(e.target.value)}
          >
            <NativeSelectOption value="">全部状态</NativeSelectOption>
            {["active", "suspended", "removed"].map((s) => (
              <NativeSelectOption key={s} value={s}>
                {s}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        ) : null}
        <Input
          type="datetime-local"
          aria-label="加入团队时间起"
          value={from}
          onChange={(e) => setFrom(e.target.value)}
        />
        <Input
          type="datetime-local"
          aria-label="加入团队时间止（不含）"
          value={to}
          onChange={(e) => setTo(e.target.value)}
        />
        <Button disabled={pending}>查询成员</Button>
      </form>
      {team.role === "team_admin" ? (
        <Button
          variant="outline"
          aria-expanded={toolsOpen}
          onClick={() => setToolsOpen((v) => !v)}
        >
          成员统计与导出
        </Button>
      ) : null}
      {toolsOpen && team.role === "team_admin" ? (
        <DirectoryTools
          key={`${team.id}:${JSON.stringify(filter)}`}
          teamID={team.id}
          filter={filter}
          reload={reload}
        />
      ) : null}
      {page?.as_of ? (
        <p className="text-sm text-muted-foreground">
          目录读取时间：{new Date(page.as_of).toLocaleString()}
        </p>
      ) : null}
      {pending ? <p role="status">正在读取成员…</p> : null}
      {error ? <p role="alert">{error}</p> : null}
      {page?.items.length === 0 ? <p>没有符合条件的成员。</p> : null}
      {page?.items.map((m) => (
        <article
          key={m.user_id}
          className="flex flex-wrap items-center gap-3 rounded-xl border p-4"
        >
          <BoundAvatar
            assetID={m.avatar_asset_id}
            load={(signal) =>
              teamMemberAvatar({ id: team.id, uid: m.user_id }, { signal })
            }
          />
          <div className="min-w-0 flex-1 break-words">
            <p>
              {m.display_name} · {m.role}
            </p>
            <p>
              {m.email} · {m.status}
              {m.access_available === false ? " · 当前不可访问" : ""}
            </p>
          </div>
          {admin && m.revision ? (
            <div className="flex flex-wrap gap-2">
              {m.status === "active" ? (
                <>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => mutation(m, "role")}
                  >
                    {m.role === "team_admin" ? "改为普通成员" : "设为管理员"}
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => mutation(m, "suspend")}
                  >
                    停用
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={
                      m.user_id === actor?.actor.id ||
                      m.access_available === false
                    }
                    onClick={() => mutation(m, "transfer")}
                  >
                    转交管理
                  </Button>
                </>
              ) : m.status === "suspended" ? (
                <Button size="sm" onClick={() => mutation(m, "restore")}>
                  恢复成员
                </Button>
              ) : null}
              {m.status !== "removed" ? (
                <Button
                  size="sm"
                  variant="destructive"
                  onClick={() => mutation(m, "remove")}
                >
                  移出团队
                </Button>
              ) : null}
            </div>
          ) : null}
        </article>
      ))}
      {page?.next_cursor ? (
        <Button
          disabled={pending}
          variant="outline"
          onClick={async () => {
            setPending(true);
            try {
              await read(
                (signal) =>
                  listTeamMembers(
                    {
                      id: team.id,
                      ...Object.fromEntries(
                        Object.entries(filter).filter(([, v]) => v),
                      ),
                      cursor: page.next_cursor,
                      limit: 50,
                    },
                    { signal },
                  ),
                (p) =>
                  setPage((v) => ({
                    ...p,
                    items: [...(v?.items ?? []), ...p.items],
                  })),
              );
            } catch (e) {
              setError(authErrorMessage(e));
            } finally {
              setPending(false);
            }
          }}
        >
          加载更多成员
        </Button>
      ) : null}
    </section>
  );
}
