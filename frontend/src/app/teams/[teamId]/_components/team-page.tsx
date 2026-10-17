"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { getTeam, renameTeam, leaveTeam, teamAvatar } from "@/api/identity";
import { archiveTeam, restoreTeam, updateTeamSettings } from "@/api/workspace";
import { MemberDirectory } from "@/components/teams/member-directory";
import { GovernanceLists } from "@/components/teams/governance-lists";
import { AvatarEditor } from "@/components/account/avatar-editor";
import {
  Confirmation,
  type ConfirmAction,
} from "@/components/governance/confirmation";
import { useSpace } from "@/components/spaces/space-context";
import { authErrorMessage } from "@/components/auth/contracts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
export function TeamPage({ id }: { id: string }) {
  const router = useRouter();
  const space = useSpace();
  const [team, setTeam] = useState<API.TeamView | null>(null);
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const [action, setAction] = useState<ConfirmAction | null>(null);
  useEffect(() => {
    const c = new AbortController();
    getTeam({ id }, { signal: c.signal })
      .then((t) => {
        if (!c.signal.aborted) {
          setTeam(t);
          setName(t.name);
          setError("");
        }
      })
      .catch((e) => {
        if (!c.signal.aborted) {
          setTeam(null);
          setError(authErrorMessage(e));
        }
      });
    return () => c.abort();
  }, [id, reload]);
  async function refresh() {
    setReload((n) => n + 1);
    await space.refresh();
  }
  const manage = team?.capabilities.includes("team:manage");
  return (
    <div className="mx-auto grid w-full max-w-5xl gap-6 p-4">
      <h1 className="text-2xl">团队管理</h1>
      {error ? (
        <FeedbackToast
          message={error}
          action={{
            label: "重新读取团队",
            onClick: () => {
              setError("");
              setReload((n) => n + 1);
            },
          }}
        />
      ) : !team ? (
        <p role="status">正在读取团队…</p>
      ) : null}
      {team ? (
        <>
          <section className="grid gap-4 rounded-xl border p-4">
            <h2>{team.name}</h2>
            <p>
              {team.status} · {team.member_count} 位成员
              {team.governance_blocked ? " · 管理操作已冻结" : ""}
            </p>
            {manage ? (
              <>
                <AvatarEditor
                  assetID={team.avatar_asset_id}
                  onConflict={() => setReload((n) => n + 1)}
                  load={(signal) => teamAvatar({ id }, { signal })}
                  bind={async (asset, signal) => {
                    const body = { expected_revision: team.revision };
                    await renameTeam({ id }, body, {
                      signal,
                      data: { ...body, avatar_asset_id: asset },
                    });
                    const t = await getTeam({ id }, { signal });
                    if (!signal.aborted) setTeam(t);
                  }}
                />
                <form
                  className="flex flex-wrap gap-2"
                  onSubmit={(e) => {
                    e.preventDefault();
                    setAction({
                      title: "保存团队资料",
                      effect: "更新团队名称。成员和创作归属保持原状态。",
                      password: false,
                      run: (_r, signal) =>
                        renameTeam(
                          { id },
                          {
                            expected_revision: team.revision,
                            name: name.trim(),
                          },
                          { signal },
                        ),
                    });
                  }}
                >
                  <Input
                    aria-label="团队名称"
                    required
                    minLength={2}
                    maxLength={50}
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                  />
                  <Button>保存团队名称</Button>
                </form>
                <Button
                  variant="outline"
                  onClick={() =>
                    setAction({
                      title: team.invitation_enabled
                        ? "关闭团队邀请"
                        : "开放团队邀请",
                      effect: team.invitation_enabled
                        ? "关闭后未接受的邀请失效。"
                        : "允许管理员继续邀请成员。",
                      password: false,
                      run: (_r, signal) =>
                        updateTeamSettings(
                          { id },
                          {
                            expected_revision: team.revision,
                            invitation_enabled: !team.invitation_enabled,
                          },
                          { signal },
                        ),
                    })
                  }
                >
                  {team.invitation_enabled ? "关闭团队邀请" : "开放团队邀请"}
                </Button>
                <Button
                  variant="outline"
                  onClick={() =>
                    setAction({
                      title: "归档团队",
                      effect: "归档后团队内容只读，未完成的创作可能阻止归档。",
                      reason: true,
                      run: (reason, signal) =>
                        archiveTeam(
                          { id },
                          { expected_revision: team.revision, reason },
                          { signal },
                        ),
                    })
                  }
                >
                  归档团队
                </Button>
              </>
            ) : null}
            {team.role === "team_admin" &&
            team.status === "archived" &&
            !team.governance_blocked ? (
              <Button
                onClick={() =>
                  setAction({
                    title: "恢复归档团队",
                    effect: "恢复后重新允许团队创作；平台冻结独立生效。",
                    reason: true,
                    run: (reason, signal) =>
                      restoreTeam(
                        { id },
                        { expected_revision: team.revision, reason },
                        { signal },
                      ),
                  })
                }
              >
                恢复团队
              </Button>
            ) : null}
            <Button
              variant="destructive"
              onClick={() =>
                setAction({
                  title: "退出团队",
                  effect:
                    "退出后失去团队内容访问权限。个人账号保留；最后一位管理员须先完成管理转交。",
                  run: async (_r, signal) => {
                    await leaveTeam(
                      { id },
                      { expected_revision: team.membership_revision },
                      { signal },
                    );
                    if (!signal.aborted) {
                      await space.refresh();
                      router.replace("/teams");
                    }
                  },
                })
              }
            >
              退出团队
            </Button>
          </section>
          <MemberDirectory team={team} reload={reload} onAction={setAction} />
          {manage ? (
            <GovernanceLists teamID={id} reload={reload} onAction={setAction} />
          ) : null}
        </>
      ) : null}
      <Confirmation
        key={action?.title ?? "closed"}
        action={action}
        onClose={() => setAction(null)}
        onDone={refresh}
      />
    </div>
  );
}
