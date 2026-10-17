"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import {
  acceptInvitation,
  declineInvitation,
  previewInvitation,
  myProposals,
  acceptProposal,
  declineProposal,
} from "@/api/identity";
import {
  readInvitationFragment,
  pendingInvitation,
  clearInvitation,
} from "@/components/auth/invitation-continuity";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import {
  Confirmation,
  type ConfirmAction,
} from "@/components/governance/confirmation";
import { useReadBoundary } from "@/components/governance/use-read-boundary";
import { authErrorMessage } from "@/components/auth/contracts";
import { Button } from "@/components/ui/button";
export function InvitationsPage() {
  const session = useOptionalSession();
  const space = useSpace();
  const [token, setToken] = useState("");
  const [preview, setPreview] = useState<API.InvitationPreview | null>(null);
  const [proposals, setProposals] = useState<API.ProposalListPage | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [action, setAction] = useState<ConfirmAction | null>(null);
  const [retry, setRetry] = useState(0);
  const parsed = useRef(false);
  const read = useReadBoundary(`${session?.actor.id}:${retry}`);
  useEffect(() => {
    if (!parsed.current) {
      parsed.current = true;
      setToken(readInvitationFragment());
    }
  }, []);
  useEffect(() => {
    const c = new AbortController();
    if (!token) return;
    previewInvitation({ token }, { signal: c.signal })
      .then((value) => {
        if (!c.signal.aborted) setPreview(value);
      })
      .catch((e) => {
        if (!c.signal.aborted) {
          setPreview(null);
          setError(authErrorMessage(e));
        }
      });
    return () => c.abort();
  }, [token, retry]);
  useEffect(() => {
    const c = new AbortController();
    if (session?.actor.id)
      myProposals({ limit: 50 }, { signal: c.signal })
        .then((p) => {
          if (!c.signal.aborted) setProposals(p);
        })
        .catch((e) => {
          if (!c.signal.aborted) setError(authErrorMessage(e));
        });
    return () => c.abort();
  }, [session?.actor.id, retry]);
  return (
    <div className="mx-auto grid w-full max-w-3xl gap-6 p-4">
      <h1 className="text-2xl">邀请与授权</h1>
      {notice ? <FeedbackToast message={notice} tone="info" /> : null}
      {error ? (
        <FeedbackToast
          message={error}
          action={{
            label: "重新读取",
            onClick: () => {
              setError("");
              setRetry((n) => n + 1);
            },
          }}
        />
      ) : null}
      {preview ? (
        <section className="grid gap-3 rounded-xl border p-4">
          <h2>收到邀请</h2>
          <p>
            {preview.kind === "team"
              ? `团队邀请：${preview.team_id}，角色 ${preview.team_role}`
              : "账号邀请"}
          </p>
          <p>
            有效期至 {preview.expires_at}
            。仅邀请所指定且已验证的邮箱账号能够接受。
          </p>
          {session ? (
            <div className="flex gap-2">
              {[true, false].map((accept) => (
                <Button
                  key={String(accept)}
                  variant={accept ? "default" : "outline"}
                  onClick={() =>
                    setAction({
                      title: accept ? "接受邀请" : "拒绝邀请",
                      effect: accept
                        ? "接受团队邀请将加入该团队；平台角色保持原状态。"
                        : "拒绝后该邀请将不可再使用。",
                      password: false,
                      run: async (_r, signal) => {
                        await (accept ? acceptInvitation : declineInvitation)(
                          { token: pendingInvitation() },
                          { signal },
                        );
                        if (!signal.aborted) {
                          clearInvitation();
                          setToken("");
                          setPreview(null);
                          setNotice(accept ? "邀请已接受。" : "邀请已拒绝。");
                          await space.refresh();
                        }
                      },
                    })
                  }
                >
                  {accept ? "接受邀请" : "拒绝邀请"}
                </Button>
              ))}
            </div>
          ) : (
            <div className="flex gap-4">
              <Link href="/login" className="underline">
                使用受邀邮箱登录
              </Link>
              <Link href="/register" className="underline">
                验证邮箱并注册
              </Link>
            </div>
          )}
          <Button
            variant="outline"
            onClick={() => {
              clearInvitation();
              setToken("");
              setPreview(null);
              setNotice("已清除当前邀请链接，可稍后重新打开邮件处理。");
            }}
          >
            暂不处理并清除邀请
          </Button>
        </section>
      ) : !token ? (
        <p>请打开邮件中的邀请链接。邀请令牌不会保存在浏览器存储中。</p>
      ) : null}
      {session ? (
        <section className="grid gap-3">
          <h2>待您确认的管理授权</h2>
          {proposals?.items?.length === 0 ? <p>暂无授权提议。</p> : null}
          {proposals?.items?.map((p) => (
            <article key={p.id} className="grid gap-2 rounded-xl border p-4">
              <p>
                {p.kind} · {p.status} · {p.reason}
              </p>
              <p>有效期至 {p.expires_at}</p>
              {p.status === "pending" ? (
                <div className="flex gap-2">
                  {[true, false].map((accept) => (
                    <Button
                      key={String(accept)}
                      onClick={() =>
                        setAction({
                          title: accept ? "接受管理授权" : "拒绝管理授权",
                          effect:
                            "接受后获得提议对应的管理职责。团队权限与平台权限分别生效。",
                          run: (_r, signal) =>
                            (accept ? acceptProposal : declineProposal)(
                              { id: p.id! },
                              { expected_revision: p.revision },
                              { signal },
                            ),
                        })
                      }
                    >
                      {accept ? "接受授权" : "拒绝授权"}
                    </Button>
                  ))}
                </div>
              ) : null}
            </article>
          ))}
          {proposals?.next_cursor ? (
            <Button
              variant="outline"
              onClick={async () => {
                try {
                  await read(
                    (signal) =>
                      myProposals(
                        {
                          limit: 50,
                          cursor: proposals.next_cursor,
                        },
                        { signal },
                      ),
                    (p) =>
                      setProposals((v) => ({
                        ...p,
                        items: [...(v?.items ?? []), ...(p.items ?? [])],
                      })),
                  );
                } catch (e) {
                  setError(authErrorMessage(e));
                }
              }}
            >
              加载更多授权
            </Button>
          ) : null}
        </section>
      ) : null}
      <Confirmation
        key={action?.title ?? "closed"}
        action={action}
        onClose={() => setAction(null)}
        onDone={() => setRetry((n) => n + 1)}
      />
    </div>
  );
}
