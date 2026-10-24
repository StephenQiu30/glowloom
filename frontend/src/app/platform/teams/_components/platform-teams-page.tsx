"use client";
import { ContentState } from "@/components/layout/content-state";
import { readErrorMessage } from "@/lib/read-error";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { PlatformTeamInvitations } from "./platform-team-invitations";
import { FieldLabel } from "@/components/ui/field";
import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
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
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
export function PlatformTeamsPage() {
  const avatarDeferred = useRef<{
    resolve: () => void;
    reject: (e: Error) => void;
  } | null>(null);
  const selectionGeneration = useRef(0);
  const [selection, setSelection] = useState(0);
  const [page, setPage] = useState<API.PlatformTeamPage | null>(null);
  const [team, setTeam] = useState<API.PlatformTeam | null>(null);
  const [filters, setFilters] = useState<API.platformTeamsParams>({
    limit: 50,
  });
  const [reload, setReload] = useState(0);
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
  }, []);
  const read = useReadBoundary(`${JSON.stringify(filters)}:${reload}`);
  useEffect(() => {
    const c = new AbortController();
    platformTeams(filters, { signal: c.signal })
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
      });
    return () => c.abort();
  }, [filters, reload, clearDetail]);
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
  return (
    <div className="mx-auto grid w-full max-w-5xl gap-5 p-4">
      <h1 className="text-2xl">平台团队管理</h1>
      <p>仅管理团队元数据、账号与治理状态。平台角色不会授予团队作品访问权。</p>
      <div className="flex gap-4">
        <Link href="/users" className="underline">
          用户管理
        </Link>
        <Link href="/platform/settings" className="underline">
          平台设置
        </Link>
      </div>
      <form
        className="grid gap-3 sm:grid-cols-2"
        onSubmit={(e) => {
          e.preventDefault();
          const data = new FormData(e.currentTarget);
          const f: Record<string, string | number | boolean> = { limit: 50 };
          for (const [k, v] of data)
            if (typeof v === "string" && v.trim())
              f[k] = k === "governance_blocked" ? v === "true" : v.trim();
          setPage(null);
          setError("");
          clearDetail();
          setFilters(f);
        }}
      >
        {[
          ["search", "团队名称"],
          ["id", "团队 ID"],
          ["administrator_id", "当前管理员 ID"],
          ["created_from", "创建时间起（RFC3339）"],
          ["created_to", "创建时间止（不含）"],
        ].map(([key, label]) => (
          <FieldLabel key={key}>
            {label}
            <Input
              name={key}
              placeholder={
                key.startsWith("created") ? "2026-10-01T00:00:00Z" : undefined
              }
            />
          </FieldLabel>
        ))}
        <FieldLabel>
          团队状态
          <NativeSelect name="status">
            <NativeSelectOption value="">全部状态</NativeSelectOption>
            {["active", "archived", "suspended"].map((s) => (
              <NativeSelectOption key={s} value={s}>
                {s}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </FieldLabel>
        <FieldLabel>
          治理冻结
          <NativeSelect name="governance_blocked">
            <NativeSelectOption value="">全部</NativeSelectOption>
            <NativeSelectOption value="true">已冻结</NativeSelectOption>
            <NativeSelectOption value="false">未冻结</NativeSelectOption>
          </NativeSelect>
        </FieldLabel>
        <Button disabled={pending}>查询团队</Button>
      </form>
      {error && !page ? (
        <ContentState
          state="error"
          title="团队目录暂时无法读取"
          description={error}
          actions={
            <Button
              onClick={() => {
                setError("");
                setReload((n) => n + 1);
              }}
            >
              重试
            </Button>
          }
        />
      ) : error ? (
        <FeedbackToast
          message={error}
          action={{
            label: "重新读取",
            onClick: () => {
              setError("");
              setReload((n) => n + 1);
            },
          }}
        />
      ) : null}
      {page?.items?.length === 0 ? (
        <ContentState title="没有符合条件的团队" />
      ) : null}
      {!page && !error ? (
        <ContentState state="loading" title="正在读取团队" />
      ) : null}
      <div className="grid gap-3">
        {page?.items?.map((t) => (
          <Button
            className="h-auto justify-start p-4 text-left whitespace-normal"
            variant="outline"
            disabled={pending}
            key={t.id}
            onClick={async () => {
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
            }}
          >
            {t.name} · {t.status} · {t.member_count} 位成员 ·{" "}
            {t.administrator_count} 位管理员
          </Button>
        ))}
      </div>
      {page?.next_cursor ? (
        <Button
          disabled={pending}
          variant="outline"
          onClick={async () => {
            setPending(true);
            try {
              await read(
                (signal) =>
                  platformTeams(
                    {
                      ...filters,
                      cursor: page.next_cursor,
                    },
                    { signal },
                  ),
                (p) =>
                  setPage((v) => ({
                    ...p,
                    items: [...(v?.items ?? []), ...(p.items ?? [])],
                  })),
              );
            } catch (e) {
              setError(authErrorMessage(e));
            } finally {
              setPending(false);
            }
          }}
        >
          加载更多团队
        </Button>
      ) : null}
      {team?.id ? (
        <section className="grid gap-4 rounded-xl border p-4">
          <h2>{team.name}</h2>
          <p className="break-all">
            {team.id} · {team.status}
            {team.governance_blocked ? " · 治理冻结" : ""}
          </p>
          <AvatarEditor
            key={`${team.id}:${selection}`}
            assetID={team.avatar_asset_id}
            load={(signal) => platformTeamAvatar({ id: team.id! }, { signal })}
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
                    const body = { expected_revision: team.revision, reason };
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
          <Button
            variant="destructive"
            onClick={() =>
              setAction({
                title:
                  team.status === "suspended" ? "恢复平台停用" : "平台停用团队",
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
                    { signal, headers: { "Idempotency-Key": requestID } },
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
                    { signal, headers: { "Idempotency-Key": requestID } },
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
            <NativeSelect
              aria-label="平台邀请团队角色"
              value={role}
              onChange={(e) => setRole(e.target.value)}
            >
              <NativeSelectOption value="member">普通成员</NativeSelectOption>
              <NativeSelectOption value="team_admin">
                团队管理员
              </NativeSelectOption>
            </NativeSelect>
            <Button>发送团队邀请</Button>
          </form>
          <PlatformTeamInvitations
            key={team.id}
            teamID={team.id}
            reload={reload}
            onAction={setAction}
          />
          <Button variant="outline" onClick={clearDetail}>
            关闭团队详情
          </Button>
        </section>
      ) : null}
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
