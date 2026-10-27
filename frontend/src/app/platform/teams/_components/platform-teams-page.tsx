"use client";
import { ContentState } from "@/components/layout/content-state";
import { readErrorMessage } from "@/lib/read-error";
import { PlatformTeamInvitations } from "./platform-team-invitations";
import { FieldLabel } from "@/components/ui/field";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  AdminHeading,
  SelectionBar,
} from "@/components/governance/admin-layout";
import {
  DataTable,
  type DataTableColumn,
} from "@/components/canvas/runtime/components/account/list/data-table";
import { ListPagination } from "@/components/canvas/runtime/components/layout/workspace/list-pagination";
import {
  Sheet,
  SheetContent,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Badge } from "@/components/ui/badge";
import { useIsMobile } from "@/hooks/use-mobile";
import { useSession } from "@/components/auth/session";
import { useBatchOperation } from "@/components/governance/use-batch-operation";
import { BatchDialog } from "@/components/governance/batch-dialog";
import { teamBatchAction } from "./team-batch-actions";
import {
  platformTeams,
  platformTeam,
  updatePlatformTeam,
  platformTeamAvatar,
  suspendPlatformTeam,
  restorePlatformTeam,
  proposeTakeover,
  createPlatformTeamInvitation,
} from "@/api/identity";
import { useReadBoundary } from "@/components/governance/use-read-boundary";
import { AvatarEditor } from "@/components/account/avatar-editor";
import {
  Confirmation,
  type ConfirmAction,
} from "@/components/governance/confirmation";
import { authErrorMessage } from "@/components/auth/contracts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
export function PlatformTeamsPage() {
  const session = useSession();
  if (session.actor.platform_role !== "platform_admin")
    return (
      <ContentState
        state="error"
        title="暂无访问权限"
        description="此页面仅向平台管理员开放。"
      />
    );
  return <TeamAdministration key={session.actor.id} />;
}
function TeamAdministration() {
  const mobile = useIsMobile();
  const [selected, setSelected] = useState<API.PlatformTeam[]>([]);
  const [cursors, setCursors] = useState<string[]>([""]);
  const [pageSize, setPageSize] = useState(20);
  const [loading, setLoading] = useState(true);
  const [advanced, setAdvanced] = useState(false);
  const [reload, setReload] = useState(0);
  const { registerResultTrigger, ...batch } =
    useBatchOperation<API.PlatformTeam>((ids) => {
      setSelected((rows) => rows.filter((r) => !ids.includes(r.id!)));
      setReload((n) => n + 1);
    });
  const avatarDeferred = useRef<{
    resolve: () => void;
    reject: (e: Error) => void;
  } | null>(null);
  const selectionGeneration = useRef(0);
  const [selection, setSelection] = useState(0);
  const [page, setPage] = useState<API.PlatformTeamPage | null>(null);
  const [team, setTeam] = useState<API.PlatformTeam | null>(null);
  const [filters, setFilters] = useState<API.platformTeamsParams>({
    limit: 20,
  });
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [action, setAction] = useState<ConfirmAction | null>(null);
  const [name, setName] = useState("");
  const [target, setTarget] = useState("");
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const clearDetail = useCallback(() => {
    const generation = ++selectionGeneration.current;
    avatarDeferred.current?.reject(new Error("团队选择已变化，已取消头像保存"));
    avatarDeferred.current = null;
    setAction(null);
    setTeam(null);
    setSelection(generation);
    return generation;
  }, [setAction, setTeam, setSelection]);
  const read = useReadBoundary(`${JSON.stringify(filters)}:${reload}`);
  useEffect(() => {
    const c = new AbortController();
    platformTeams(
      { ...filters, limit: pageSize, cursor: cursors.at(-1) || undefined },
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
          clearDetail();
          setError(readErrorMessage(e));
        }
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [filters, reload, clearDetail, cursors, pageSize]);
  async function refresh() {
    if (team?.id) {
      try {
        await read(
          (signal) => platformTeam({ id: team.id! }, { signal }),
          (t) => {
            if (selectionGeneration.current !== selection) return;
            setTeam(t);
            setName(t.name ?? "");
          },
        );
      } catch (e) {
        clearDetail();
        setError(authErrorMessage(e));
      }
    }
    setReload((n) => n + 1);
  }
  const locked = pending || loading || batch.busy || batch.unresolved;
  async function inspect(t: API.PlatformTeam) {
    const generation = clearDetail();
    setPending(true);
    try {
      await read(
        (signal) => platformTeam({ id: t.id! }, { signal }),
        (detail) => {
          if (selectionGeneration.current !== generation) return;
          setTeam(detail);
          setName(detail.name ?? "");
        },
      );
    } catch (e) {
      setError(authErrorMessage(e));
    } finally {
      setPending(false);
    }
  }
  const statusLabels: Record<string, string> = {
    active: "正常",
    suspended: "已停用",
    archived: "已归档",
  };
  const columns: DataTableColumn<API.PlatformTeam>[] = [
    {
      id: "name",
      header: "团队",
      cell: ({ row }) => (
        <div className="flex min-w-0 flex-col gap-1">
          <span className="break-words whitespace-normal">
            {row.original.name}
          </span>
          {mobile ? (
            <span className="text-xs text-muted-foreground">
              {row.original.member_count ?? 0} 位成员
            </span>
          ) : null}
        </div>
      ),
    },
    {
      id: "admins",
      header: "管理员",
      cell: ({ row }) => `${row.original.administrator_count ?? 0} 位管理员`,
    },
    {
      id: "members",
      header: "成员数",
      cell: ({ row }) => `${row.original.member_count ?? 0} 人`,
    },
    {
      id: "status",
      header: "状态",
      cell: ({ row }) => (
        <div className="flex flex-wrap gap-1">
          <Badge variant="secondary">
            {statusLabels[row.original.status ?? ""] ?? "未知"}
          </Badge>
          {row.original.governance_blocked ? (
            <Badge variant="outline">待接管</Badge>
          ) : null}
        </div>
      ),
    },
    {
      id: "created",
      header: "创建日期",
      cell: ({ row }) =>
        row.original.created_at
          ? new Date(row.original.created_at).toLocaleDateString()
          : "—",
    },
    {
      id: "actions",
      header: "操作",
      cell: ({ row }) => (
        <Button
          variant="ghost"
          disabled={locked}
          aria-label={`查看 ${row.original.name}`}
          onClick={() => void inspect(row.original)}
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
      <AdminHeading
        title="团队管理"
        description="管理团队信息、成员与团队状态。"
      />
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (locked) return;
          const data = new FormData(e.currentTarget);
          const f: Record<string, string | number | boolean> = {};
          for (const [k, v] of data)
            if (
              typeof v === "string" &&
              v.trim() &&
              !(["status", "governance_blocked"].includes(k) && v === "all")
            )
              f[k] = k === "governance_blocked" ? v === "true" : v.trim();
          setPage(null);
          setError("");
          clearDetail();
          setSelected([]);
          setCursors([""]);
          setLoading(true);
          setFilters(f);
        }}
      >
        <Input
          name="search"
          aria-label="搜索团队名称"
          placeholder="搜索团队名称…"
          className="max-w-sm"
          disabled={locked}
        />
        <Select name="status" disabled={locked} defaultValue="all">
          <SelectTrigger aria-label="团队状态" className="max-w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="all">全部状态</SelectItem>
              {Object.entries(statusLabels).map(([k, v]) => (
                <SelectItem key={k} value={k}>
                  {v}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Collapsible open={advanced} onOpenChange={setAdvanced}>
          <CollapsibleTrigger asChild>
            <Button type="button" variant="ghost">
              更多筛选
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent className="flex flex-wrap gap-2 pt-3">
            <Input
              name="administrator_id"
              aria-label="团队管理员 ID"
              placeholder="团队管理员 ID"
            />
            <Input name="id" aria-label="团队 ID" placeholder="团队 ID" />
            <Select name="governance_blocked" defaultValue="all">
              <SelectTrigger aria-label="治理状态" className="max-w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="all">全部治理状态</SelectItem>
                  <SelectItem value="true">待接管</SelectItem>
                  <SelectItem value="false">正常</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </CollapsibleContent>
        </Collapsible>
        <Button disabled={locked}>查询</Button>
        <Button
          type="button"
          variant="outline"
          disabled={locked}
          onClick={() => {
            setLoading(true);
            setReload((n) => n + 1);
          }}
        >
          刷新
        </Button>
      </form>
      {error ? (
        <ContentState
          state="error"
          title="团队信息暂时无法读取"
          description={error}
          actions={
            <Button
              onClick={() => {
                setError("");
                setLoading(true);
                setReload((n) => n + 1);
              }}
            >
              重新读取
            </Button>
          }
        />
      ) : null}
      {loading && !page ? (
        <ContentState state="loading" title="正在读取团队…" />
      ) : null}
      {page ? (
        <>
          <SelectionBar
            count={selected.length}
            total={page.items?.length ?? 0}
            disabled={locked}
            onClear={() => setSelected([])}
          >
            {(["restore", "suspend"] as const).map((kind) => (
              <Button
                key={kind}
                variant="outline"
                disabled={locked}
                onClick={() =>
                  void batch.prepare(selected, teamBatchAction(kind))
                }
              >
                {kind === "restore" ? "恢复团队" : "停用团队"}
              </Button>
            ))}
          </SelectionBar>
          <DataTable
            label="团队目录"
            density="comfortable"
            columns={columns}
            data={page.items ?? []}
            emptyTitle="没有符合条件的团队"
            columnVisibility={{
              admins: !mobile,
              members: !mobile,
              created: !mobile,
            }}
            selection={{
              selected,
              onChange: setSelected,
              getLabel: (r) => r.name ?? "团队",
              pageLabel: "选择当前页全部团队",
              disabled: locked,
            }}
          />
          <ListPagination
            mode="cursor"
            label="团队分页"
            current={cursors.length}
            pageSize={pageSize}
            pageSizeOptions={[20, 50]}
            itemCount={page.items?.length ?? 0}
            hasNextPage={!!page.next_cursor}
            disabled={locked}
            onPageChange={(next) => {
              setSelected([]);
              clearDetail();
              setLoading(true);
              setCursors((v) =>
                next < v.length ? v.slice(0, -1) : [...v, page.next_cursor!],
              );
            }}
            onPageSizeChange={(size) => {
              setSelected([]);
              clearDetail();
              setLoading(true);
              setPageSize(size);
              setCursors([""]);
            }}
          />
        </>
      ) : null}
      {batch.action ? (
        <Button
          ref={registerResultTrigger}
          variant="outline"
          className="self-start"
          onClick={() => batch.setOpen(true)}
        >
          查看团队批量结果
        </Button>
      ) : null}
      <BatchDialog key={batch.generation} batch={batch} />
      <Sheet
        open={!!team}
        onOpenChange={(open) => {
          if (!open) clearDetail();
        }}
      >
        <SheetContent className="overflow-y-auto p-6 data-[side=right]:w-full data-[side=right]:sm:max-w-lg">
          <SheetTitle>团队详情</SheetTitle>
          <SheetDescription>管理团队资料与成员邀请。</SheetDescription>
          {team?.id ? (
            <section className="flex min-w-0 flex-col gap-4">
              <h2 className="text-xl font-medium">{team.name}</h2>
              <p className="break-all">
                {team.id} · {statusLabels[team.status ?? ""] ?? team.status}
                {team.governance_blocked ? " · 治理冻结" : ""}
              </p>
              <Collapsible>
                <CollapsibleTrigger asChild>
                  <Button variant="outline">编辑团队资料</Button>
                </CollapsibleTrigger>
                <CollapsibleContent className="flex flex-col gap-4 pt-4">
                  <AvatarEditor
                    key={`${team.id}:${selection}`}
                    assetID={team.avatar_asset_id}
                    load={(signal) =>
                      platformTeamAvatar({ id: team.id! }, { signal })
                    }
                    bind={(asset, signal) =>
                      new Promise<void>((resolve, reject) => {
                        if (
                          signal.aborted ||
                          selectionGeneration.current !== selection
                        ) {
                          reject(new Error("团队选择已变化"));
                          return;
                        }
                        const deferred = { resolve, reject };
                        avatarDeferred.current = deferred;
                        signal.addEventListener(
                          "abort",
                          () => {
                            if (avatarDeferred.current === deferred)
                              avatarDeferred.current = null;
                            reject(new Error("已取消"));
                          },
                          { once: true },
                        );
                        setAction({
                          title: `修改 ${team.name} 的头像`,
                          effect: "修改公开团队资料，不取得团队内容访问权限。",
                          reason: true,
                          run: async (reason, proofSignal) => {
                            if (
                              signal.aborted ||
                              proofSignal.aborted ||
                              selectionGeneration.current !== selection
                            )
                              return;
                            const body = {
                              expected_revision: team.revision,
                              reason,
                            };
                            await updatePlatformTeam({ id: team.id! }, body, {
                              signal: proofSignal,
                              data: { ...body, avatar_asset_id: asset },
                            });
                            if (
                              proofSignal.aborted ||
                              selectionGeneration.current !== selection
                            )
                              return;
                            await refresh();
                            if (avatarDeferred.current === deferred)
                              avatarDeferred.current = null;
                            resolve();
                          },
                        });
                      })
                    }
                  />
                  <form
                    className="flex flex-wrap gap-2"
                    onSubmit={(e) => {
                      e.preventDefault();
                      setAction({
                        title: "修改团队名称",
                        effect: "修改团队元数据并通知团队管理员。",
                        reason: true,
                        run: (reason, signal) =>
                          updatePlatformTeam(
                            { id: team.id! },
                            {
                              name: name.trim(),
                              expected_revision: team.revision,
                              reason,
                            },
                            { signal },
                          ),
                      });
                    }}
                  >
                    <Input
                      aria-label="平台团队名称"
                      required
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                    />
                    <Button>保存名称</Button>
                  </form>
                </CollapsibleContent>
              </Collapsible>
              <Button
                variant="outline"
                onClick={() =>
                  setAction({
                    title:
                      team.status === "suspended"
                        ? "恢复平台停用"
                        : "平台停用团队",
                    effect:
                      team.status === "suspended"
                        ? "恢复到停用前的启用或归档状态；治理冻结独立保留。"
                        : "团队访问和创作将被停用；资料与作品保留。",
                    reason: true,
                    run: (reason, signal) =>
                      (team.status === "suspended"
                        ? restorePlatformTeam
                        : suspendPlatformTeam)(
                        { id: team.id! },
                        { expected_revision: team.revision, reason },
                        { signal },
                      ),
                  })
                }
              >
                {team.status === "suspended" ? "恢复团队" : "停用团队"}
              </Button>
              <Collapsible>
                <CollapsibleTrigger asChild>
                  <Button variant="outline">管理交接</Button>
                </CollapsibleTrigger>
                <CollapsibleContent className="pt-4">
                  <form
                    className="grid gap-2"
                    onSubmit={(e) => {
                      e.preventDefault();
                      setAction({
                        title: "提议团队管理接管",
                        effect:
                          "目标账号需主动接受。平台操作本身不会让您成为团队成员。",
                        reason: true,
                        run: (reason, signal, requestID) =>
                          proposeTakeover(
                            { id: team.id! },
                            {
                              target_user_id: target,
                              expected_revision: team.revision,
                              reason,
                            },
                            {
                              signal,
                              headers: { "Idempotency-Key": requestID },
                            },
                          ),
                      });
                    }}
                  >
                    <FieldLabel>
                      接管目标账号 ID
                      <Input
                        required
                        pattern="[0-9a-fA-F-]{36}"
                        value={target}
                        onChange={(e) => setTarget(e.target.value)}
                      />
                    </FieldLabel>
                    <Button>发起管理接管提议</Button>
                  </form>
                </CollapsibleContent>
              </Collapsible>
              <Collapsible>
                <CollapsibleTrigger asChild>
                  <Button variant="outline">管理邀请</Button>
                </CollapsibleTrigger>
                <CollapsibleContent className="flex flex-col gap-4 pt-4">
                  <form
                    className="grid gap-2"
                    onSubmit={(e) => {
                      e.preventDefault();
                      setAction({
                        title: "平台邀请团队成员",
                        effect:
                          "邀请指定邮箱主动加入团队，平台操作者不会取得团队内容权限。",
                        reason: true,
                        run: async (reason, signal, requestID) => {
                          await createPlatformTeamInvitation(
                            { id: team.id! },
                            { email: email.trim(), team_role: role, reason },
                            {
                              signal,
                              headers: { "Idempotency-Key": requestID },
                            },
                          );
                        },
                      });
                    }}
                  >
                    <FieldLabel>
                      受邀邮箱
                      <Input
                        type="email"
                        required
                        value={email}
                        onChange={(e) => setEmail(e.target.value)}
                      />
                    </FieldLabel>
                    <Select
                      value={role}
                      onValueChange={(value) => setRole(value)}
                    >
                      <SelectTrigger
                        aria-label="平台邀请团队角色"
                        className="max-w-full"
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          <SelectItem value="member">普通成员</SelectItem>
                          <SelectItem value="team_admin">团队管理员</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <Button>发送团队邀请</Button>
                  </form>
                  <PlatformTeamInvitations
                    key={team.id}
                    teamID={team.id}
                    reload={reload}
                    onAction={setAction}
                  />
                </CollapsibleContent>
              </Collapsible>
              <Button variant="outline" onClick={clearDetail}>
                关闭团队详情
              </Button>
            </section>
          ) : null}
        </SheetContent>
      </Sheet>
      <Confirmation
        key={`${selection}:${action?.title ?? "closed"}`}
        action={action}
        onClose={() => {
          avatarDeferred.current?.reject(new Error("头像保存已取消"));
          avatarDeferred.current = null;
          setAction(null);
        }}
        onDone={refresh}
      />
    </div>
  );
}
