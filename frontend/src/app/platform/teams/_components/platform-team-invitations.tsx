"use client";
import { ContentState } from "@/components/layout/content-state";
import { readErrorMessage } from "@/lib/read-error";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { useEffect, useState } from "react";
import {
  listPlatformTeamInvitations,
  revokePlatformTeamInvitation,
} from "@/api/identity";
import { authErrorMessage } from "@/components/auth/contracts";
import type { ConfirmAction } from "@/components/governance/confirmation";
import { useReadBoundary } from "@/components/governance/use-read-boundary";
import { Button } from "@/components/ui/button";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
export function PlatformTeamInvitations({
  teamID,
  reload,
  onAction,
}: {
  teamID: string;
  reload: number;
  onAction: (action: ConfirmAction) => void;
}) {
  const [page, setPage] = useState<API.InvitationListPage | null>(null);
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [retry, setRetry] = useState(0);
  const read = useReadBoundary(`${teamID}:${status}:${reload}:${retry}`);
  useEffect(() => {
    const c = new AbortController();
    listPlatformTeamInvitations(
      { id: teamID, limit: 50, ...(status ? { status } : {}) },
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
          setError(readErrorMessage(e));
        }
      });
    return () => c.abort();
  }, [teamID, status, reload, retry]);
  return (
    <section className="grid gap-3" aria-label="团队历史邀请">
      <h3>团队历史邀请</h3>
      <NativeSelect
        aria-label="平台团队邀请状态"
        value={status}
        onChange={(e) => {
          setPage(null);
          setError("");
          setStatus(e.target.value);
        }}
      >
        <NativeSelectOption value="">全部邀请状态</NativeSelectOption>
        {["pending", "accepted", "declined", "revoked", "expired"].map((s) => (
          <NativeSelectOption key={s} value={s}>
            {
              (
                {
                  pending: "待接受",
                  accepted: "已接受",
                  declined: "已拒绝",
                  revoked: "已撤销",
                  expired: "已过期",
                } as Record<string, string>
              )[s]
            }
          </NativeSelectOption>
        ))}
      </NativeSelect>
      {error && !page ? (
        <ContentState
          className="min-h-40"
          state="error"
          title="历史邀请暂时无法读取"
          description={error}
          actions={
            <Button
              onClick={() => {
                setError("");
                setRetry((n) => n + 1);
              }}
            >
              重试
            </Button>
          }
        />
      ) : error ? (
        <FeedbackToast message={error} />
      ) : !page ? (
        <ContentState
          className="min-h-40"
          state="loading"
          title="正在读取历史邀请"
        />
      ) : null}
      {page?.items?.length === 0 ? (
        <ContentState className="min-h-40" title="暂无历史邀请" />
      ) : null}
      {page?.items?.map((i) => (
        <article key={i.id} className="grid gap-2 rounded-xl border p-3">
          <p className="break-all">
            {i.email} ·{" "}
            {i.team_role === "team_admin" ? "团队管理员" : "普通成员"}
          </p>
          <p>
            {(
              {
                pending: "待接受",
                accepted: "已接受",
                declined: "已拒绝",
                revoked: "已撤销",
                expired: "已过期",
              } as Record<string, string>
            )[i.status ?? ""] ?? "状态待确认"}{" "}
            · 有效期至 {i.expires_at}
          </p>
          {i.status === "pending" ? (
            <Button
              variant="outline"
              onClick={() =>
                onAction({
                  title: "撤销团队历史邀请",
                  effect: "该邀请将立即失效，已加入的团队关系不受此操作影响。",
                  reason: true,
                  run: (reason, signal) =>
                    revokePlatformTeamInvitation(
                      { id: teamID, iid: i.id! },
                      { expected_revision: i.revision, reason },
                      { signal },
                    ),
                })
              }
            >
              撤销邀请
            </Button>
          ) : null}
        </article>
      ))}
      {page?.next_cursor ? (
        <Button
          variant="outline"
          disabled={pending}
          onClick={async () => {
            setPending(true);
            try {
              await read(
                (signal) =>
                  listPlatformTeamInvitations(
                    {
                      id: teamID,
                      limit: 50,
                      cursor: page.next_cursor,
                      ...(status ? { status } : {}),
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
          更多历史邀请
        </Button>
      ) : null}
    </section>
  );
}
