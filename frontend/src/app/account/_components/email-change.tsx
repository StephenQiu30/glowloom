"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { FieldLabel } from "@/components/ui/field";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  startEmailChange,
  verifyEmailChange,
  completeEmailChange,
  cancelEmailChange,
  reauthenticate,
} from "@/api/identity";
import { clearPrivateSession } from "@/components/auth/session";
import { authErrorMessage, requireAccepted } from "@/components/auth/contracts";
import { RequestError } from "@/lib/request";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
export function EmailChange({
  revision,
  onConflict,
}: {
  revision: string;
  onConflict: () => void;
}) {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [operation, setOperation] = useState("");
  const [codes, setCodes] = useState({ old: "", new: "" });
  const [verified, setVerified] = useState({ old: false, new: false });
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const flight = useRef<AbortController | null>(null);
  const lock = useRef(false);
  useEffect(
    () => () => {
      flight.current?.abort();
    },
    [],
  );
  function clear() {
    setEmail("");
    setPassword("");
    setCodes({ old: "", new: "" });
    setVerified({ old: false, new: false });
    setOperation("");
    setNotice("");
  }
  async function run(action: (signal: AbortSignal) => Promise<void>) {
    if (lock.current) return;
    lock.current = true;
    setPending(true);
    setError("");
    const c = new AbortController();
    flight.current = c;
    try {
      await action(c.signal);
    } catch (e) {
      if (!c.signal.aborted) {
        setError(authErrorMessage(e));
        if (e instanceof RequestError && e.status === 409) {
          clear();
          onConflict();
        }
      }
    } finally {
      lock.current = false;
      if (!c.signal.aborted) setPending(false);
    }
  }
  return (
    <section className="my-6 grid gap-3" aria-label="更换邮箱">
      <h3>更换邮箱</h3>
      <p className="text-sm text-muted-foreground">
        分别验证原邮箱与新邮箱后完成更换。资料和团队关系保留，完成后需重新登录。
      </p>
      {!operation ? (
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            void run(async (signal) => {
              await reauthenticate({ password }, { signal });
              setPassword("");
              const receipt = requireAccepted(
                await startEmailChange(
                  { expected_revision: revision, new_email: email.trim() },
                  { signal },
                ),
              );
              if (!signal.aborted) {
                setOperation(receipt.operation_id);
                setNotice(
                  "验证码请求已受理，请分别检查两个邮箱；邮件是否送达尚未确认。",
                );
              }
            });
          }}
        >
          <FieldLabel>
            新邮箱
            <Input
              type="email"
              required
              autoComplete="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              disabled={pending}
            />
          </FieldLabel>
          <FieldLabel>
            当前密码
            <Input
              type="password"
              required
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              disabled={pending}
            />
          </FieldLabel>
          <Button disabled={pending || !password || !email}>
            验证密码并申请更换
          </Button>
        </form>
      ) : (
        <>
          {(["old", "new"] as const).map((side) => (
            <form
              key={side}
              className="grid gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                void run(async (signal) => {
                  await verifyEmailChange(
                    { id: operation },
                    { side, code: codes[side] },
                    { signal },
                  );
                  if (!signal.aborted) {
                    setVerified((v) => ({ ...v, [side]: true }));
                    setCodes((v) => ({ ...v, [side]: "" }));
                  }
                });
              }}
            >
              <FieldLabel>
                {side === "old" ? "原邮箱验证码" : "新邮箱验证码"}
                <Input
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  required
                  value={codes[side]}
                  onChange={(e) =>
                    setCodes((v) => ({ ...v, [side]: e.target.value }))
                  }
                  disabled={pending || verified[side]}
                />
              </FieldLabel>
              <Button
                disabled={
                  pending || verified[side] || !/^\d{6}$/.test(codes[side])
                }
              >
                {verified[side] ? "已验证" : "验证此邮箱"}
              </Button>
            </form>
          ))}
          <Button
            disabled={pending || !verified.old || !verified.new}
            onClick={() =>
              void run(async (signal) => {
                await completeEmailChange(
                  { id: operation },
                  { expected_revision: revision },
                  { signal },
                );
                if (!signal.aborted) {
                  clear();
                  await clearPrivateSession();
                  router.replace("/login?email_changed=1");
                }
              })
            }
          >
            完成更换并退出登录
          </Button>
          <Button
            variant="outline"
            disabled={pending}
            onClick={() =>
              void run(async (signal) => {
                await cancelEmailChange({ id: operation }, { signal });
                if (!signal.aborted) clear();
              })
            }
          >
            取消本次更换
          </Button>
        </>
      )}
      {error ? <FeedbackToast message={error} /> : null}
      {notice ? <FeedbackToast message={notice} tone="info" /> : null}
    </section>
  );
}
