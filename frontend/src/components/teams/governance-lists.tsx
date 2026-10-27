"use client";
import { ContentState } from "@/components/layout/content-state";
import { readErrorMessage } from "@/lib/read-error";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { FieldLabel } from "@/components/ui/field";
import { useEffect, useState } from "react";
import {
  teamListInvitations,
  createTeamInvitation,
  resendTeamInvitation,
  revokeTeamInvitation,
  teamAuthorityProposals,
  cancelTeamTransfer,
} from "@/api/identity";
import { useReadBoundary } from "@/components/governance/use-read-boundary";
import type { ConfirmAction } from "@/components/governance/confirmation";
import { authErrorMessage } from "@/components/auth/contracts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
export function GovernanceLists({
  teamID,
  reload,
  onAction,
}: {
  teamID: string;
  reload: number;
  onAction: (a: ConfirmAction) => void;
}) {
  const [invites, setInvites] = useState<API.InvitationListPage | null>(null);
  const [proposals, setProposals] = useState<API.ProposalListPage | null>(null);
  const [error, setError] = useState("");
  const [inviteError, setInviteError] = useState("");
  const [proposalError, setProposalError] = useState("");
  const [retry, setRetry] = useState(0);
  function retryRead() {
    setInvites(null);
    setProposals(null);
    setInviteError("");
    setProposalError("");
    setRetry((n) => n + 1);
  }
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const [status, setStatus] = useState("");
  const [proposalStatus, setProposalStatus] = useState("");
  const [pending, setPending] = useState(false);
  const read = useReadBoundary(
    `${teamID}:${reload}:${status}:${proposalStatus}:${retry}`,
  );
  useEffect(() => {
    const c = new AbortController();
    const filter = { limit: 50, ...(status ? { status } : {}) };
    const proposalFilter = {
      limit: 50,
      ...(proposalStatus ? { status: proposalStatus } : {}),
    };
    const opts = { signal: c.signal };
    Promise.allSettled([
      teamListInvitations({ id: teamID, ...filter }, opts),
      teamAuthorityProposals({ id: teamID, ...proposalFilter }, opts),
    ]).then(([i, p]) => {
      if (c.signal.aborted) return;
      if (i.status === "fulfilled") {
        setInvites(i.value);
        setInviteError("");
      } else {
        setInvites(null);
        setInviteError(readErrorMessage(i.reason));
      }
      if (p.status === "fulfilled") {
        setProposals(p.value);
        setProposalError("");
      } else {
        setProposals(null);
        setProposalError(readErrorMessage(p.reason));
      }
    });
    return () => c.abort();
  }, [teamID, reload, status, proposalStatus, retry]);
  return (
    <section className="grid gap-4">
      <h2>{"团队邀请与转交"}</h2>
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          onAction({
            title: "邀请团队成员",
            effect:
              "请求成功表示邀请已创建并进入通知队列；邮件送达尚未确认。受邀者验证邮箱后需主动接受。",
            run: (_r, signal, requestID) =>
              createTeamInvitation(
                { id: teamID },
                { email: email.trim(), team_role: role },
                { signal, headers: { "Idempotency-Key": requestID } },
              ),
          });
        }}
      >
        <FieldLabel className="grow">
          受邀邮箱
          <Input
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </FieldLabel>
        {
          <NativeSelect
            aria-label="邀请团队角色"
            value={role}
            onChange={(e) => setRole(e.target.value)}
          >
            <NativeSelectOption value="member">普通成员</NativeSelectOption>
            <NativeSelectOption value="team_admin">
              团队管理员
            </NativeSelectOption>
          </NativeSelect>
        }
        <Button>发送邀请</Button>
      </form>
      <NativeSelect
        aria-label="邀请状态"
        value={status}
        onChange={(e) => {
          setInvites(null);
          setProposals(null);
          setInviteError("");
          setProposalError("");
          setStatus(e.target.value);
        }}
      >
        <NativeSelectOption value="">全部状态</NativeSelectOption>
        {["pending", "accepted", "declined", "revoked", "expired"].map((s) => (
          <NativeSelectOption key={s} value={s}>
            {s}
          </NativeSelectOption>
        ))}
      </NativeSelect>
      {error ? <FeedbackToast message={error} /> : null}
      {inviteError ? (
        <ContentState
          className="min-h-40"
          state="error"
          title="邀请暂时无法读取"
          description={inviteError}
          actions={<Button onClick={retryRead}>重试读取邀请</Button>}
        />
      ) : !invites ? (
        <ContentState
          className="min-h-40"
          state="loading"
          title="正在读取邀请"
        />
      ) : invites.items?.length === 0 ? (
        <ContentState className="min-h-40" title="暂无邀请" />
      ) : null}
      {invites?.items?.map((i) => (
        <article key={i.id} className="grid gap-2 rounded-xl border p-4">
          <p className="break-all">
            {i.email} · {i.team_role} · {i.status}
          </p>
          <p>有效期至 {i.expires_at}</p>
          {i.status === "pending" ? (
            <div className="flex gap-2">
              {[true, false].map((resend) => (
                <Button
                  key={String(resend)}
                  variant="outline"
                  onClick={() =>
                    onAction({
                      title: resend ? "重发邀请" : "撤销邀请",
                      effect: resend
                        ? "旧链接立即失效；新链接进入通知队列。"
                        : "此邀请立即失效。",
                      reason: !resend,
                      run: (reason, signal) =>
                        (resend ? resendTeamInvitation : revokeTeamInvitation)(
                          { id: teamID, iid: i.id! },
                          {
                            expected_revision: i.revision,
                            ...(!resend ? { reason } : {}),
                          },
                          { signal },
                        ),
                    })
                  }
                >
                  {resend ? "重发" : "撤销"}
                </Button>
              ))}
            </div>
          ) : null}
        </article>
      ))}
      {invites?.next_cursor ? (
        <Button
          disabled={pending}
          variant="outline"
          onClick={async () => {
            setPending(true);
            try {
              const f = {
                limit: 50,
                cursor: invites.next_cursor,
                ...(status ? { status } : {}),
              };
              await read(
                (signal) =>
                  teamListInvitations({ id: teamID, ...f }, { signal }),
                (p) =>
                  setInvites((v) => ({
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
          更多邀请
        </Button>
      ) : null}
      <h3>管理授权提议</h3>
      <NativeSelect
        aria-label="授权提议状态"
        value={proposalStatus}
        onChange={(e) => {
          setProposals(null);
          setProposalError("");
          setProposalStatus(e.target.value);
        }}
      >
        <NativeSelectOption value="">全部提议状态</NativeSelectOption>
        {["pending", "accepted", "declined", "expired", "cancelled"].map(
          (s) => (
            <NativeSelectOption key={s} value={s}>
              {s}
            </NativeSelectOption>
          ),
        )}
      </NativeSelect>
      {proposalError ? (
        <ContentState
          className="min-h-40"
          state="error"
          title="授权提议暂时无法读取"
          description={proposalError}
          actions={<Button onClick={retryRead}>重试读取授权</Button>}
        />
      ) : !proposals ? (
        <ContentState
          className="min-h-40"
          state="loading"
          title="正在读取授权提议"
        />
      ) : proposals.items?.length === 0 ? (
        <ContentState className="min-h-40" title="暂无管理授权提议" />
      ) : null}
      {proposals?.items?.map((p) => (
        <article key={p.id} className="rounded-xl border p-4">
          <p className="break-all">
            {p.kind} · {p.target_user_id} · {p.status}
          </p>
          <p>{p.reason}</p>
          {p.status === "pending" ? (
            <Button
              variant="outline"
              onClick={() =>
                onAction({
                  title: "撤销授权提议",
                  effect: "目标用户将无法接受该提议。",
                  reason: true,
                  run: (reason, signal) =>
                    cancelTeamTransfer(
                      { id: teamID, pid: p.id! },
                      { expected_revision: p.revision, reason },
                      { signal },
                    ),
                })
              }
            >
              撤销提议
            </Button>
          ) : null}
        </article>
      ))}
      {proposals?.next_cursor ? (
        <Button
          disabled={pending}
          variant="outline"
          onClick={async () => {
            setPending(true);
            try {
              const f = {
                limit: 50,
                cursor: proposals.next_cursor,
                ...(proposalStatus ? { status: proposalStatus } : {}),
              };
              await read(
                (signal) =>
                  teamAuthorityProposals({ id: teamID, ...f }, { signal }),
                (p) =>
                  setProposals((v) => ({
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
          更多提议
        </Button>
      ) : null}
    </section>
  );
}
